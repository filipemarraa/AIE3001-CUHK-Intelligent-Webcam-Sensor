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

Browsers deliver frames at irregular intervals. Samples are placed on their capture timestamps and linearly interpolated onto a 30 Hz grid before any filtering or FFT. A gap of up to 500 ms (a brief worker stall) is bridged by linear interpolation; a longer gap restarts the warm-up instead of being interpolated across. On the simulated stalls of 400 ms every 20 s, bridging kept the heart-rate coverage at 100% (it was 83%) and the breathing coverage at 100% (it was 67%), with the same error.

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

## 15. Every engine change is measured on a fixed benchmark

`engine/sim` generates sessions with a known heart rate (with drift and breathing-synchronous variation), breathing rate and phase, and adds the nuisances seen in the browser: frames dropped by a busy worker, stalls, flicker, exposure steps, motion, talking, a lost face, body sway and the heartbeat moving the shoulders. `go run ./cmd/validate` reports, per scenario, the coverage and error of heart rate and breathing, the share of confident readings that are wrong, and how often the breathing phase is called and right. A change to the engine enters only if this table does not get worse; recorded sessions against a reference device replace the simulation as soon as they exist, because the simulation is easy on heart rate by construction.

## 16. Breathing needs its own evidence

A breathing rate is reported only when the band-passed shoulder signal correlates with itself at the detected period (at least 0.5) and the spectral SNR is at least 3 dB. On the simulation, the spectral SNR alone could not separate breathing from noise (noise reached 15 dB, noisy breathing fell to 13.6 dB), while the periodicity could (noise at most 0.35, breathing at least 0.73). Before this, shoulders without any breathing produced a rate in every window, 9 breaths per minute off.

The phase comes from a sinusoid fitted at the detected rate over the last cycle, which works as a matched filter: faster motion such as the heartbeat moving the shoulders is rejected, and there is no ringing at the window edge. Turning points come out as `hold` through the hysteresis on that direction. A real pause is detected separately: the engine measures how long the shoulders have held still, on the signal before the detrend (what is left after a straight line and the heartbeat sinusoid, and the drift of that line, stay under 10% of the breathing amplitude or a few times the landmark noise floor, whichever is larger), fits the breathing on the data before that stretch, and calls a pause when a straight line explains the still stretch clearly better than that breathing continued would, in absolute terms and by a ratio of at least 3. A window that moves is never a pause. The heartbeat frequency, when it stands out in the shoulder signal, is modelled in every fit so it cannot tip the balance, and a pause already called holds while the stretch stays still for more than twice the minimum window.

Tested on a matrix of 6, 10, 15 and 24 breaths per minute, pauses starting at seven points of the cycle, heartbeat motion at 0.7 to 2 Hz and landmark noise: every pause is recognised within 3 s, and continuous breathing is never called a pause while it moves at more than half its peak speed. Known limits: strong heartbeat motion (a quarter of the breathing amplitude) together with landmark noise can delay a pause past 4 s, and noise can flip a single estimate inside a long pause; with strong periodic landmark noise (about a third of the breathing amplitude), the phase can stay on `hold` for a few seconds after a pause ends (a missed call, not a wrong one); and after a change of rhythm (for example from a natural 12 breaths per minute to the guided 6) the rate of the 30 s window lags, so the phase can be called against the breathing until the new rhythm dominates the window: up to 18.5 s after the change in our measurements (six starting phases, five changes of rhythm, one and ten estimates per second), never after 22 s. A local rate estimate for the phase is the next step. On the simulation, phase agreement rose from about 90% to 100% of the calls, and the phase is called in every clear moment of the scenarios without motion or speech.

## 17. Motion is a speed, and moving frames leave the heart-rate window

Landmark motion is divided by the time since the previous face frame, so the threshold (0.75 interocular distances per second, the old per-frame limit at about 15 fps) does not change when the frame rate changes. Frames above it are left out of the heart-rate window, and only the last 2 s of motion decide the `too_much_motion` reason. A burst of motion now costs the burst plus a fresh 10 s warm-up, instead of the burst plus a full 15 s window, and corrupted frames never enter the estimate. When the heart-rate extraction fails after the warm-up, the reason is `warming_up` instead of none, so no empty value is shown without a reason.

## 18. Breathing is not hidden by a missing pulse

The breathing rate and phase come from the shoulders, not from the skin colour, so they are reported whenever breathing passes its own test (decision 16) and nothing that also disturbs the shoulders is present: a face in view, no motion above the limit and no talking, checked on the measurements themselves rather than on the first reason, so talking in a dark room still hides them. A missing pulse (`warming_up`) or a dark room (`low_light`) no longer hides breathing. The module keeps the breathing the engine reports even when the confidence is `invalid` (the confidence describes the heart rate), and the summary averages every breathing rate reported. The demo already shows the measured phase in that case; its numbers panel still hides both numbers while the heart rate is invalid. On the benchmark, breathing coverage rose from 87% to 100% with a lost face and from 67% to 86% with motion bursts, and a new scenario with no pulse at all and steady breathing at 10 per minute keeps its breathing rate. The estimate also carries the heart-rate SNR for validation tools; the browser bridge is unchanged.

Known limit found by the same scenario: with no pulse in the skin colour, the heart rate is still reported, at low confidence, 30 bpm off on average. On the simulation, the SNR of noise (-6.3 to -0.4 dB, 5th to 95th percentile) touches that of a weak pulse on dark skin (-0.6 to 5.3 dB), and real cameras give lower SNR than the simulation, so a cut is set only from recorded sessions against a reference device.

## 19. Anatomical skin regions, behind an option until an A/B wins

The three boxes (forehead and two cheek squares) cover little skin and are axis-aligned: on a 148 px wide face they read 1,350 pixels, a tenth of the visible skin, and they collapse to almost nothing when the head tilts. The anatomical regions are polygons of the Face Landmarker mesh: the lower forehead (half way to the top landmarks, above the brows, so a fringe and the brows stay out), the glabella and both cheekbones, leaving out the lower face, which moves with speech and grows a beard. They were checked on the Face Landmarker output for a real face, where every region lies on skin and the mask holds 3.2 times the pixels of the boxes, and are tested on the MediaPipe canonical face: about three times the boxes, the same area with a 30° tilt, and the cheek on the far side left out past a 35° head turn (estimated from the depth of the face edges). A region below 20 pixels is dropped. The mask is one label image for the three channels and costs about 0.4 ms per frame.

More pixels are not more accuracy by themselves, so the regions stay behind `capture.skin`, with the boxes as default, until recordings on the lab page show a lower error against a reference device. The same applies to a lower face detection threshold (`capture.minFaceConfidence`): the detector stops finding faces narrower than about 74 px (around 0.9 m from a laptop camera), 68 px at 0.3, and a lower threshold must not report faces in an empty scene.

## Pending

- Decision pending: license for the repository (all authors must agree).
- Decision pending: minimum SNR (or periodicity) for reporting a heart rate, from the error against a reference device by SNR on recorded sessions.
