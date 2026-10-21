import type { FaceLandmarkerOptions, PoseLandmarkerOptions } from '@mediapipe/tasks-vision';
import { loadEngine, type Engine } from '../engine/engine.js';
import { SAMPLE } from '../engine/layout.js';
import { isFromWorker, type FromWorker, type ToWorker } from './protocol.js';
import { motion, mouthOpening, shoulders } from './vision/metrics.js';
import { meanRgb, roiBoxes, type Box, type Point } from './vision/roi.js';

export type VisionImage = Exclude<CanvasImageSource, SVGImageElement>;
export type SensorSource = VisionImage & { close?(): void };

export interface FaceDetector {
  detect(img: VisionImage, tMs: number): Point[][];
}

export interface PoseDetector {
  detect(img: VisionImage, tMs: number): Point[] | null;
}

export interface SensorDeps {
  face: FaceDetector;
  pose: PoseDetector;
  engine: Engine;
  post: (m: FromWorker) => void;
  now: () => number;
}

export interface Sensor {
  // pixels(rect) returns the RGBA bytes of that rectangle of the frame, row by row.
  onFrame(
    source: SensorSource,
    tMs: number,
    width: number,
    height: number,
    pixels: (rect: Box) => Uint8ClampedArray,
    startMs?: number,
  ): void;
  // A frame that never reached the detectors (e.g. drawing it failed): report once, count its cost, ack it.
  dropFrame(error: unknown, startMs: number): void;
  reset(): void;
}

// Pose at 15 Hz is enough for breathing (≤ 0.5 Hz) and halves the vision cost per frame.
const POSE_EVERY_N_FRAMES = 2;
const FRAME_WIDTH = 640;
const FRAME_HEIGHT = 480;

// The privacy boundary: nothing leaves the worker unless it is a protocol.ts message without buffers or frames.
export function checkedPost(sink: (m: FromWorker) => void): (m: FromWorker) => void {
  return (m) => {
    if (!isFromWorker(m)) {
      console.error(`sensor.worker: dropped a '${String((m as { type?: unknown }).type)}' message outside protocol.ts`);
      return;
    }
    sink(m);
  };
}

function boundingBox(boxes: Box[]): Box {
  const x0 = Math.min(...boxes.map((b) => b.x));
  const y0 = Math.min(...boxes.map((b) => b.y));
  const x1 = Math.max(...boxes.map((b) => b.x + b.w));
  const y1 = Math.max(...boxes.map((b) => b.y + b.h));
  return { x: x0, y: y0, w: x1 - x0, h: y1 - y0 };
}

function skinColour(
  faces: Point[][],
  width: number,
  height: number,
  pixels: (rect: Box) => Uint8ClampedArray,
): ReturnType<typeof meanRgb> {
  const boxes = roiBoxes(faces, width, height);
  if (boxes.length === 0) return null;
  const rect = boundingBox(boxes);
  const local = boxes.map((b) => ({ x: b.x - rect.x, y: b.y - rect.y, w: b.w, h: b.h }));
  return meanRgb(pixels(rect), rect.w, local);
}

function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

export function createSensor(deps: SensorDeps): Sensor {
  const post = checkedPost(deps.post);
  let frames = 0;
  let prevFace: Point[] | null = null;
  let held: { y: number; w: number } | null = null;
  let second: number | null = null;
  let costSum = 0;
  let costCount = 0;
  const reported = { vision: false, wasm: false };

  function sample(img: VisionImage, tMs: number, width: number, height: number, pixels: (r: Box) => Uint8ClampedArray) {
    const faces = deps.face.detect(img, tMs);
    if (frames % POSE_EVERY_N_FRAMES === 0) held = shoulders(deps.pose.detect(img, tMs));
    frames++;
    const x = new Float64Array(SAMPLE.length);
    x[SAMPLE.timeMs] = tMs;
    x[SAMPLE.shoulderY] = held?.y ?? 0;
    x[SAMPLE.shoulderW] = held?.w ?? 0;
    const face = faces[0];
    const skin = face ? skinColour(faces, width, height, pixels) : null;
    if (!face || !skin) {
      prevFace = null;
      return x;
    }
    x[SAMPLE.face] = 1;
    x[SAMPLE.r] = skin.r;
    x[SAMPLE.g] = skin.g;
    x[SAMPLE.b] = skin.b;
    x[SAMPLE.light] = skin.luma;
    x[SAMPLE.mouth] = mouthOpening(face);
    x[SAMPLE.motion] = motion(prevFace, face);
    prevFace = face;
    return x;
  }

  function estimateOnNewSecond(nowMs: number, tMs: number): void {
    const s = Math.floor(nowMs / 1000);
    if (second === null) second = s;
    if (s <= second) return;
    second = s;
    const { estimate, pulse } = deps.engine.estimate(tMs);
    post({ type: 'estimate', estimate });
    const frameCostMs = costCount === 0 ? 0 : costSum / costCount;
    // droppedFrames is counted on the main thread, which never sends the frames it drops.
    post({ type: 'diagnostics', diagnostics: { frameCostMs, droppedFrames: 0, pulse: Array.from(pulse) } });
    costSum = 0;
    costCount = 0;
  }

  function fail(kind: 'vision' | 'wasm', e: unknown): void {
    if (reported[kind]) return;
    reported[kind] = true;
    post({ type: 'error', code: kind === 'vision' ? 'vision_failed' : 'wasm_failed', message: errorMessage(e) });
  }

  function done(startMs: number): void {
    costSum += deps.now() - startMs;
    costCount++;
    post({ type: 'frameDone' });
  }

  return {
    onFrame(source, tMs, width, height, pixels, startMs = deps.now()) {
      try {
        let x: Float64Array;
        try {
          x = sample(source, tMs, width, height, pixels);
        } catch (e) {
          fail('vision', e);
          return;
        }
        try {
          deps.engine.push(x);
          estimateOnNewSecond(deps.now(), tMs);
        } catch (e) {
          fail('wasm', e);
        }
      } finally {
        source.close?.();
        done(startMs);
      }
    },
    dropFrame(error, startMs) {
      fail('vision', error);
      done(startMs);
    },
    reset() {
      deps.engine.reset();
      frames = 0;
      prevFace = null;
      held = null;
      reported.vision = false;
      reported.wasm = false;
    },
  };
}

export type Delegate = 'GPU' | 'CPU';

export function faceOptions(assetsBaseUrl: string, delegate: Delegate): FaceLandmarkerOptions {
  return {
    baseOptions: { modelAssetPath: `${assetsBaseUrl}/mediapipe/face_landmarker.task`, delegate },
    runningMode: 'VIDEO',
    // A second person in view must never mix into the skin region.
    numFaces: 1,
  };
}

export function poseOptions(assetsBaseUrl: string, delegate: Delegate): PoseLandmarkerOptions {
  return {
    baseOptions: { modelAssetPath: `${assetsBaseUrl}/mediapipe/pose_landmarker_lite.task`, delegate },
    runningMode: 'VIDEO',
    numPoses: 1,
  };
}

export async function withCpuFallback<T>(create: (delegate: Delegate) => Promise<T>): Promise<T> {
  try {
    return await create('GPU');
  } catch {
    return create('CPU');
  }
}

export interface Detectors {
  face: FaceDetector;
  pose: PoseDetector;
  close(): void;
}

// Like Promise.all, but on failure closes whatever did load, so a failed init leaks no landmarker.
export async function allOrClose<T extends readonly Promise<{ close(): void }>[]>(
  loads: [...T],
): Promise<{ -readonly [K in keyof T]: Awaited<T[K]> }> {
  const settled = await Promise.allSettled(loads);
  const failed = settled.find((r): r is PromiseRejectedResult => r.status === 'rejected');
  if (failed === undefined) {
    return settled.map((r) => (r as PromiseFulfilledResult<unknown>).value) as {
      -readonly [K in keyof T]: Awaited<T[K]>;
    };
  }
  for (const r of settled) if (r.status === 'fulfilled') r.value.close();
  throw failed.reason;
}

async function loadDetectors(assetsBaseUrl: string): Promise<Detectors> {
  const vision = await import('@mediapipe/tasks-vision');
  // useModule = true: the classic loader fails inside a module worker with "ModuleFactory not set." (Task 7 spike).
  const fileset = await vision.FilesetResolver.forVisionTasks(`${assetsBaseUrl}/mediapipe/wasm`, true);
  const [face, pose] = await allOrClose([
    withCpuFallback((d) => vision.FaceLandmarker.createFromOptions(fileset, faceOptions(assetsBaseUrl, d))),
    withCpuFallback((d) => vision.PoseLandmarker.createFromOptions(fileset, poseOptions(assetsBaseUrl, d))),
  ]);
  return {
    face: { detect: (img, tMs) => face.detectForVideo(img, tMs).faceLandmarks },
    pose: { detect: (img, tMs) => pose.detectForVideo(img, tMs).landmarks[0] ?? null },
    close() {
      face.close();
      pose.close();
    },
  };
}

export interface FrameCanvas {
  image: VisionImage;
  // Returns the size the frame was drawn at, when it differs from the default.
  draw: (frame: VideoFrame) => { width: number; height: number } | undefined;
  pixels: (rect: Box) => Uint8ClampedArray;
}

// A fixed width with the camera's own aspect ratio: stretching to 4:3 would distort the face the landmarker sees.
export function frameHeightFor(displayWidth: number, displayHeight: number): number {
  return displayWidth > 0 && displayHeight > 0
    ? Math.round((FRAME_WIDTH * displayHeight) / displayWidth)
    : FRAME_HEIGHT;
}

export interface WorkerDeps {
  post: (m: FromWorker) => void;
  canvas: FrameCanvas | null;
  loadEngine: (assetsBaseUrl: string) => Promise<Engine>;
  loadDetectors: (assetsBaseUrl: string) => Promise<Detectors>;
  now: () => number;
  width: number;
  height: number;
}

export function createWorkerHandler(deps: WorkerDeps): (m: ToWorker) => void {
  const post = checkedPost(deps.post);
  const { canvas, width, height } = deps;
  let sensor: Sensor | null = null;
  let loading = false;
  // The Go runtime has no close; once loaded it is kept and reused by a retried init.
  let engine: Engine | null = null;

  async function init(assetsBaseUrl: string): Promise<void> {
    if (sensor !== null) {
      post({ type: 'ready' });
      return;
    }
    if (loading) return;
    if (canvas === null) {
      post({ type: 'error', code: 'vision_failed', message: '2d context unavailable' });
      return;
    }
    loading = true;
    const [e, d] = await Promise.allSettled([
      engine ?? deps.loadEngine(assetsBaseUrl),
      deps.loadDetectors(assetsBaseUrl),
    ]);
    loading = false;
    if (e.status === 'fulfilled') engine = e.value;
    if (e.status === 'rejected') {
      if (d.status === 'fulfilled') d.value.close();
      post({ type: 'error', code: 'wasm_failed', message: errorMessage(e.reason) });
      return;
    }
    if (d.status === 'rejected') {
      post({ type: 'error', code: 'vision_failed', message: errorMessage(d.reason) });
      return;
    }
    sensor = createSensor({ face: d.value.face, pose: d.value.pose, engine: e.value, post: deps.post, now: deps.now });
    post({ type: 'ready' });
  }

  function frame(f: VideoFrame, tMs: number): void {
    const startMs = deps.now();
    if (sensor === null || canvas === null) {
      f.close();
      post({ type: 'frameDone' });
      return;
    }
    let drawError: unknown = null;
    let drawn = false;
    let size: { width: number; height: number } | undefined;
    try {
      size = canvas.draw(f);
      drawn = true;
    } catch (e) {
      drawError = e;
    } finally {
      f.close();
    }
    if (drawn) sensor.onFrame(canvas.image, tMs, size?.width ?? width, size?.height ?? height, canvas.pixels, startMs);
    else sensor.dropFrame(drawError, startMs);
  }

  return (m) => {
    if (m.type === 'init') void init(m.assetsBaseUrl);
    else if (m.type === 'reset') sensor?.reset();
    else frame(m.frame, m.tMs);
  };
}

interface WorkerScope {
  postMessage(m: FromWorker): void;
  onmessage: ((e: MessageEvent<ToWorker>) => void) | null;
}

function offscreenCanvas(): FrameCanvas | null {
  const image = new OffscreenCanvas(FRAME_WIDTH, FRAME_HEIGHT);
  const ctx = image.getContext('2d', { willReadFrequently: true });
  if (ctx === null) return null;
  return {
    image,
    draw: (frame) => {
      const height = frameHeightFor(frame.displayWidth, frame.displayHeight);
      if (image.height !== height) image.height = height;
      ctx.drawImage(frame, 0, 0, FRAME_WIDTH, height);
      return { width: FRAME_WIDTH, height };
    },
    pixels: (r) => ctx.getImageData(r.x, r.y, r.w, r.h).data,
  };
}

function startWorker(scope: WorkerScope): void {
  const handle = createWorkerHandler({
    post: (m) => {
      scope.postMessage(m);
    },
    canvas: offscreenCanvas(),
    loadEngine: (base) => loadEngine(`${base}/engine.wasm`, `${base}/wasm_exec.js`),
    loadDetectors,
    now: () => performance.now(),
    width: FRAME_WIDTH,
    height: FRAME_HEIGHT,
  });
  scope.onmessage = (e) => {
    handle(e.data);
  };
}

// Importing this module elsewhere (tests, bundlers) must not start a worker.
if (typeof (globalThis as { WorkerGlobalScope?: unknown }).WorkerGlobalScope === 'function') {
  startWorker(globalThis);
}
