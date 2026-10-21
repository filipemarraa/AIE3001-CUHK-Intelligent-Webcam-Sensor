# Design decisions

Each entry records what was decided, why, and what would make us revisit it.

## 1. Processing stays on the device

Frames are read inside a vision worker and reduced to numbers (timestamp, mean colour of the skin region, quality metrics). Only those numbers reach the engine and the interface. Nothing is stored or sent over the network.

Why: physiological signals of an identifiable person are health data, and the system must work offline in an exhibition setting.

## 2. Signal core in Go, compiled to WebAssembly

The engine is a Go module. It compiles to WebAssembly for the browser and natively for unit tests and for the offline validation tool, so the code that is validated is the code that runs in the demo.

Spike criteria: compressed `.wasm` under 1.5 MB, JS to Go round trip under 2 ms per frame, 30 fps sustained on a mid-range phone browser.

Result so far: a WebAssembly build of the heart-rate path (Go 1.27, `-ldflags="-s -w"`) is 1.9 MB raw, 577 KB with gzip and 445 KB with brotli, so the size criterion is met without TinyGo. Latency and frame rate are still to be measured.

Fallback: if the latency or frame-rate criteria fail, the core is ported to TypeScript behind the same `Sample` and `Estimate` contract; only the engine worker changes.

## 3. POS as the default heart-rate method, CHROM as the alternative

POS (Wang et al., 2017) projects the temporally normalised RGB signal onto a plane orthogonal to the skin-tone direction. Intensity changes that affect all three channels equally (screen flicker, auto-exposure steps, small head movements toward or away from the light) cancel out. The test `TestPOSIgnoresLightFlickerThatFoolsGreenChannel` shows a case where the green channel alone locks onto a 108 bpm flicker and POS still returns the 72 bpm pulse.

## 4. Uniform resampling from frame timestamps

Browsers deliver frames at irregular intervals. Samples are placed on their capture timestamps and linearly interpolated onto a 30 Hz grid before any filtering or FFT. A gap above 250 ms invalidates the window instead of being interpolated across.

## 5. Heart-rate band of 0.7 to 3.0 Hz

42 to 180 bpm covers a seated person during a breathing exercise. Narrowing the upper edge from the common 4 Hz removes a range where motion and lighting noise dominate.

A peak is accepted only if it is a local maximum inside the band, so a rate just outside the band is rejected instead of being reported as the nearest edge value. Content far outside the band is attenuated by the filter and leaves only weak in-band peaks with low SNR, which the confidence level is designed to reject.

Known limitation: a heart rate below the band can appear as its first harmonic, at twice the true rate. The band assumes a seated adult at rest.

## 6. Spectral estimate with explicit SNR

The pulse signal is detrended, band-passed with a zero-phase Butterworth filter, Hann-windowed and zero-padded so that each bin is under 0.5 bpm wide, then the peak is refined by parabolic interpolation. The SNR compares the power around the peak and its first harmonic with the rest of the band. It feeds the confidence level.

## 7. Breathing from shoulder movement, not from the pulse signal

Breathing derived from chest and shoulder movement reaches 1 to 2 breaths per minute of error in controlled conditions, while breathing derived from the pulse signal is two to three times worse. The shoulders come from MediaPipe Pose Landmarker.

## 8. No value without confidence

Every estimate carries a quality score and a confidence level. When the confidence is `invalid` the value is `null` and a reason from a closed list is given. The system never fills a gap with a guess.

## 9. Out of scope by design

Blood pressure, SpO2, heart rate variability as a number, and any inference of emotion or stress. The evidence for camera-based measurement of these is weak or biased against darker skin, and emotion inference from faces is scientifically contested.

## 10. Framework-agnostic module with a simulated twin

`packages/vitals-web` has no framework dependency. The demo app consumes it, and so can any other web app. `createMockVitals` implements the same interface with synthetic data, so interface work does not wait for the camera pipeline.

## 11. One sensor worker for vision and engine

Face and pose landmarks (MediaPipe) and the Go/WebAssembly engine run in a single `sensor.worker`, as separate modules in code. The main thread transfers one `VideoFrame` at a time to the worker, which closes it after use; frames that arrive while the worker is busy are closed and counted as dropped, so no queue builds up. Only numbers come back: one `Estimate` per second and the diagnostics.

Why: with two workers, the numeric sample would cross a second boundary on every frame and the `VideoFrame` could not stay in one place. One worker keeps the privacy boundary simple (frames never leave it, and a test checks that every message posted from it is a type from `protocol.ts` without frames or buffers) and removes a hop per frame. This replaces the two-worker diagram of the first README.

Revisit if the mean cost per frame on the exhibition laptop stays above 25 ms and the cheaper options in `docs/exhibition-checklist.md` (pose every third frame, smaller detection draw, feeding the `VideoFrame` to MediaPipe directly) are not enough; splitting vision and engine would then be the next step.

## 12. Stateful engine session with a 30 s buffer

`engine/session` keeps a preallocated ring buffer of 30 s of samples. `Push` copies a sample in (no allocation per frame) and `Estimate(nowMs)` runs once per second over the buffer, combining heart rate (15 s window), breathing and confidence. The engine is stateful on purpose: the browser calls `push` per frame and `estimate` per second, with no JSON on the hot path.

The breathing band is 0.08 to 0.5 Hz instead of 0.1 to 0.5 Hz. The guided exercise runs at 6 breaths per minute, which is 0.1 Hz, so a 0.1 Hz band edge would put the target rhythm exactly on the edge, where a peak is rejected as a band-edge artefact (decision 5). Lowering the edge to 0.08 Hz keeps 6 rpm inside the band. A slower band needs a longer window to hold two periods, so the first breathing value appears after 25 s of samples (at least two periods of the lowest frequency, 2 / 0.08 Hz); the heart rate is available after the 10 s warm-up. The buffer is 30 s so that the 25 s breathing window and the 15 s heart-rate window both fit with margin.

Revisit if the team sessions show breathing values that lag too much for the exercise, or if the lower edge lets slow drift of the shoulders through.

## 13. MediaPipe inside a module worker

MediaPipe Tasks Vision runs inside the module worker with `FilesetResolver.forVisionTasks(base, true)`: the second argument selects the module loader. The classic loader fails in a module worker with "ModuleFactory not set.". The face and pose landmarkers use the GPU delegate and fall back to the CPU delegate when the GPU one cannot be created. The models and the WebAssembly runtime are pinned copies under `assets/` with SHA-256 checksums.

The Angular application builder only bundles a worker that is created from an entry of the app, so the app has its own `sensor.worker.ts` that imports `vitals-web/worker`, and passes it to `createVitals` as `createWorker`. The module therefore stays framework-agnostic and the app owns the bundling.

Revisit when MediaPipe fixes the classic loader in module workers, or if the GPU delegate misbehaves on the exhibition laptop (then fix the CPU delegate).

## 14. Content Security Policy, exact and without nonces

The policy is the same in the `<meta>` tag of `index.html` and in the header sent by `apps/demo/serve.mjs`: `default-src 'self'; connect-src 'self'; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self' blob:; img-src 'self' blob: data:; style-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'`. A test compares the header with this string.

There is no nonce and no `'unsafe-inline'`: scripts are external files, the components declare no `styles`, and the build does not inline critical CSS (`inlineCritical: false`), so `style-src 'self'` is enough. The one dynamic style, the breathing period of the circle, is set as a CSS custom property through the style object model, which the policy does not block. `'wasm-unsafe-eval'` is needed only for the Go and MediaPipe WebAssembly; `blob:` in `worker-src` and `img-src` and `data:` in `img-src` leave room for worker and image sources created at run time. The `<base href>` tag was removed because `base-uri 'none'` forbids it; the app is served from the root and uses no relative base.

`frame-ancestors` is ignored by browsers when it comes from a `<meta>` tag (Chrome logs a console message for it, which is expected); it is enforced only because the server also sends it in the header. The e2e test asserts that no `securitypolicyviolation` event fires during a measuring session and that every request goes to the app's own origin.

Revisit if a dependency needs inline styles or scripts (prefer changing the dependency's usage over loosening the policy), or if the demo is hosted behind another server (the header must be set there as well).

## Pending

- Decision pending: license for the repository (all authors must agree).
