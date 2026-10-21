import { fileURLToPath } from 'node:url';
import { defineConfig } from 'playwright/test';

const ORIGIN = 'http://127.0.0.1:4300';
const FAKE_VIDEO = fileURLToPath(new URL('./e2e/fake.y4m', import.meta.url));
const ROOT = fileURLToPath(new URL('../..', import.meta.url));

export default defineConfig({
  testDir: './e2e',
  timeout: 90_000,
  workers: 1,
  fullyParallel: false,
  forbidOnly: process.env['CI'] !== undefined,
  retries: 0,
  reporter: [['list']],
  use: {
    baseURL: ORIGIN,
    browserName: 'chromium',
    // Full Chromium in the new headless mode: the headless shell has no GPU path and runs MediaPipe several times slower.
    channel: 'chromium',
    permissions: ['camera'],
    launchOptions: {
      args: [
        '--use-fake-ui-for-media-stream',
        '--use-fake-device-for-media-stream',
        `--use-file-for-fake-video-capture=${FAKE_VIDEO}`,
      ],
    },
  },
  webServer: {
    command: 'node apps/demo/serve.mjs',
    cwd: ROOT,
    url: ORIGIN,
    reuseExistingServer: process.env['CI'] === undefined,
  },
});
