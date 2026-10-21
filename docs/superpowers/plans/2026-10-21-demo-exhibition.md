# Exhibition Demo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A laptop + Chrome demo that measures heart rate and breathing from the webcam, with confidence, inside a guided breathing exercise, offline and without any frame leaving the browser.

**Architecture:** One `sensor.worker` receives `VideoFrame`s from the main thread, runs MediaPipe Face/Pose Landmarker, reduces each frame to a numeric `Sample`, and pushes it into a stateful Go engine compiled to WebAssembly; once per second the engine returns an `Estimate`. `packages/vitals-web` exposes `createVitals()` with the same `Vitals` interface as the existing `createMockVitals()`; the Angular app in `apps/demo` uses only that public API.

**Tech Stack:** Go 1.25+ (`GOOS=js GOARCH=wasm`), TypeScript 5.9 strict, Vitest 4, `@mediapipe/tasks-vision` (exact 0.10.x pinned at install), Angular (current stable major, standalone + signals), Playwright, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-21-demo-exhibition-design.md`

## Global Constraints

- Target: laptop + Chrome desktop only. No phone, PWA, CHROM, or validation tool work.
- No frame, pixel buffer, `ImageBitmap` or `VideoFrame` is ever posted out of `sensor.worker`.
- No network and no storage: `fetch`, `XMLHttpRequest`, `WebSocket`, `localStorage`, `sessionStorage`, `indexedDB` are banned in `packages/vitals-web/src` and `apps/demo/src`, except `packages/vitals-web/src/worker/assets.ts`, which only loads same-origin asset paths.
- CSP (exact): `default-src 'self'; connect-src 'self'; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self' blob:; img-src 'self' blob: data:; style-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'`.
- Heart-rate band 0.7–3.0 Hz, grid 30 Hz, max gap 250 ms (existing `rppg.DefaultConfig()`); breathing band 0.1–0.5 Hz.
- When `confidence` is `invalid`, `hrBpm`, `rrRpm` and `breathPhase` are `null` and `reason` is set. Reason priority: `no_face` > `too_much_motion` > `low_light` > `talking` > `warming_up`.
- Warm-up 10 s; first valid `Estimate` within 15 s of consent.
- WASM size under 1.5 MB with brotli; mean worker cost per frame under 25 ms; JS→Go round trip under 2 ms.
- Real module reports `method: 'pos'` always.
- Test first for every function (`docs/ai-use.md`). Commit messages follow the repo style: `<Area> - <what>` in Portuguese, with no co-author trailer.
- Lockfile committed; CI uses `npm ci`.

## Review Focus

1. **Camera disconnected mid-session** (USB webcam pulled, track `ended`): the module emits `error` `camera_unavailable`, goes to `stopped`, and the app shows the welcome screen. Test in Task 8.
2. **Tab hidden or frames paused** (rVFC stops, then resumes): the gap above 250 ms invalidates the heart-rate window; estimates return `invalid` with a reason, then recover once 10 s of contiguous samples exist again. Test in Task 4.
3. **Two people in front of the camera**: Face Landmarker runs with `numFaces: 1`; the second person never mixes into the skin region. Test in Task 6 (`roiBoxes` uses only the first face) and config assertion in Task 7.
4. **`start()` twice, or `stop()` while `start()` is still resolving**: no second camera stream, no orphaned worker, tracks end. Test in Task 8.
5. **Dark or back-lit room**: `low_light` with `invalid`, never a heart-rate number from a near-black skin region. Test in Task 2 (threshold) and Task 4 (session with dark samples).

---

## File Structure

```
.github/workflows/ci.yml                     CI: go, ts, wasm size, app build, e2e
eslint.config.js, .prettierrc.json           shared TS lint/format (root)
scripts/build-wasm.sh                        go build → packages/vitals-web/assets/engine.wasm + wasm_exec.js, brotli size gate
scripts/fetch-assets.sh, assets.sha256       pinned MediaPipe models + wasm, checksum verify

engine/quality/quality.go(+_test)            Assess(Inputs) → Quality, Confidence, Reason
engine/resp/resp.go(+_test)                  breathing rate + phase from shoulders
engine/session/sample.go                     Sample, flat field layout
engine/session/session.go(+_test)            ring buffer, Push, Estimate, Reset
engine/session/encode.go(+_test)             Estimate ↔ []float64 layout shared with TS
engine/cmd/wasm/main.go                      syscall/js exports: push, estimate, reset

packages/vitals-web/src/
  types.ts                                   + Diagnostics event
  engine/layout.ts                           field indices mirrored from Go
  engine/decode.ts(+test)                    Float64Array → Estimate
  engine/engine.ts(+test)                    load wasm_exec + engine.wasm, typed wrapper
  worker/protocol.ts                         main ↔ worker messages (discriminated union)
  worker/vision/roi.ts(+test)                skin boxes from face landmarks, mean RGB + luma
  worker/vision/metrics.ts(+test)            motion, mouth opening, shoulders
  worker/assets.ts                           same-origin asset URLs (only file allowed to fetch)
  worker/sensor.worker.ts                    MediaPipe + engine loop
  camera.ts(+test)                           getUserMedia, error mapping, frame loop with backpressure
  vitals.ts(+test)                           createVitals: state machine, events, summary
  mock.ts                                    + diagnostics event
  index.ts                                   + createVitals export

apps/demo/                                   Angular app
  serve.mjs                                  static server with CSP header + wasm MIME
  src/app/vitals.service.ts                  chooses real or mock (?mock=1), exposes signals
  src/app/screens/{welcome,calibration,exercise,summary}/
  src/app/shell/                             camera indicator, Exit, transparency panel
  e2e/demo.spec.ts                           Playwright with fake camera
```

---

### Task 1: Tooling baseline and CI

**Files:**
- Create: `eslint.config.js`, `.prettierrc.json`, `.github/workflows/ci.yml`, `engine/tools.md` (one line: how to install `staticcheck` and `govulncheck`)
- Modify: `package.json` (root devDependencies + `lint`, `format:check` scripts), `packages/vitals-web/package.json` (`lint` script)
- Test: `packages/vitals-web/src/lint-rules.test.ts`

**Interfaces:**
- Produces: `npm run lint`, `npm run format:check`, CI jobs `go` and `ts` that later tasks extend.

- [ ] **Step 1: Install and confirm the current baseline**

Run: `npm install && npm test && (cd engine && go test ./...)`
Expected: all pass (vitest 1 file, Go `dsp` and `rppg` ok). Commit `package-lock.json` changes if any.

- [ ] **Step 2: Write the failing lint-rule test**

`lint-rules.test.ts` runs ESLint programmatically (`new ESLint({ overrideConfigFile: '<root>/eslint.config.js' })`) on in-memory source with `lintText(code, { filePath: 'packages/vitals-web/src/x.ts' })` and asserts:
```ts
expect(errorsFor("fetch('/x')")).toContain('no-restricted-globals');
expect(errorsFor('localStorage.setItem("a","b")')).toContain('no-restricted-globals');
expect(errorsFor('new WebSocket("ws://x")')).toContain('no-restricted-globals');
expect(errorsFor('indexedDB.open("x")')).toContain('no-restricted-globals');
expect(errorsFor("fetch('/x')", 'packages/vitals-web/src/worker/assets.ts')).toEqual([]);
```

- [ ] **Step 3: Run it, expect FAIL** (`eslint.config.js` missing)

Run: `npx vitest run packages/vitals-web/src/lint-rules.test.ts`

- [ ] **Step 4: Add the config**

`eslint.config.js`: typescript-eslint `strictTypeChecked` for `packages/**/src/**/*.ts` and `apps/**/src/**/*.ts`; `no-restricted-globals` for `fetch`, `XMLHttpRequest`, `WebSocket`, `EventSource`, `localStorage`, `sessionStorage`, `indexedDB`, with message "Network and storage are banned (spec §3)"; an override that turns it off for `packages/vitals-web/src/worker/assets.ts` only. Prettier: `singleQuote: true`, `printWidth: 120` (matches existing code). Fix existing lint findings in `mock.ts` if any, without behaviour changes.

- [ ] **Step 5: Run lint test and full lint, expect PASS**

Run: `npx vitest run packages/vitals-web/src/lint-rules.test.ts && npm run lint && npm run format:check`

- [ ] **Step 6: CI workflow**

`ci.yml` on `pull_request` and push to `main`:
- job `go` (`actions/setup-go` with `go-version-file: engine/go.mod`): `gofmt -l .` must print nothing, `go vet ./...`, `staticcheck ./...`, `govulncheck ./...`, `go test -race ./...`, all in `engine/`.
- job `ts` (Node 22): `npm ci`, `npm run lint`, `npm run format:check`, `npm run typecheck`, `npm test`, `npm audit --omit=dev --audit-level=high`.

Run locally: `cd engine && gofmt -l . && go vet ./... && go test -race ./...`
Expected: no gofmt output, vet clean, tests pass.

- [ ] **Step 7: Commit** — `Infra - Lint, formatação e CI para Go e TypeScript`

---

### Task 2: `engine/quality`

**Files:**
- Create: `engine/quality/quality.go`, `engine/quality/quality_test.go`

**Interfaces:**
- Produces:
```go
type Confidence int // ConfidenceHigh=0, ConfidenceMedium=1, ConfidenceLow=2, ConfidenceInvalid=3; String() → "high"|"medium"|"low"|"invalid"
type Reason int     // ReasonNone=0, ReasonNoFace=1, ReasonMotion=2, ReasonLowLight=3, ReasonTalking=4, ReasonWarmingUp=5; String() → ""|"no_face"|"too_much_motion"|"low_light"|"talking"|"warming_up"
type Inputs struct {
    FaceFraction float64 // fraction of samples in the window with a face, 0..1
    MotionP90    float64 // 90th percentile of per-frame landmark motion, in inter-ocular distances
    LightMean    float64 // mean luma of the skin region, 0..255
    MouthStd     float64 // std of mouth opening (fraction of face height) over the last 2 s
    JitterMs     float64 // std of frame intervals in the window
    SNRdB        float64
    HasHR        bool    // heart-rate estimator returned a value
    WarmingUp    bool
}
type Result struct { Quality float64; Confidence Confidence; Reason Reason }
func Assess(in Inputs) Result
```

All thresholds live as named constants at the top of `quality.go` (initial values below, tuned in Task 11):
`minFaceFraction = 0.8`, `maxMotionP90 = 0.05`, `minLight = 40`, `goodLight = 120`, `maxMouthStd = 0.03`, `maxJitterMs = 20`, `goodSNRdB = 10`, `highQuality = 0.75`, `mediumQuality = 0.6` (the last two match `mock.ts`).

- [ ] **Step 1: Write failing tests**

Table test `TestAssessReasonPriority` with a `good` baseline (`FaceFraction 1, MotionP90 0.01, LightMean 130, MouthStd 0.005, JitterMs 3, SNRdB 12, HasHR true`) and cases:
- `FaceFraction 0.5` + `MotionP90 0.2` → `ReasonNoFace`, `ConfidenceInvalid`
- `MotionP90 0.2` + `LightMean 10` → `ReasonMotion`
- `LightMean 10` + `MouthStd 0.1` → `ReasonLowLight`
- `MouthStd 0.1` + `WarmingUp` → `ReasonTalking`
- `WarmingUp` → `ReasonWarmingUp`, `ConfidenceInvalid`
- `HasHR false`, nothing else wrong → `ConfidenceInvalid`, `ReasonNone`

`TestAssessGoodSignalIsHigh`: baseline → `ConfidenceHigh`, `Quality >= 0.75`, `ReasonNone`.
`TestAssessLowSNRLowersConfidence`: baseline with `SNRdB 2` → confidence `ConfidenceLow` or `ConfidenceMedium`, never `High`.
`TestAssessQualityInUnitRange`: for a grid of inputs (including negative SNR and `LightMean 255`), `0 <= Quality <= 1`.
`TestStringsMatchTypeScript`: every `Confidence` and `Reason` `String()` equals the literal in `packages/vitals-web/src/types.ts`.

- [ ] **Step 2: Run, expect FAIL** — `cd engine && go test ./quality/` → `undefined: Assess`

- [ ] **Step 3: Implement `Assess`**

Reason = first failing check in priority order. Quality = `0.4*snr + 0.2*motion + 0.2*light + 0.2*jitter`, each sub-score clamped to [0,1]: `snr = SNRdB/goodSNRdB`, `motion = 1 - MotionP90/maxMotionP90`, `light = (LightMean-minLight)/(goodLight-minLight)`, `jitter = 1 - JitterMs/maxJitterMs`. Confidence `invalid` if any reason or `!HasHR`; else by `highQuality` / `mediumQuality`.

- [ ] **Step 4: Run, expect PASS** — `go test ./quality/`

- [ ] **Step 5: Commit** — `Engine - Qualidade do sinal, confiança e motivo`

---

### Task 3: `engine/resp`

**Files:**
- Create: `engine/resp/resp.go`, `engine/resp/resp_test.go`

**Interfaces:**
- Consumes: `dsp.Resample`, `dsp.Detrend`, `dsp.Bandpass`, `dsp.DominantFrequency`.
- Produces:
```go
type Phase int // PhaseUnknown=0, PhaseIn=1, PhaseOut=2, PhaseHold=3; String() → ""|"in"|"out"|"hold"
type Config struct { SampleRateHz, MaxGapMs, LowHz, HighHz, MinDurationS, PhaseWindowS, EnterThreshold, ExitThreshold float64 }
func DefaultConfig() Config // 10, 250, 0.1, 0.5, 20, 0.5, 0.2, 0.1
type Breath struct { RPM float64; SNRdB float64; Phase Phase }
var ErrTooShort = errors.New("resp: window shorter than the minimum duration")
func Estimate(tsMs, shoulderY, shoulderW []float64, prev Phase, cfg Config) (Breath, error)
```

Signal: `v = -shoulderY/shoulderW` (image y grows downward, so inhaling raises the shoulders and increases `v`). Samples with `shoulderW <= 0` are rejected with `ErrTooShort`-style error `ErrNoShoulders`.

- [ ] **Step 1: Write failing tests** (add a `synthShoulders(rpm, durationS, fps, jitterMs, noise, seed)` helper in `resp_test.go`, modelled on `engine/rppg/synth_test.go`: `y = 0.6 - 0.004*sin(2π f t)`, `w = 0.3`)

- `TestEstimateRate`: for `rpm ∈ {6, 10, 15, 24}`, 25 s at 30 fps, jitter 8 ms → `|RPM - rpm| <= 1`.
- `TestEstimateTooShort`: 15 s → `ErrTooShort`.
- `TestPhaseFollowsSignal`: 6 rpm, window ending at the rising zero-crossing (`t = 25 s` → phase of `sin` is 0, rising) → `PhaseIn`; window ending a half period later → `PhaseOut`.
- `TestPhaseHoldWhenFlat`: 25 s of 6 rpm followed by 3 s of constant `y` → `PhaseHold`.
- `TestPhaseHysteresis`: with `prev = PhaseIn` and a normalised derivative of `0.15` (between exit and enter thresholds), phase stays `PhaseIn`; with `prev = PhaseHold`, same input stays `PhaseHold`.

- [ ] **Step 2: Run, expect FAIL** — `go test ./resp/`

- [ ] **Step 3: Implement `Estimate`**

Resample `v` onto `SampleRateHz` (10 Hz), detrend, band-pass, `DominantFrequency` → `RPM = 60*FreqHz`. Phase: slope of a least-squares line over the last `PhaseWindowS` of the filtered signal, divided by the filtered signal's standard deviation (units: std per second). Enter `in`/`out` when `|d| > EnterThreshold`; leave the current `in`/`out` for `hold` only when `|d| < ExitThreshold`; otherwise keep `prev`.

- [ ] **Step 4: Run, expect PASS** — `go test ./resp/`

- [ ] **Step 5: Commit** — `Engine - Frequência e fase respiratória pelos ombros`

---

### Task 4: `engine/session`

**Files:**
- Create: `engine/session/sample.go`, `engine/session/session.go`, `engine/session/encode.go`, `engine/session/session_test.go`, `engine/session/encode_test.go`

**Interfaces:**
- Consumes: `rppg.EstimateHeartRate`, `rppg.DefaultConfig`, `resp.Estimate`, `resp.DefaultConfig`, `quality.Assess`.
- Produces:
```go
// Flat layout of one sample, shared with packages/vitals-web/src/engine/layout.ts.
const ( FieldTimeMs = iota; FieldR; FieldG; FieldB; FieldFace; FieldMotion; FieldLight; FieldMouth; FieldShoulderY; FieldShoulderW; SampleLen )
type Sample struct { TimeMs, R, G, B float64; Face bool; Motion, Light, Mouth, ShoulderY, ShoulderW float64 }
func SampleFromSlice(x []float64) (Sample, error) // len(x) == SampleLen, Face = x[FieldFace] != 0

type Estimate struct {
    TimeMs float64
    HRBpm, RRRpm float64 // NaN when absent
    Phase resp.Phase
    Quality float64
    Confidence quality.Confidence
    Reason quality.Reason
    Pulse []float64 // last 5 s of band-passed pulse at 30 Hz (≤150 values), for the transparency panel
}
type Config struct { BufferS, WarmupS, HRWindowS float64; RPPG rppg.Config; Resp resp.Config }
func DefaultConfig() Config // BufferS 30, WarmupS 10, HRWindowS 15
func New(cfg Config) *Session
func (s *Session) Push(x Sample)
func (s *Session) Estimate(nowMs float64) Estimate
func (s *Session) Reset()

// encode.go: output layout, shared with packages/vitals-web/src/engine/layout.ts.
const ( OutTimeMs = iota; OutHR; OutRR; OutPhase; OutQuality; OutConfidence; OutReason; OutPulseLen; OutPulse ) // OutPulse is the start of the pulse values
const OutMaxLen = OutPulse + 150
func Encode(e Estimate, out []float64) int // returns number of values written
```

Note: the spec says a 20 s buffer; the breathing estimator needs 20 s at 0.1 Hz (`dsp.DominantFrequency` requires `2*fs/lowHz` samples), so the buffer is 30 s.

- [ ] **Step 1: Write failing tests** (reuse the synth idea: build a test helper `synthSamples(bpm, rpm, durationS, fps)` producing `Sample`s with the rPPG colour model of `engine/rppg/synth_test.go`, shoulders from Task 3, `Face true, Motion 0.01, Light 130, Mouth 0.02`)

- `TestWarmupInvalid`: push 5 s, `Estimate(5000)` → `ConfidenceInvalid`, `ReasonWarmingUp`, `HRBpm` NaN.
- `TestHeartRateAfterWarmup`: push 15 s at 72 bpm → `|HRBpm-72| <= 2`, confidence not invalid.
- `TestBreathingAfter20s`: push 25 s at 72 bpm / 6 rpm → `|RRRpm-6| <= 1`, `Phase != PhaseUnknown`; at 15 s `RRRpm` is NaN while `HRBpm` is valid.
- `TestNoFaceInvalidates`: last 10 s with `Face false` → `ReasonNoFace`, all values NaN, `Phase == PhaseUnknown`.
- `TestDarkRoom` (Review Focus 5): `Light 15` and colour DC scaled to `{12, 8, 6}` → `ReasonLowLight`, `HRBpm` NaN.
- `TestGapInvalidatesThenRecovers` (Review Focus 2): 15 s, then a 2 s gap, then 5 s → `ConfidenceInvalid`; after 11 more contiguous seconds → valid heart rate again.
- `TestRingBufferBounded`: push 120 s; `testing.AllocsPerRun(100, func(){ s.Push(x) }) == 0`.
- `TestResetClears`: after `Reset()`, `Estimate` → `ReasonWarmingUp`.
- `encode_test.go` `TestEncodeLayout`: encode a known `Estimate`; check each `Out*` index, phase/confidence/reason as their int codes, NaN preserved, `OutPulseLen` equals `len(Pulse)`.

- [ ] **Step 2: Run, expect FAIL** — `go test ./session/`

- [ ] **Step 3: Implement**

Preallocated circular arrays of `BufferS * 60` capacity (headroom for 60 fps cameras) per field; `Push` copies values and advances the head. `Estimate`: copy the window in time order into preallocated scratch slices; heart rate on the last `HRWindowS`; breathing on the whole buffer when it spans ≥ `resp.MinDurationS`; compute `quality.Inputs` from the window (face fraction, motion p90, light mean, mouth std over last 2 s, frame interval std, SNR from the heart-rate result; `WarmingUp` when the contiguous span is < `WarmupS`); apply `quality.Assess`; when invalid, set values to NaN and phase to unknown. Keep the last phase for hysteresis. Heart-rate or resampling errors (gap, too short) yield `HasHR false`; a gap resets the contiguous span used for `WarmingUp`.

- [ ] **Step 4: Run, expect PASS** — `go test -race ./session/`

- [ ] **Step 5: Commit** — `Engine - Sessão com buffer circular e estimativa combinada`

---

### Task 5: WASM bridge and TypeScript engine wrapper

**Files:**
- Create: `engine/cmd/wasm/main.go`, `scripts/build-wasm.sh`, `packages/vitals-web/src/engine/layout.ts`, `packages/vitals-web/src/engine/decode.ts`, `packages/vitals-web/src/engine/decode.test.ts`, `packages/vitals-web/src/engine/engine.ts`, `packages/vitals-web/src/engine/engine.test.ts`
- Modify: `packages/vitals-web/package.json` (`build:wasm` script, `assets/` in `files`), `.gitignore` (`packages/vitals-web/assets/engine.wasm`, `wasm_exec.js` are build outputs), `.github/workflows/ci.yml` (job `wasm`)

**Interfaces:**
- Consumes: `session.New`, `Push`, `Estimate`, `Reset`, `SampleFromSlice`, `Encode`, `OutMaxLen`, field constants.
- Produces (TS):
```ts
// layout.ts — must match engine/session/sample.go and encode.go
export const SAMPLE = { timeMs: 0, r: 1, g: 2, b: 3, face: 4, motion: 5, light: 6, mouth: 7, shoulderY: 8, shoulderW: 9, length: 10 } as const;
export const OUT = { timeMs: 0, hr: 1, rr: 2, phase: 3, quality: 4, confidence: 5, reason: 6, pulseLen: 7, pulse: 8, maxLength: 158 } as const;
// decode.ts
export function decodeEstimate(out: Float64Array): { estimate: Estimate; pulse: Float64Array };
// engine.ts
export interface Engine { push(sample: Float64Array): void; estimate(nowMs: number): { estimate: Estimate; pulse: Float64Array }; reset(): void; }
export async function loadEngine(wasmUrl: string, wasmExecUrl: string): Promise<Engine>;
```
- Go exports on `globalThis.vitalsEngine`: `push(u8: Uint8Array)`, `estimate(nowMs: number, outU8: Uint8Array): number` (count of float64 written), `reset()`. Byte views over `Float64Array` buffers, copied with `js.CopyBytesToGo` / `js.CopyBytesToJS`, little-endian; no JSON.

- [ ] **Step 1: Write failing decode test**

`decode.test.ts`: build a `Float64Array(OUT.maxLength)` by hand (`hr = 72.4`, `rr = NaN`, `phase = 1`, `quality = 0.8`, `confidence = 0`, `reason = 0`, `pulseLen = 3`) → `estimate` equals `{ tMs, hrBpm: 72.4, rrRpm: null, breathPhase: 'in', quality: 0.8, confidence: 'high', reason: null, method: 'pos' }`, `pulse.length === 3`. Second case: `confidence = 3, reason = 2` → `confidence: 'invalid', reason: 'too_much_motion'`, `breathPhase: null`.

- [ ] **Step 2: Run, expect FAIL**; implement `layout.ts` + `decodeEstimate`; run, expect PASS.

Run: `npx vitest run packages/vitals-web/src/engine/decode.test.ts`

- [ ] **Step 3: Write failing engine test** (Node, real WASM)

`engine.test.ts` (`// @vitest-environment node`, skipped with a clear message only if `assets/engine.wasm` is absent — CI always builds it first): load via `loadEngine` with file URLs; push 15 s of synthetic samples at 72 bpm (port the colour model from `engine/rppg/synth_test.go` into a `packages/vitals-web/src/test/synth.ts` helper) → `|hrBpm - 72| <= 2`. Measure 1000 `push` calls: mean under 0.05 ms; `estimate` mean under 2 ms (spec round-trip target).

- [ ] **Step 4: Implement `main.go`, `build-wasm.sh`, `loadEngine`**

`main.go`: one `session.New(session.DefaultConfig())`, preallocated `[]byte` scratch for one sample and for `OutMaxLen` floats, register the three functions, block on `select {}`.
`build-wasm.sh`: `GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o packages/vitals-web/assets/engine.wasm ./cmd/wasm` from `engine/`; copy `$(go env GOROOT)/lib/wasm/wasm_exec.js` (fallback `misc/wasm/wasm_exec.js`); print raw and brotli sizes; exit 1 if `brotli -c engine.wasm | wc -c` > 1572864.
`loadEngine`: in Node use `fs` (test-only path guarded by `typeof process`), in the worker use the asset loader from Task 7 — for now accept an `instantiate: (url) => Promise<WebAssembly.WebAssemblyInstantiatedSource>` parameter defaulting to `WebAssembly.instantiateStreaming(fetch(url), go.importObject)` placed in `worker/assets.ts` (the one file allowed to `fetch`).

- [ ] **Step 5: Run, expect PASS**

Run: `./scripts/build-wasm.sh && npx vitest run packages/vitals-web/src/engine/`
Expected: size line under 1.5 MB brotli; tests pass.

- [ ] **Step 6: CI job `wasm`**: setup Go + Node, `./scripts/build-wasm.sh`, `npm ci`, `npx vitest run packages/vitals-web/src/engine/`. Make job `ts` depend on `wasm` artifacts (upload/download `packages/vitals-web/assets/`).

- [ ] **Step 7: Commit** — `Engine - Ponte WebAssembly e wrapper TypeScript`

---

### Task 6: Worker protocol and vision helpers (pure)

**Files:**
- Create: `packages/vitals-web/src/worker/protocol.ts`, `packages/vitals-web/src/worker/vision/roi.ts`, `roi.test.ts`, `packages/vitals-web/src/worker/vision/metrics.ts`, `metrics.test.ts`
- Modify: `packages/vitals-web/src/types.ts` (add `Diagnostics`, extend `VitalsEvents`)

**Interfaces:**
- Produces:
```ts
// types.ts
export interface Diagnostics { frameCostMs: number; droppedFrames: number; pulse: number[] }
// VitalsEvents gains: diagnostics: Diagnostics
// protocol.ts
export type ToWorker =
  | { type: 'init'; assetsBaseUrl: string }
  | { type: 'frame'; frame: VideoFrame; tMs: number }
  | { type: 'reset' };
export type FromWorker =
  | { type: 'ready' }
  | { type: 'frameDone' }
  | { type: 'estimate'; estimate: Estimate }
  | { type: 'diagnostics'; diagnostics: Diagnostics }
  | { type: 'error'; code: 'wasm_failed' | 'vision_failed'; message: string };
export function isFromWorker(x: unknown): x is FromWorker; // checks type tag and that no value is ImageBitmap, VideoFrame, ImageData, ArrayBuffer or typed array
// roi.ts  (landmarks normalised 0..1, MediaPipe Face Mesh indices)
export interface Point { x: number; y: number }
export interface Box { x: number; y: number; w: number; h: number } // pixels
export function roiBoxes(faces: Point[][], width: number, height: number): Box[]; // uses faces[0] only; [] when none
export function meanRgb(pixels: Uint8ClampedArray, width: number, boxes: Box[]): { r: number; g: number; b: number; luma: number } | null;
// metrics.ts
export function motion(prev: Point[] | null, curr: Point[]): number;      // mean displacement / inter-ocular distance (landmarks 33, 263)
export function mouthOpening(face: Point[]): number;                       // dist(13,14) / dist(10,152)
export function shoulders(pose: Point[] | null): { y: number; w: number } | null; // landmarks 11, 12: mean y, |x11 - x12|
```
Skin boxes: forehead between x of landmarks 109 and 338, from y of landmark 10 to y of landmark 151; one cheek box centred on landmark 50 and one on 280, side `0.12 ×` face width (`|x234 - x454|`). Boxes are clipped to the frame.

- [ ] **Step 1: Write failing tests**

- `roi.test.ts`: synthetic 478-point face with known coordinates → three boxes at expected pixel positions; `roiBoxes([faceA, faceB], …)` equals `roiBoxes([faceA], …)` (Review Focus 3); `roiBoxes([], …)` → `[]`; a face near the edge gives clipped boxes with `w, h > 0`. `meanRgb` on a 4×4 buffer with a red box region → `{ r: 255, g: 0, b: 0 }`, `luma ≈ 76.2` (Rec. 601: `0.299 r + 0.587 g + 0.114 b`); empty boxes → `null`.
- `metrics.test.ts`: identical landmarks → `motion = 0`; all points shifted by one inter-ocular distance → `motion = 1`; `prev = null` → `0`. Mouth closed (13 = 14) → `0`. `shoulders(null)` → `null`; known points → exact `y`, `w`.
- `protocol` test in `metrics.test.ts` or own file: `isFromWorker({ type: 'estimate', estimate })` true; with an added `new Uint8Array(4)` field → false; unknown `type` → false.

- [ ] **Step 2: Run, expect FAIL**; **Step 3: implement**; **Step 4: run, expect PASS**

Run: `npx vitest run packages/vitals-web/src/worker/`

- [ ] **Step 5: Commit** — `Vitals-web - Protocolo do worker e funções de visão`

---

### Task 7: Sensor worker with MediaPipe and pinned assets

**Files:**
- Create: `packages/vitals-web/src/worker/assets.ts`, `packages/vitals-web/src/worker/sensor.worker.ts`, `packages/vitals-web/src/worker/sensor.test.ts`, `scripts/fetch-assets.sh`, `assets.sha256`
- Modify: `packages/vitals-web/package.json` (dependency `@mediapipe/tasks-vision` exact version, `assets` script), `.gitignore` (downloaded models under `packages/vitals-web/assets/mediapipe/`)

**Interfaces:**
- Consumes: Task 5 `loadEngine`, `SAMPLE`; Task 6 `roiBoxes`, `meanRgb`, `motion`, `mouthOpening`, `shoulders`, `ToWorker`, `FromWorker`.
- Produces: worker script `sensor.worker.ts` speaking `protocol.ts`. Assets served from `${assetsBaseUrl}/`: `engine.wasm`, `wasm_exec.js`, `mediapipe/wasm/*` (copied from `node_modules/@mediapipe/tasks-vision/wasm`), `mediapipe/face_landmarker.task`, `mediapipe/pose_landmarker_lite.task`.
- Testable core exported from the worker module: `createSensor(deps: { face: FaceDetector; pose: PoseDetector; engine: Engine; post: (m: FromWorker) => void; now: () => number }): { onFrame(source: CanvasImageSource & { close?(): void }, tMs: number, width: number, height: number, pixels: (boxes: Box[]) => Uint8ClampedArray): void; reset(): void }` with `FaceDetector = { detect(img, tMs): Point[][] }`, `PoseDetector = { detect(img, tMs): Point[] | null }`.

- [ ] **Step 1: Spike (time-boxed, 2 h): MediaPipe in a module worker on Chrome**

Minimal page + worker that creates `FaceLandmarker` with `FilesetResolver.forVisionTasks('<local>/mediapipe/wasm')`, `runningMode: 'VIDEO'`, `delegate: 'GPU'`, `numFaces: 1`, and calls `detectForVideo(offscreenCanvas, tMs)`. Record in the task's commit message: works in `{ type: 'module' }` worker yes/no, GPU yes/no, ms per frame at 640×480. If module workers fail, use a classic worker built as a separate IIFE bundle (`esbuild --format=iife`) and note it in `docs/decisions.md` in Task 11. Delete the spike files.

- [ ] **Step 2: `fetch-assets.sh` + checksums**

Downloads `face_landmarker.task` and `pose_landmarker_lite.task` from `https://storage.googleapis.com/mediapipe-models/` (float16 / latest paths), copies the tasks-vision `wasm/` folder, then `shasum -a 256 -c assets.sha256`. First run writes `assets.sha256` (`--update` flag); later runs and CI verify and fail on mismatch.

Run: `./scripts/fetch-assets.sh --update && ./scripts/fetch-assets.sh`
Expected: second run prints `OK` for every file.

- [ ] **Step 3: Write failing sensor tests** (fake detectors and fake engine, no MediaPipe)

- one frame with a face → `engine.push` called once with `Float64Array(SAMPLE.length)` where `face = 1`, `r/g/b` from the pixels, `light = luma`; then `post` called with `{ type: 'frameDone' }`.
- no face → push with `face = 0`, `r/g/b = 0`.
- pose runs on every second frame only (15 Hz at 30 fps); frames in between reuse the last `shoulderY/W` (sample-and-hold).
- `now()` crossing a whole second → `engine.estimate` called and `{ type: 'estimate' }` + `{ type: 'diagnostics' }` posted, at most once per second.
- every posted message passes `isFromWorker` (privacy boundary).
- `source.close()` is called for every frame, including when a detector throws; a throw posts `{ type: 'error', code: 'vision_failed' }` once.
- `reset()` calls `engine.reset()` and clears the previous landmarks used by `motion`.
- mouth `MouthStd` input: the sample carries `mouthOpening` per frame (std is computed in Go).

- [ ] **Step 4: Run, expect FAIL**; **Step 5: implement `createSensor` and the worker shell**

Worker shell: on `init`, load engine (`loadEngine`) and detectors (`numFaces: 1`, `runningMode: 'VIDEO'`, GPU with CPU fallback on failure); post `ready` or `error`. On `frame`: draw the `VideoFrame` into one reused `OffscreenCanvas(640, 480)` with `willReadFrequently: true`, `frame.close()` immediately after drawing, run `createSensor().onFrame`. Frame cost (`performance.now()` around `onFrame`) averaged per second into `Diagnostics.frameCostMs`.

- [ ] **Step 6: Run, expect PASS** — `npx vitest run packages/vitals-web/src/worker/`

- [ ] **Step 7: Commit** — `Vitals-web - Worker do sensor com MediaPipe e assets fixados`

---

### Task 8: Camera and `createVitals`

**Files:**
- Create: `packages/vitals-web/src/camera.ts`, `camera.test.ts`, `packages/vitals-web/src/vitals.ts`, `vitals.test.ts`
- Modify: `packages/vitals-web/src/mock.ts` (+ emits `diagnostics` once per second with `frameCostMs: 0, droppedFrames: 0, pulse: []`), `mock.test.ts`, `packages/vitals-web/src/index.ts` (export `createVitals`, `Diagnostics`)

**Interfaces:**
- Consumes: `ToWorker`, `FromWorker`, `isFromWorker`, `Diagnostics`.
- Produces:
```ts
// camera.ts
export async function openCamera(video: HTMLVideoElement, media?: MediaDevices): Promise<MediaStream>; // 640x480 ideal, facingMode 'user', frameRate ideal 30; throws VitalsError
export function mapCameraError(e: unknown): VitalsError; // NotAllowedError|SecurityError → camera_denied; NotFoundError|NotReadableError|OverconstrainedError|AbortError → camera_unavailable; else camera_unavailable
export function startFrameLoop(video: HTMLVideoElement, send: (frame: VideoFrame, tMs: number) => void, isBusy: () => boolean, onDrop: () => void): () => void; // rVFC; tMs = metadata.captureTime ?? performance.now()
// vitals.ts
export interface VitalsDeps { createWorker(): Worker; media?: MediaDevices }
export function createVitals(options: VitalsOptions, deps?: VitalsDeps): Vitals;
```
`createVitals` default worker: `new Worker(new URL('./worker/sensor.worker.js', import.meta.url), { type: 'module' })` (or the classic bundle chosen in Task 7). `unsupported_browser` when `requestVideoFrameCallback`, `VideoFrame` or `OffscreenCanvas` is missing.

- [ ] **Step 1: Write failing tests** (fake `MediaDevices`, fake `Worker` that records `postMessage` and lets the test emit messages, fake `video` with a controllable `requestVideoFrameCallback`)

`camera.test.ts`:
- each DOMException name maps as listed.
- frame loop: while `isBusy()` is true, a new frame is closed and `onDrop` called, never sent (backpressure).

`vitals.test.ts`:
- states `starting → calibrating` after worker `ready`; `measuring` on the first estimate with `confidence !== 'invalid'`.
- `estimate` and `diagnostics` from the worker re-emitted; a message failing `isFromWorker` is dropped.
- denied camera → `error { code: 'camera_denied' }`, state `stopped`, `start()` rejects, worker terminated.
- worker `error` `wasm_failed` → same error re-emitted, everything stopped.
- `stop()` → every track `readyState === 'ended'`, worker terminated, frame loop cancelled, state `stopped`.
- Review Focus 1: a track fires `ended` mid-session → `error { code: 'camera_unavailable' }`, state `stopped`.
- Review Focus 4: `start()` twice → one `getUserMedia` call; `stop()` before the first `start()` resolves → tracks ended once the stream arrives, no worker left, `start()` resolves without emitting `calibrating`.
- `pagehide` on `window` → same as `stop()`.
- `summary()` after a session → same shape and rules as the mock (`hrStart`, `hrEnd` from valid estimates, `durationS`, `method: 'pos'`).

- [ ] **Step 2: Run, expect FAIL**; **Step 3: implement**; **Step 4: run, expect PASS**

Run: `npm test && npm run typecheck && npm run lint`

Share the summary computation between `mock.ts` and `vitals.ts` by moving it into `src/summary.ts` (`summarize(history: Estimate[], durationS: number, method: Method): ExerciseSummary`); `mock.test.ts` must still pass unchanged.

- [ ] **Step 5: Commit** — `Vitals-web - Câmera e módulo real createVitals`

---

### Task 9: Angular app shell, CSP server, consent and Exit

**Files:**
- Create: `apps/demo/` (Angular CLI: standalone, no SSR, no routing module — one `AppComponent` switching screens by a signal), `apps/demo/serve.mjs`, `apps/demo/src/app/vitals.service.ts`, `vitals.service.spec.ts`, `apps/demo/src/app/screens/welcome/*`, `apps/demo/src/app/shell/*`
- Modify: root `package.json` (`workspaces: ["packages/*", "apps/*"]`), `apps/demo/angular.json` (`optimization.styles.inlineCritical: false`, assets glob copying `packages/vitals-web/assets/**` to `/assets/vitals/`), `apps/demo/src/index.html` (CSP `<meta>` with the exact string from Global Constraints), `.github/workflows/ci.yml` (job `app`: build)

**Interfaces:**
- Consumes: `createVitals`, `createMockVitals`, `Vitals`, `Estimate`, `Diagnostics`, `ExerciseSummary`.
- Produces:
```ts
export type Screen = 'welcome' | 'calibration' | 'exercise' | 'summary';
@Injectable({ providedIn: 'root' }) export class VitalsService {
  readonly screen: Signal<Screen>; readonly estimate: Signal<Estimate | null>; readonly diagnostics: Signal<Diagnostics | null>;
  readonly summary: Signal<ExerciseSummary | null>; readonly cameraOn: Signal<boolean>; readonly error: Signal<VitalsError | null>;
  consentAndStart(video: HTMLVideoElement): Promise<void>; finishExercise(): void; exit(): void;
}
// VITALS_FACTORY injection token: () => Vitals; mock when location.search contains 'mock=1'
```
`serve.mjs`: `node:http` static server on `127.0.0.1:4300` for `dist/demo/browser`, sets `Content-Security-Policy` (exact string), `Content-Type: application/wasm` for `.wasm`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `Permissions-Policy: camera=(self), microphone=()`; rejects paths containing `..`.

- [ ] **Step 1: Scaffold** — `npx @angular/cli@<current stable> new demo --directory apps/demo --standalone --style=css --ssr=false --routing=false --skip-git`; add `vitals-web` as a workspace dependency.

- [ ] **Step 2: Write failing tests** (`vitals.service.spec.ts` with a fake `Vitals` through `VITALS_FACTORY`)

- before `consentAndStart`, `start` is never called and `cameraOn()` is false.
- after it, `start` called once, `cameraOn()` true, `screen()` = `'calibration'`; first non-invalid estimate → `'exercise'`.
- `exit()` from any screen → `stop` called, `screen()` = `'welcome'`, `estimate()`, `summary()`, `diagnostics()` all `null`.
- `error` event → `screen()` = `'welcome'`, `error()` set, `cameraOn()` false.
- Welcome component test: the "Turn on camera" button is the only path to `consentAndStart`; the not-a-medical-device text is rendered.
- `serve.mjs` test (Node, vitest in `apps/demo/serve.test.mjs`): response headers include the exact CSP; `/../package.json` → 404; `.wasm` MIME.

- [ ] **Step 3: Run, expect FAIL**; **Step 4: implement**; **Step 5: run, expect PASS**

Run: `npm test --workspace apps/demo && npm run build --workspace apps/demo && node apps/demo/serve.mjs` then open `http://127.0.0.1:4300/?mock=1` in Chrome: no CSP violations in the console, consent screen visible.

- [ ] **Step 6: Commit** — `Demo - App Angular com consentimento, Sair e servidor com CSP`

---

### Task 10: Calibration, exercise, summary, transparency, exhibition mode

**Files:**
- Create: `apps/demo/src/app/screens/{calibration,exercise,summary}/*`, `apps/demo/src/app/shell/transparency-panel.*`, `apps/demo/src/app/reason-text.ts` (+spec)
- Modify: `vitals.service.ts` (+spec)

**Interfaces:**
- Consumes: `VitalsService` from Task 9.
- Produces: `reasonText(reason: Reason | null): string`; `VitalsService` gains `readonly showNumbers: WritableSignal<boolean>`, constants `EXERCISE_S = 120`, `TARGET_RPM = 6`, `NO_FACE_TIMEOUT_S = 60`.

Copy (exact, English, the app's language): `no_face` → "Move into the frame", `too_much_motion` → "Hold still", `low_light` → "More light, please", `talking` → "Try not to talk", `warming_up` → "Getting a steady signal…", `null` → "".

- [ ] **Step 1: Write failing tests**

- `reasonText` for each reason, exact strings above.
- service: 60 consecutive estimates with `reason: 'no_face'` → `exit()` behaviour (stop, welcome, state cleared); a valid estimate in between resets the count.
- service: `EXERCISE_S` after entering `'exercise'` (fake timers) → `stop` called, `screen()` = `'summary'`, `summary()` from `Vitals.summary()`.
- service: from `'summary'`, "Start again" → `'welcome'` with all signals cleared; after `NO_FACE_TIMEOUT_S` idle on summary → `'welcome'`.
- exercise component: numbers (`hrBpm`, `rrRpm`) hidden until `showNumbers` is true; when `confidence` is `invalid`, the reason text is shown instead of numbers; the breathing circle animation period equals `60 / TARGET_RPM` s (CSS variable set from the constant).
- transparency panel: shows `quality`, `confidence`, `method`, `frameCostMs`, `droppedFrames`; draws `pulse` as an SVG `polyline` (no canvas), empty when `pulse` is `[]`.

- [ ] **Step 2: Run, expect FAIL**; **Step 3: implement**; **Step 4: run, expect PASS**

Run: `npm test --workspace apps/demo`

- [ ] **Step 5: Manual check on mock** — `npm run build --workspace apps/demo && node apps/demo/serve.mjs`, open `/?mock=1`, walk welcome → calibration → exercise → summary → welcome; Exit from each screen; no console errors.

- [ ] **Step 6: Commit** — `Demo - Calibração, exercício, resumo, painel de transparência e modo exposição`

---

### Task 11: End to end, performance, docs

**Files:**
- Create: `apps/demo/e2e/demo.spec.ts`, `apps/demo/playwright.config.ts`, `scripts/make-fake-video.mjs`, `docs/exhibition-checklist.md`
- Modify: `.github/workflows/ci.yml` (job `e2e`), `README.md` (one-worker diagram, status table, how to run the demo), `docs/decisions.md` (entries 11 one-worker model, 12 stateful session and 30 s buffer, plus the Task 7 worker-type outcome), `engine/quality/quality.go` constants if team sessions require

**Interfaces:**
- Consumes: everything above.

- [ ] **Step 1: Fake video** — `make-fake-video.mjs` writes `apps/demo/e2e/fake.y4m`: 10 s, 640×480, 30 fps, plain mid-grey with a slow brightness ramp (no face, no person). Generated in CI, gitignored.

- [ ] **Step 2: Write E2E tests** (Chrome flags `--use-fake-ui-for-media-stream --use-fake-device-for-media-stream --use-file-for-fake-video-capture=apps/demo/e2e/fake.y4m`, server `node apps/demo/serve.mjs`)

- real module: consent → within 15 s the calibration screen shows "Move into the frame" (estimates arrive with `no_face`, proving camera → worker → engine → UI).
- Exit → camera indicator gone and `page.evaluate` confirms all tracks of the stream are `ended`.
- `?mock=1`: full flow to the summary with fake timers (`page.clock`).
- no `securitypolicyviolation` events and no requests to any origin other than `127.0.0.1:4300` (`page.on('request')`).

Run: `npx playwright test --config apps/demo/playwright.config.ts`
Expected: all pass.

- [ ] **Step 3: CI job `e2e`** — depends on `wasm`; runs `fetch-assets.sh` (verify), app build, Playwright with Chromium.

- [ ] **Step 4: `docs/exhibition-checklist.md`** — exhibition-laptop procedure: production build, `serve.mjs`, Chrome only, lighting check, 10-session oximeter protocol (numbers only, under `data/`), pass criteria from the spec (every `high`/`medium` estimate within ±5 bpm of the oximeter, no crash or freeze, frame cost under 25 ms and no sustained dropped frames in the transparency panel), and the `?mock=1` fallback.

- [ ] **Step 5: Run the checklist on the exhibition laptop**; adjust `engine/quality` constants only if the sessions show a threshold is wrong, with a test that encodes the new value's reason. Record results (numbers only) in the PR description.

- [ ] **Step 6: Docs** — README status table and diagram match the code; `decisions.md` entries added.

- [ ] **Step 7: Commit** — `Demo - E2E com câmera falsa, checklist da exposição e documentação`
