# Exhibition checklist

Procedure to run on the exhibition laptop before the exhibition. The team fills the results tables; nothing here is a result yet.

## Pass criteria

All of them, over 10 consecutive sessions:

1. Every estimate with confidence `high` or `medium` is within ±5 bpm of the finger pulse oximeter reading taken at the same moment.
2. No crash and no freeze: the app never needs a reload during or between sessions.
3. Frame cost under 25 ms (mean, read from the transparency panel) and no sustained dropped frames. The panel counter is the number of frames dropped since the previous reading, that is, per second: it must stay near zero (a few per second at most) and must not stay high second after second while the video plays smoothly.
4. First estimate on screen within 15 s of pressing "Turn on camera".

## Setup (once per laptop)

1. Install Go 1.25 or newer, Node.js 24.15 or newer and `brotli`. Use Chrome (current stable). Other browsers are not supported.
2. Production build and server:

   ```bash
   npm ci
   npm run build:wasm --workspace packages/vitals-web
   ./scripts/fetch-assets.sh
   npm run build --workspace packages/vitals-web
   npm run build --workspace apps/demo
   node apps/demo/serve.mjs
   ```

3. Open `http://127.0.0.1:4300/` in Chrome. Do not use `ng serve`: only `serve.mjs` sends the exact Content Security Policy.
4. Close other heavy applications. Keep the laptop plugged in with the power mode on "high performance" (no battery saver), and the display awake.
5. Run the unit and end-to-end tests once on this laptop: `npm test` and `npx playwright test --config apps/demo/playwright.config.ts`.

## Lighting check (every time the stand is set up)

- Face lit from the front by diffuse light. No window or bright screen behind the visitor.
- No flicker: avoid cheap LED strips and screens in the camera view.
- Turn on the camera and open "Show details". Calibration must leave the reasons `low_light` and `no_face` within a few seconds for a person sitting normally.
- If `low_light` stays, add light in front of the visitor; do not lower the threshold in `engine/quality` on the day.

## Performance read (transparency panel)

1. Start a session with a face in view, press "Show details".
2. After 30 s read "Frame cost" three times, 10 s apart, and the "Dropped frames" counter (frames dropped since the previous reading, that is, per second).
3. Record the numbers in the table below.

If the mean frame cost is above 25 ms, the team decides on one of these mitigations (they are not applied today):

- run the pose landmarker every third frame instead of every second;
- draw a smaller image for detection;
- give the `VideoFrame` directly to MediaPipe instead of drawing it on the offscreen canvas.

### Measurements

| Date | Machine and browser | Mode | Frame cost (ms) | Dropped frames per second | Time to first estimate (s) |
|---|---|---|---|---|---|
| 2026-10-21 | Apple M1, Chrome-based Chromium, Playwright | headless, no face (fake camera) | about 20 (19.4, 19.7, 22.3 in three runs) | 3 to 5 | about 2.5 |
|  | exhibition laptop | headed, with a face |  |  |  |

The first row is a regression reference for the end-to-end test, not an exhibition result: there is no face, so the face-dependent work after detection (skin region, mouth, motion) is not exercised. The headed measurement with a face on the exhibition laptop is required before the exhibition.

Note for whoever runs the end-to-end test elsewhere: the Playwright headless shell has no GPU path and runs MediaPipe about five times slower (100 to 200 ms per frame); the test config therefore uses full Chromium (`channel: 'chromium'`). A cold first frame can also take several seconds to compile on the GPU.

## Oximeter protocol (10 sessions)

Reference: a finger pulse oximeter on the hand that rests on the lap, not the hand near the face. Participants stay seated and still, as in the exercise.

For each session:

1. Participant reads the welcome screen and gives consent. The operator starts the session; the full 2 min exercise runs.
2. A second person reads the oximeter every 10 s during the exercise (first read after the calibration ends) and writes the number down. Only numbers are written: no names, no photos, no video.
3. At the same moment they read "Show numbers" on the screen: the heart rate and the confidence (open "Show details" for the confidence).
4. After the exercise, note the lighting condition (front light, mixed, dim) and a free-form skin-tone group on the Monk scale (1 to 10) only if the participant volunteers it.
5. Save the readings under `data/` as CSV with the columns `session, t_s, oximeter_bpm, app_bpm, confidence, lighting`. The folder is gitignored: these are health data and never go to the repository.

For the pass criterion, compare only rows where the confidence is `high` or `medium`. Rows with `low` are recorded but excluded from the ±5 bpm criterion (the screen still shows a value for them). Rows with `invalid` must show no heart rate on screen at all (check the screen, not only the CSV).

### Session results

| Session | Date | Lighting | Rows compared | Max error (bpm) | Mean error (bpm) | Crash or freeze | Pass |
|---|---|---|---|---|---|---|---|
| 1 |  |  |  |  |  |  |  |
| 2 |  |  |  |  |  |  |  |
| 3 |  |  |  |  |  |  |  |
| 4 |  |  |  |  |  |  |  |
| 5 |  |  |  |  |  |  |  |
| 6 |  |  |  |  |  |  |  |
| 7 |  |  |  |  |  |  |  |
| 8 |  |  |  |  |  |  |  |
| 9 |  |  |  |  |  |  |  |
| 10 |  |  |  |  |  |  |  |

If a threshold in `engine/quality` looks wrong after these sessions, change it only with a test that encodes the reason for the new value, and write the change and its numbers in the pull request description.

## During the exhibition

- Chrome in full screen, the `serve.mjs` terminal visible only to the team.
- The demo returns to the welcome screen on its own after 60 s without a face and 60 s after the summary; the camera is off at that point. Check that the "Camera off" indicator is shown whenever nobody is using it.
- "Exit" stops the camera at any time.

## Fallback: `?mock=1`

If the camera, the lighting or the performance fails on the day, open `http://127.0.0.1:4300/?mock=1`. The simulated module follows the same screens and rules with synthetic numbers and no camera. Say so to visitors: the numbers on screen are then simulated and are not their own.
