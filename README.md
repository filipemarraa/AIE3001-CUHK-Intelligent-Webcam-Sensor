# Intelligent Webcam Sensor

Estimate **heart rate** and **breathing rate** from an ordinary webcam, in the browser, with an explicit **confidence level**, and use those signals to drive a guided breathing exercise.

Everything runs on the user's device. No video frame leaves the browser, nothing is recorded, and no one is identified.

Course project for **AIE 3001 · AI and Human-Computer Interaction**, The Chinese University of Hong Kong, Shenzhen (Term 1, 2026-27).

## Team

| Member | Owns |
|---|---|
| Tiago de Jesus Chehab | Signal processing and system architecture |
| Vinicius Borges de Alencar | Interface and validation study |
| Filipe Jacobson Marra | Privacy and consent, exhibition safety, user testing |

Every area has one owner and one reviewer from the other two members.

> **Not a medical device.** The values are wellness estimates for a breathing exercise. They are not a diagnosis and must not be used for clinical decisions.

## Status

| Part | State |
|---|---|
| Signal core (`engine/`): resampling, band-pass filter, spectrum, POS heart rate | done, tested on synthetic signals |
| Breathing rate and phase from shoulder movement (`engine/resp`) | done, tested on synthetic signals |
| Signal quality, confidence and reason (`engine/quality`), stateful session (`engine/session`) | done, tested on synthetic signals |
| WebAssembly bridge for the engine (`engine/cmd/wasm`) | done, under 1.5 MB with brotli |
| Module contract, simulated module and real module `createVitals` (`packages/vitals-web/`) | done |
| Camera capture and sensor worker (face and pose landmarks, one worker) | done |
| Demo app (Angular) with consent, calibration, exercise, summary, transparency panel and exhibition mode | done |
| End-to-end test with a fake camera, CI job and exhibition checklist | done; oximeter sessions on the exhibition laptop still to run ([checklist](docs/exhibition-checklist.md)) |
| Validation tool against a reference device | planned |

## What it measures, and what it does not

| Measured | Never measured or inferred |
|---|---|
| Heart rate (beats per minute), from skin colour changes (remote photoplethysmography) | Blood pressure, SpO2, glucose, temperature |
| Breathing rate and phase (in, out, hold), from shoulder movement | Heart rate variability as a number, "stress" scores |
| Signal quality: face position, motion, light, talking, frame timing, spectral SNR | Emotion, mood or any psychological state |

When the signal is not good enough the module reports **no value and a reason** (for example `too_much_motion` or `low_light`) instead of a guess.

## How it works

```
main thread (vitals-web)                    sensor.worker
┌──────────────────────────┐   VideoFrame  ┌──────────────────────────────────────┐
│ camera.ts                │ ───────────►  │ vision/face.ts    forehead + cheeks  │
│  getUserMedia + rVFC     │  (transfer,   │ vision/pose.ts    shoulders y, width │
│  at most 1 frame in      │   closed in   │ vision/quality.ts motion, light,     │
│  flight, others dropped  │   the worker) │                   mouth, frame jitter│
│ vitals.ts (createVitals) │               │        │ Sample (numbers only)       │
│  state machine, events   │  ◄─────────── │ engine/bridge.ts → Go WASM           │
│                          │   Estimate    │   push(sample) / estimate(nowMs)     │
└──────────────────────────┘    1 per s    └──────────────────────────────────────┘
```

One worker holds both the vision step (MediaPipe) and the Go/WebAssembly engine, as separate modules in code. Frames exist only in the `<video>` element and inside the worker; only numbers come back.

1. **Capture.** `getUserMedia` with `requestVideoFrameCallback`, using the frame capture time so that irregular frame intervals do not distort the spectrum.
2. **Vision.** In the sensor worker, MediaPipe Face Landmarker selects the forehead and cheeks; MediaPipe Pose Landmarker tracks the shoulders. Each frame becomes one numeric sample: timestamp, mean R, G, B of the skin region and a few quality metrics.
3. **Heart rate.** Samples are resampled to a uniform 30 Hz grid (a gap above 250 ms invalidates the window). The POS method (Wang et al., 2017) projects the normalised colour signal onto the plane orthogonal to skin tone, which removes changes in light intensity that affect all channels equally. The pulse signal is filtered to 0.7 to 3 Hz (42 to 180 bpm) and the dominant frequency of a 15 s window becomes the estimate, together with its signal-to-noise ratio. A peak sitting on the band edge is rejected instead of reported.
4. **Breathing.** The vertical position of the shoulders, normalised by shoulder width, is filtered to the breathing band; its derivative gives the phase and its spectrum gives the rate.
5. **Confidence.** Face, motion, light, talking, frame timing and SNR combine into a quality score and a confidence level (`high`, `medium`, `low`, `invalid`).

## Repository layout

```
engine/                 Go signal core, compiled to WebAssembly for the browser and natively for tests
  dsp/                  resampling, detrending, band-pass filter, FFT, spectral peak and SNR
  rppg/                 POS projection and heart-rate estimation
  resp/                 breathing rate and phase from shoulder movement
  quality/              signal quality, confidence level and reason
  session/              stateful session: 30 s ring buffer, combined estimate once per second
  cmd/wasm/             syscall/js entry point (push and estimate)
packages/vitals-web/    framework-agnostic TypeScript module: contract, simulated module, real module, sensor worker
apps/demo/              Angular demo app, production server with the exact CSP (serve.mjs), end-to-end tests (e2e/)
scripts/                WebAssembly build, pinned MediaPipe assets, fake camera video for the tests
docs/                   design decisions, exhibition checklist and AI-use statement
```

## Getting started

Requirements: Go 1.25 or newer, Node.js 24.15 or newer (Angular 22; `.nvmrc` says 24), `brotli` for the WebAssembly size check.

```bash
cd engine && go test ./...     # signal core
npm ci && npm test             # TypeScript module and demo app
npm run build                  # emits packages/vitals-web/dist and the demo bundle
```

## Running the demo

The demo is a production build served by a small Node server that applies the exact Content Security Policy. Chrome on a laptop is the only supported target.

```bash
npm ci
npm run build:wasm --workspace packages/vitals-web   # Go to WebAssembly, size check with brotli
./scripts/fetch-assets.sh                            # MediaPipe models and runtime, checked against assets.sha256
npm run build --workspace packages/vitals-web
npm run build --workspace apps/demo
node apps/demo/serve.mjs                             # http://127.0.0.1:4300/ (PORT overrides the port)
```

Open `http://127.0.0.1:4300/` in Chrome. Add `?mock=1` to use the simulated module (no camera needed), for interface work and as the fallback at the exhibition.

End-to-end tests use Chrome's fake camera (a synthetic video without a face):

```bash
node scripts/make-fake-video.mjs
npx playwright install chromium
npx playwright test --config apps/demo/playwright.config.ts
```

Notes on the setup:

- `apps/demo/vitals-assets` is a committed symlink to `packages/vitals-web/assets`; the Angular build copies it to `assets/vitals`.
- The toolchain is pinned exactly so that builds are repeatable: Angular and its build tooling at 22.2.3, `vite` 8.3.0 and `rolldown` 1.2.8 (root `devDependencies`), `playwright` 1.64.1, `@mediapipe/tasks-vision` 0.10.35.
- The exhibition laptop procedure is in [docs/exhibition-checklist.md](docs/exhibition-checklist.md).

## Using the module

The real camera pipeline and the simulated module share one interface, so the interface can be developed and tested before the camera pipeline is finished.

```ts
import { createMockVitals, type Estimate } from 'vitals-web';

const vitals = createMockVitals({ seed: 1 });
vitals.on('state', (s) => console.log('state', s));
vitals.on('estimate', (e: Estimate) => console.log(e.hrBpm, e.rrRpm, e.confidence, e.reason));
vitals.on('error', (e) => console.error(e.code));

await vitals.start({ video: document.querySelector('video')! });
// ...
vitals.stop();
console.log(vitals.summary());
```

One `Estimate` is emitted per second:

| Field | Meaning |
|---|---|
| `tMs` | end of the analysis window, in milliseconds |
| `hrBpm` | heart rate, or `null` when it cannot be measured |
| `rrRpm` | breathing rate, or `null` |
| `breathPhase` | `in`, `out`, `hold` or `null` |
| `quality` | 0 to 1 |
| `confidence` | `high`, `medium`, `low` or `invalid` |
| `reason` | why there is no value: `no_face`, `too_much_motion`, `low_light`, `talking`, `warming_up`, or `null` |
| `method` | `pos` or `chrom` |

The simulated module follows the same rules as the real one: 10 s of warm-up with no values, then values with a confidence, and periodic motion episodes with no value and a reason. `createMockVitals({ failWith: 'camera_denied' })` simulates a start failure.

## Privacy and ethics

- Video frames never leave the vision worker; only numbers do. Nothing is stored or sent over the network.
- No face recognition and no biometric template: the face is located, never identified.
- The camera starts only after explicit consent, with a visible indicator and a one-tap way out.
- Feedback is abstract by default; the number appears on request.
- Accuracy will be measured against a reference device and reported per skin tone (Monk scale) and lighting condition. Until then, the only evidence is the unit tests on synthetic signals.

## Validation plan

Recorded sessions keep only the numeric samples and estimates, never frames. Each session is compared with a reference device (finger pulse oximeter, optionally a chest strap) using mean absolute error, Bland-Altman limits of agreement and a bootstrap over participants, reported per skin-tone group and lighting condition.

## References

- W. Wang, A. C. den Brinker, S. Stuijk, G. de Haan. *Algorithmic Principles of Remote PPG.* IEEE Transactions on Biomedical Engineering 64(7), 2017.
- G. de Haan, V. Jeanne. *Robust Pulse Rate From Chrominance-Based rPPG.* IEEE Transactions on Biomedical Engineering 60(10), 2013.
- MediaPipe Tasks Vision: Face Landmarker and Pose Landmarker (Apache-2.0).

## AI use

The course allows AI tools with explicit acknowledgement. How they were used is described in [docs/ai-use.md](docs/ai-use.md).

## License

Decision pending among the authors. Until a license file is added, all rights are reserved.
