# Exhibition demo: design

Date: 2026-10-21. Target: working exhibition demo by 2026-11-18.

## Goal

A visitor sits at a laptop running Chrome, gives consent, and within 15 s sees their heart rate and breathing estimated from the webcam, with a confidence level, while a guided breathing exercise runs. Everything works offline, nothing is stored, and no frame leaves the browser.

### Decisions taken

| Topic | Decision |
|---|---|
| Success target | Exhibition demo with a real camera. The offline validation tool is out of scope for this cycle. |
| Device | Laptop + Chrome (desktop). Phones may work but are not supported or tested. |
| App framework | Angular (standalone components, signals). |
| Deadline | 4 weeks. CHROM, phone support and the validation tool are out of scope. |
| Execution model | One `sensor.worker` holding both vision (MediaPipe) and the Go/WASM engine, as separate modules in code. This replaces the two-worker diagram in the README. |

### Out of scope

CHROM (the `Method` type keeps `'chrom'` but the real module only reports `'pos'`), phone support, PWA, validation tool, participant studies with reference devices beyond the team's own sessions, licence choice (tracked separately in `docs/decisions.md`).

## 1. Components and data flow

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

### Go engine (`engine/`)

- `engine/session`: stateful session. Preallocated ring buffer of 20 s of samples. `Push(Sample)` and `Estimate(nowMs) Estimate`, which combines heart rate, breathing and confidence. No allocation per frame.
- `engine/rppg`: existing POS heart rate, unchanged in behaviour.
- `engine/resp` (new): vertical shoulder position normalised by shoulder width, band 0.1 to 0.5 Hz (6 to 30 breaths per minute). Rate from the spectrum (reusing `dsp`), phase `in` / `out` / `hold` from the derivative with hysteresis.
- `engine/quality` (new): face presence, motion, light, talking, frame-timing jitter and SNR combine into `quality` in [0, 1], a `confidence` and a `reason`. Reason priority is fixed: `no_face` > `too_much_motion` > `low_light` > `talking` > `warming_up`. When confidence is `invalid`, `hrBpm`, `rrRpm` and `breathPhase` are `null`.
- `engine/cmd/wasm`: thin `main` using `syscall/js`. Exports only `push` and `estimate`, exchanging `Float64Array` values, never JSON per frame.

The `Sample` passed to `push` is a fixed layout of numbers: timestamp, mean R, G, B of the skin region, face present, motion, light, mouth opening, shoulder y, shoulder width.

### TypeScript module (`packages/vitals-web`)

- `createVitals(options)` implements the existing `Vitals` interface. `createMockVitals` stays and remains interchangeable.
- `protocol.ts`: discriminated union of all worker ↔ main messages, imported by both sides.
- Errors map to the existing `VitalsErrorCode` values (`camera_denied`, `camera_unavailable`, `wasm_failed`, `vision_failed`, `unsupported_browser`).

### Efficiency

- At most one frame in flight. When the worker is busy, the new frame is closed and dropped, so no queue builds up.
- MediaPipe in `VIDEO` running mode with the GPU delegate and a CPU fallback.
- Face landmarks every frame; pose landmarks at 15 Hz, enough for breathing.
- The engine estimates once per second over the ring buffer; per-frame work is a copy into the buffer.

### Privacy boundary

Video frames exist only in the `<video>` element and inside `sensor.worker`. The worker never posts pixels. A test checks that every message from the worker is a type from `protocol.ts` and that no `ImageBitmap`, `VideoFrame` or image buffer is ever posted.

## 2. Demo app and user flow

`apps/demo/`: Angular with standalone components and signals, no state library. It uses only the public API of `vitals-web`.

1. **Welcome and consent.** Plain-language explanation of what is measured and what is not, and the "not a medical device" notice. The camera starts only on an explicit "Turn on camera" action.
2. **Calibration (about 10 s).** Video with a framing guide; the current `reason` is shown as a live instruction ("move closer", "more light", "hold still").
3. **Guided exercise (2 min by default).** A circle expands and contracts at the target rhythm (6 breaths per minute); the measured `breathPhase` is shown beside it. Feedback is abstract by default; heart and breathing rates appear behind a "show numbers" toggle.
4. **Summary.** `summary()` values: heart rate at start and end, mean breathing rate, mean quality. "Start again" button. Nothing is saved.

Always visible: a "camera on" indicator, an **Exit** button that stops the camera and clears state in one tap, and a transparency panel toggle (raw pulse signal, quality, confidence, method).

Exhibition mode: after 60 s without a face, or at the end of the summary, the app returns to the welcome screen and stops the camera, so a visitor never sees the previous visitor's data.

`?mock=1` switches to `createMockVitals`, for interface work before the camera pipeline is ready and as a fallback at the exhibition.

## 3. Security, code quality and CI

### Security and privacy

- **Content Security Policy** served by the demo server and set in a `<meta>` tag: `default-src 'self'; connect-src 'self'; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self' blob:; img-src 'self' blob: data:; style-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'`. No CDN.
- **Pinned assets.** MediaPipe models (`.task`) and WASM are copied into `assets/` at an exact version, with SHA-256 checksums verified at build time.
- **Secure context.** The demo is served from `localhost` as a production build.
- **No persistence and no network.** No `localStorage`, IndexedDB or analytics. A lint rule forbids `fetch`, `XMLHttpRequest`, `WebSocket`, `localStorage`, `sessionStorage` and `indexedDB` in `packages/vitals-web/src` and `apps/demo/src`, except the asset loader, which only reads same-origin paths.
- **Camera lifecycle.** Tracks are stopped on `stop()`, on `pagehide`, and on the exhibition-mode timeout. A test asserts `track.readyState === 'ended'`.
- **Dependencies.** Lockfile committed, `npm ci` only, `npm audit --omit=dev` and `govulncheck` in CI.

### Code quality

- Go: `gofmt`, `go vet`, `staticcheck`; small packages with one responsibility.
- TypeScript: `strict` and `noUncheckedIndexedAccess`, ESLint with typescript-eslint strict, Prettier. Angular ESLint for the app.
- Dependency direction: `apps/demo` → `vitals-web` public API → `engine.wasm`. Nothing points back; the app never imports module internals.
- Test-first, as stated in `docs/ai-use.md`.

### CI (GitHub Actions, on every pull request)

1. `go test -race ./...`, `go vet`, `staticcheck`, `govulncheck`.
2. WASM build; fails if the brotli size exceeds 1.5 MB.
3. `npm ci`, lint, typecheck, `vitest`.
4. Angular production build.
5. Playwright end-to-end test with Chrome's fake camera (`--use-fake-device-for-media-stream --use-file-for-fake-video-capture=<synthetic.y4m>`), checking camera → worker → `Estimate` and that `stop()` ends the tracks.

## 4. Testing, targets and schedule

### Tests per layer

| Layer | How |
|---|---|
| `engine/resp`, `engine/quality`, `engine/session` | Synthetic signals with a known answer, following `engine/rppg/synth_test.go`. |
| WASM bridge | Vitest in Node with `wasm_exec.js`: push 15 s of synthetic samples, expect the heart rate within ±2 bpm. |
| `createVitals` | Fake worker and fake camera: state transitions, error mapping, `stop()` releasing everything. |
| App | Component tests: consent gates the camera, Exit clears state, exhibition timeout returns to welcome. |
| End to end | Playwright with the fake camera. |
| Real use | Team sessions against a finger pulse oximeter; only numbers kept, under `data/` (gitignored). |

### Performance targets (laptop + Chrome)

- 30 fps capture with no visible stutter in the video.
- Mean worker cost per frame under 25 ms.
- JS to Go round trip under 2 ms.
- WASM under 1.5 MB with brotli.
- First valid `Estimate` within 15 s of consent.

The worker reports per-frame cost and dropped frames in the transparency panel, so the targets can be checked on the exhibition laptop.

### Definition of done

Ten consecutive team sessions on the exhibition laptop where every `Estimate` with confidence `high` or `medium` is within ±5 bpm of the oximeter, with no crash or freeze, and all CI jobs green.

### Schedule

| Week | Deliverable | Lead |
|---|---|---|
| 1 (21–28 Oct) | Foundation: CI, lint, WASM bridge and `engine/session`, MediaPipe-in-worker spike. Angular app on the mock (screens 1–4). | Tiago (engine), Vinicius (app), Filipe (CSP, consent) |
| 2 (28 Oct–4 Nov) | Full `sensor.worker`: camera → skin region → `Sample` → real heart rate. Real `createVitals`. End-to-end test with the fake camera. | Tiago, Filipe |
| 3 (4–11 Nov) | `engine/resp` and `engine/quality`. App on the real module. Transparency panel. Exhibition mode. | Tiago, Vinicius |
| 4 (11–18 Nov) | Hardening: performance on the exhibition laptop, oximeter sessions, user tests, README and `decisions.md` updated. | Filipe (user tests), all |

### Scope cuts if late, in this order

1. Simplified transparency panel (numbers only, no signal plot).
2. Breathing phase reduced to `in` / `out` (no `hold`).
3. Breathing rate without phase.

Heart rate and confidence are never cut.

## Documentation updates

- README: one-worker diagram, status table, how to run the demo.
- `docs/decisions.md`: new entry for the one-worker execution model and the stateful engine session.
