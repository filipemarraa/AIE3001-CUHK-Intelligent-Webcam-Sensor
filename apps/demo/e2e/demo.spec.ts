import { expect, test as base, type Page } from 'playwright/test';

const ORIGIN = 'http://127.0.0.1:4300';
const CALIBRATION_TEXT = 'Move into the frame';
// The product target is 15 s from consent to the first estimate; shared CI runners get twice that before a test fails.
const FIRST_ESTIMATE_TIMEOUT_MS = 30_000;

declare global {
  interface Window {
    __cspViolations: string[];
    __streams: MediaStream[];
  }
}

// Every test records page errors, CSP violations and the origin of every request, and asserts all three at the end.
const test = base.extend<{ guard: { requests: string[] } }>({
  guard: [
    async ({ page }, use) => {
      const pageErrors: string[] = [];
      const requests: string[] = [];
      page.on('pageerror', (error) => pageErrors.push(error.message));
      page.on('request', (request) => requests.push(request.url()));
      await page.addInitScript(() => {
        window.__cspViolations = [];
        window.__streams = [];
        document.addEventListener('securitypolicyviolation', (e) => {
          window.__cspViolations.push(`${e.violatedDirective} ${e.blockedURI}`);
        });
        const getUserMedia = navigator.mediaDevices.getUserMedia.bind(navigator.mediaDevices);
        navigator.mediaDevices.getUserMedia = async (constraints) => {
          const stream = await getUserMedia(constraints);
          window.__streams.push(stream);
          return stream;
        };
      });

      const guard = { requests };
      await use(guard);

      const violations = await page.evaluate(() => window.__cspViolations);
      expect(violations).toEqual([]);
      expect(pageErrors).toEqual([]);
      const foreign = requests.filter((url) => /^(https?|wss?):/.test(url) && !url.startsWith(`${ORIGIN}/`));
      expect(foreign).toEqual([]);
    },
    { auto: true },
  ],
});

async function turnOnCamera(page: Page): Promise<void> {
  await page.getByRole('button', { name: 'Turn on camera' }).click();
}

test.describe('real module with the fake camera', () => {
  test('camera, worker, engine and UI: calibration shows the no_face reason', async ({ page, guard }) => {
    await page.goto('/');
    const startedAt = Date.now();
    await turnOnCamera(page);

    await expect(page.getByText(CALIBRATION_TEXT)).toBeVisible({ timeout: FIRST_ESTIMATE_TIMEOUT_MS });
    const firstEstimateMs = Date.now() - startedAt;
    console.log(`firstEstimateMs=${String(firstEstimateMs)} (consent to the no_face reason on screen)`);
    test.info().annotations.push({ type: 'firstEstimateMs', description: String(firstEstimateMs) });
    await expect(page.getByTestId('camera-indicator')).toHaveText('Camera on');

    // Frame cost on the real path: logged and attached, never asserted (shared runners are slower than the laptop).
    await page.getByRole('button', { name: 'Show details' }).click();
    const details = page.getByTestId('details');
    await expect(details).toContainText('Frame cost:', { timeout: 15_000 });
    await page.waitForTimeout(10_000);
    const text = await details.innerText();
    const frameCostMs = /Frame cost: ([\d.]+) ms/.exec(text)?.[1] ?? 'unknown';
    const dropped = /Dropped frames: (\d+)/.exec(text)?.[1] ?? 'unknown';
    console.log(`frameCostMs=${frameCostMs} droppedFrames=${dropped} (headless, fake camera, no face)`);
    test.info().annotations.push({ type: 'frameCostMs', description: frameCostMs });

    // The page only ever talked to its own origin, workers and wasm included.
    expect(guard.requests.some((url) => url.endsWith('/engine.wasm'))).toBe(true);
  });

  test('Exit turns the camera indicator off and ends every track', async ({ page }) => {
    await page.goto('/');
    await turnOnCamera(page);
    await expect(page.getByText(CALIBRATION_TEXT)).toBeVisible({ timeout: FIRST_ESTIMATE_TIMEOUT_MS });
    const live = await page.evaluate(() => window.__streams.flatMap((s) => s.getTracks()).map((t) => t.readyState));
    expect(live.length).toBeGreaterThan(0);
    expect(live).toContain('live');

    await page.getByRole('button', { name: 'Exit' }).click();

    await expect(page.getByTestId('camera-indicator')).toHaveText('Camera off');
    await expect(page.getByRole('button', { name: 'Turn on camera' })).toBeVisible();
    await expect
      .poll(() => page.evaluate(() => window.__streams.flatMap((s) => s.getTracks()).map((t) => t.readyState)))
      .toEqual(live.map(() => 'ended'));
  });

  test('stays on one origin and raises no CSP violation during a measuring session', async ({ page, guard }) => {
    await page.goto('/');
    await turnOnCamera(page);
    await expect(page.getByText(CALIBRATION_TEXT)).toBeVisible({ timeout: FIRST_ESTIMATE_TIMEOUT_MS });
    await page.waitForTimeout(3_000);

    const http = guard.requests.filter((url) => url.startsWith('http'));
    expect(http.length).toBeGreaterThan(0);
    expect(http.every((url) => url.startsWith(`${ORIGIN}/`))).toBe(true);
  });
});

test.describe('simulated module (?mock=1)', () => {
  test('welcome to summary with a fake clock', async ({ page }) => {
    await page.clock.install();
    await page.goto('/?mock=1');
    await turnOnCamera(page);
    await expect(page.getByRole('heading', { name: 'Getting ready' })).toBeVisible();

    // 10 s of warm-up, then the first valid estimate moves the app to the exercise.
    const exercise = page.getByRole('heading', { name: 'Breathe with the circle' });
    for (let seconds = 0; seconds < 30 && !(await exercise.isVisible()); seconds += 1) {
      await page.clock.runFor(1_000);
    }
    await expect(exercise).toBeVisible();

    await page.clock.runFor(120_000);

    await expect(page.getByRole('heading', { name: 'Well done' })).toBeVisible();
    await expect(page.getByText('Heart rate at start:')).toBeVisible();
    await expect(page.getByTestId('camera-indicator')).toHaveText('Camera off');

    await page.getByRole('button', { name: 'Start again' }).click();
    await expect(page.getByRole('button', { name: 'Turn on camera' })).toBeVisible();
  });
});
