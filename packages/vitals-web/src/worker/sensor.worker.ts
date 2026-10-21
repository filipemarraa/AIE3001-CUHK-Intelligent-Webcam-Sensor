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
  ): void;
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

  return {
    onFrame(source, tMs, width, height, pixels) {
      const start = deps.now();
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
          const end = deps.now();
          costSum += end - start;
          costCount++;
          estimateOnNewSecond(end, tMs);
        } catch (e) {
          fail('wasm', e);
        }
      } finally {
        source.close?.();
        post({ type: 'frameDone' });
      }
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

async function createDetectors(assetsBaseUrl: string): Promise<{ face: FaceDetector; pose: PoseDetector }> {
  const vision = await import('@mediapipe/tasks-vision');
  // useModule = true: the classic loader fails inside a module worker with "ModuleFactory not set." (Task 7 spike).
  const fileset = await vision.FilesetResolver.forVisionTasks(`${assetsBaseUrl}/mediapipe/wasm`, true);
  const [face, pose] = await Promise.all([
    withCpuFallback((d) => vision.FaceLandmarker.createFromOptions(fileset, faceOptions(assetsBaseUrl, d))),
    withCpuFallback((d) => vision.PoseLandmarker.createFromOptions(fileset, poseOptions(assetsBaseUrl, d))),
  ]);
  return {
    face: { detect: (img, tMs) => face.detectForVideo(img, tMs).faceLandmarks },
    pose: { detect: (img, tMs) => pose.detectForVideo(img, tMs).landmarks[0] ?? null },
  };
}

interface WorkerScope {
  postMessage(m: FromWorker): void;
  onmessage: ((e: MessageEvent<ToWorker>) => void) | null;
}

function startWorker(scope: WorkerScope): void {
  const raw = (m: FromWorker): void => {
    scope.postMessage(m);
  };
  const post = checkedPost(raw);
  const canvas = new OffscreenCanvas(FRAME_WIDTH, FRAME_HEIGHT);
  const ctx = canvas.getContext('2d', { willReadFrequently: true });
  let sensor: Sensor | null = null;
  let initialising = false;

  async function init(assetsBaseUrl: string): Promise<void> {
    if (initialising) return;
    initialising = true;
    const [engine, detectors] = await Promise.allSettled([
      loadEngine(`${assetsBaseUrl}/engine.wasm`, `${assetsBaseUrl}/wasm_exec.js`),
      createDetectors(assetsBaseUrl),
    ]);
    if (engine.status === 'rejected') {
      post({ type: 'error', code: 'wasm_failed', message: errorMessage(engine.reason) });
      return;
    }
    if (detectors.status === 'rejected' || ctx === null) {
      const reason: unknown = detectors.status === 'rejected' ? detectors.reason : '2d context unavailable';
      post({ type: 'error', code: 'vision_failed', message: errorMessage(reason) });
      return;
    }
    sensor = createSensor({ ...detectors.value, engine: engine.value, post: raw, now: () => performance.now() });
    post({ type: 'ready' });
  }

  scope.onmessage = (e) => {
    const m = e.data;
    if (m.type === 'init') {
      void init(m.assetsBaseUrl);
    } else if (m.type === 'reset') {
      sensor?.reset();
    } else if (sensor === null || ctx === null) {
      m.frame.close();
      post({ type: 'frameDone' });
    } else {
      ctx.drawImage(m.frame, 0, 0, FRAME_WIDTH, FRAME_HEIGHT);
      m.frame.close();
      sensor.onFrame(canvas, m.tMs, FRAME_WIDTH, FRAME_HEIGHT, (r) => ctx.getImageData(r.x, r.y, r.w, r.h).data);
    }
  };
}

// Importing this module elsewhere (tests, bundlers) must not start a worker.
if (typeof (globalThis as { WorkerGlobalScope?: unknown }).WorkerGlobalScope === 'function') {
  startWorker(globalThis);
}
