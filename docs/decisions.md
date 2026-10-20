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

## Pending

- Decision pending: license for the repository (all authors must agree).
