import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Decoded } from '../engine/decode.js';
import type { Engine } from '../engine/engine.js';
import { SAMPLE } from '../engine/layout.js';
import type { Estimate } from '../types.js';
import { isFromWorker, type FromWorker } from './protocol.js';
import {
  allOrClose,
  createSensor,
  createWorkerHandler,
  faceOptions,
  frameHeightFor,
  poseOptions,
  withCpuFallback,
  type FaceDetector,
  type Detectors,
  type FrameCanvas,
  type PoseDetector,
  type SensorSource,
} from './sensor.worker.js';
import type { ToWorker } from './protocol.js';
import type { Box, Point } from './vision/roi.js';
import { mouthOpening } from './vision/metrics.js';

const WIDTH = 100;
const HEIGHT = 50;
const SKIN = [200, 100, 50] as const;
const BACKGROUND = [10, 10, 10] as const;

// Same geometry as roi.test.ts: at 100×50 the boxes are forehead {40,10,20,10} and cheeks {26,26,8,8}, {66,26,8,8}.
function makeFace(dx = 0, lipGap = 0.02): Point[] {
  const face: Point[] = Array.from({ length: 478 }, () => ({ x: 0.5 + dx, y: 0.5 }));
  const at: Record<number, Point> = {
    109: { x: 0.4, y: 0.2 },
    338: { x: 0.6, y: 0.2 },
    10: { x: 0.5, y: 0.2 },
    151: { x: 0.5, y: 0.4 },
    50: { x: 0.3, y: 0.6 },
    280: { x: 0.7, y: 0.6 },
    234: { x: 0.2, y: 0.5 },
    454: { x: 0.8, y: 0.5 },
    33: { x: 0.42, y: 0.35 },
    263: { x: 0.58, y: 0.35 },
    13: { x: 0.5, y: 0.7 },
    14: { x: 0.5, y: 0.7 + lipGap },
    152: { x: 0.5, y: 0.9 },
  };
  for (const [i, p] of Object.entries(at)) face[Number(i)] = { x: p.x + dx, y: p.y };
  return face;
}

const SKIN_BOXES: Box[] = [
  { x: 40, y: 10, w: 20, h: 10 },
  { x: 26, y: 26, w: 8, h: 8 },
  { x: 66, y: 26, w: 8, h: 8 },
];

// Full RGBA image: skin colour inside the skin boxes, background elsewhere, so wrong offsets change the mean.
function image(): Uint8ClampedArray {
  const px = new Uint8ClampedArray(WIDTH * HEIGHT * 4);
  for (let y = 0; y < HEIGHT; y++) {
    for (let x = 0; x < WIDTH; x++) {
      const skin = SKIN_BOXES.some((b) => x >= b.x && x < b.x + b.w && y >= b.y && y < b.y + b.h);
      px.set([...(skin ? SKIN : BACKGROUND), 255], (y * WIDTH + x) * 4);
    }
  }
  return px;
}

function crop(px: Uint8ClampedArray, rect: Box): Uint8ClampedArray {
  const out = new Uint8ClampedArray(rect.w * rect.h * 4);
  for (let y = 0; y < rect.h; y++) {
    const from = ((rect.y + y) * WIDTH + rect.x) * 4;
    out.set(px.subarray(from, from + rect.w * 4), y * rect.w * 4);
  }
  return out;
}

const estimate: Estimate = {
  tMs: 0,
  hrBpm: 70,
  rrRpm: 12,
  breathPhase: 'in',
  quality: 0.9,
  confidence: 'high',
  reason: null,
  method: 'pos',
};

const POSE: Point[] = Array.from({ length: 33 }, (_, i) =>
  i === 11 ? { x: 0.3, y: 0.7 } : i === 12 ? { x: 0.7, y: 0.74 } : { x: 0.5, y: 0.5 },
);

interface SetupOptions {
  record?: boolean;
  faces?: () => Point[][];
  pose?: () => Point[] | null;
  estimate?: () => Decoded;
  push?: () => void;
}

function setup(opts: SetupOptions = {}) {
  const clock = { t: 0 };
  const face: FaceDetector = {
    detect: vi.fn(() => {
      clock.t += 4;
      return opts.faces ? opts.faces() : [makeFace()];
    }),
  };
  const detectPose = vi.fn(() => (opts.pose ? opts.pose() : POSE));
  const pose: PoseDetector = { detect: detectPose };
  const pushed: Float64Array[] = [];
  const push = vi.fn((x: Float64Array) => {
    opts.push?.();
    pushed.push(x.slice());
  });
  const estimateAt = vi.fn(opts.estimate ?? (() => ({ estimate, pulse: new Float64Array([0.1, -0.2]) })));
  const resetEngine = vi.fn();
  const engine: Engine = { push, estimate: estimateAt, reset: resetEngine };
  const posted: FromWorker[] = [];
  const post = vi.fn((m: FromWorker) => {
    posted.push(m);
  });
  const sensor = createSensor({ face, pose, engine, post, now: () => clock.t, record: opts.record ?? false });
  const px = image();
  const rects: Box[] = [];
  const pixels = (rect: Box): Uint8ClampedArray => {
    rects.push(rect);
    return crop(px, rect);
  };
  const close = vi.fn();
  const source = { close } as unknown as SensorSource;
  const frame = (tMs: number): void => {
    sensor.onFrame(source, tMs, WIDTH, HEIGHT, pixels);
  };
  return { clock, detectPose, push, estimateAt, resetEngine, pushed, posted, sensor, rects, close, frame };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('createSensor', () => {
  it('posts the numeric samples of the last second with the estimate when recording', () => {
    const s = setup({ record: true });
    s.clock.t = 900;
    s.frame(10_000);
    s.clock.t = 996;
    s.frame(10_033);
    const samples = s.posted.find((m) => m.type === 'samples');
    expect(samples !== undefined && isFromWorker(samples)).toBe(true);
    const rows = samples?.type === 'samples' ? samples.rows : [];
    expect(rows.map((r) => r[SAMPLE.timeMs])).toEqual([10_000, 10_033]);
    expect(rows.every((r) => r.length === SAMPLE.length)).toBe(true);
    s.clock.t = 2100;
    s.frame(10_066);
    const later = s.posted.filter((m) => m.type === 'samples').at(-1);
    expect(later?.type === 'samples' ? later.rows.map((r) => r[SAMPLE.timeMs]) : []).toEqual([10_066]);
  });

  it('posts no samples unless recording', () => {
    const s = setup();
    s.clock.t = 900;
    s.frame(10_000);
    s.clock.t = 2100;
    s.frame(10_033);
    expect(s.posted.some((m) => m.type === 'samples')).toBe(false);
  });

  it('reduces a frame with a face to one sample, then posts frameDone', () => {
    const s = setup();
    s.frame(1234);
    expect(s.push).toHaveBeenCalledTimes(1);
    const x = s.pushed[0] as Float64Array;
    expect(x).toBeInstanceOf(Float64Array);
    expect(x.length).toBe(SAMPLE.length);
    expect(x[SAMPLE.timeMs]).toBe(1234);
    expect(x[SAMPLE.face]).toBe(1);
    expect([x[SAMPLE.r], x[SAMPLE.g], x[SAMPLE.b]]).toEqual([...SKIN]);
    expect(x[SAMPLE.light]).toBeCloseTo(0.299 * 200 + 0.587 * 100 + 0.114 * 50, 9);
    expect(x[SAMPLE.motion]).toBe(0);
    expect(x[SAMPLE.shoulderY]).toBeCloseTo(0.72, 9);
    expect(x[SAMPLE.shoulderW]).toBeCloseTo(0.4, 9);
    expect(s.posted.at(-1)).toEqual({ type: 'frameDone' });
  });

  it('reads back only the bounding box of the skin regions', () => {
    const s = setup();
    s.frame(0);
    expect(s.rects).toEqual([{ x: 26, y: 10, w: 48, h: 24 }]);
  });

  it('pushes a zero sample without a face and keeps the shoulders from pose', () => {
    const s = setup({ faces: () => [] });
    s.frame(500);
    const x = s.pushed[0] as Float64Array;
    expect(x[SAMPLE.timeMs]).toBe(500);
    for (const k of ['face', 'r', 'g', 'b', 'light', 'mouth', 'motion'] as const) expect(x[SAMPLE[k]]).toBe(0);
    expect(x[SAMPLE.shoulderW]).toBeCloseTo(0.4, 9);
    expect(s.rects).toEqual([]);
    expect(s.posted).toEqual([{ type: 'frameDone' }]);
  });

  it('writes zero shoulders when pose finds no body', () => {
    const s = setup({ pose: () => null });
    s.frame(0);
    expect(s.pushed[0]?.[SAMPLE.shoulderY]).toBe(0);
    expect(s.pushed[0]?.[SAMPLE.shoulderW]).toBe(0);
  });

  it('runs pose on every second frame and holds the last shoulders in between', () => {
    let call = 0;
    const s = setup({
      pose: () => {
        call++;
        return POSE.map((p, i) => (i === 11 || i === 12 ? { x: p.x, y: p.y + call / 100 } : p));
      },
    });
    for (let i = 0; i < 4; i++) s.frame(i * 33);
    expect(s.detectPose).toHaveBeenCalledTimes(2);
    const ys = s.pushed.map((x) => x[SAMPLE.shoulderY]);
    expect(ys[0]).toBeCloseTo(0.73, 9);
    expect(ys[1]).toBe(ys[0]);
    expect(ys[2]).toBeCloseTo(0.74, 9);
    expect(ys[3]).toBe(ys[2]);
  });

  it('estimates at most once per whole second of now() and posts estimate plus diagnostics', () => {
    const s = setup();
    s.clock.t = 900;
    s.frame(10_000);
    expect(s.estimateAt).not.toHaveBeenCalled();
    s.clock.t = 996;
    s.frame(10_033); // detect advances now() to 1000: crosses a second
    expect(s.estimateAt).toHaveBeenCalledTimes(1);
    expect(s.estimateAt).toHaveBeenCalledWith(10_033);
    expect(s.posted.slice(-3)).toEqual([
      { type: 'estimate', estimate },
      { type: 'diagnostics', diagnostics: { frameCostMs: 4, droppedFrames: 0, pulse: [0.1, -0.2] } },
      { type: 'frameDone' },
    ]);
    s.clock.t = 1500;
    s.frame(10_066);
    s.clock.t = 1990;
    s.frame(10_100);
    expect(s.estimateAt).toHaveBeenCalledTimes(1);
    s.clock.t = 2100;
    s.frame(10_133);
    expect(s.estimateAt).toHaveBeenCalledTimes(2);
  });

  it('posts only messages that pass isFromWorker', () => {
    const s = setup();
    for (let i = 0; i < 40; i++) {
      s.clock.t = i * 100;
      s.frame(i * 33);
    }
    expect(s.posted.length).toBeGreaterThan(40);
    expect(s.posted.every((m) => isFromWorker(m))).toBe(true);
    const diag = s.posted.find((m) => m.type === 'diagnostics');
    expect(Array.isArray(diag?.diagnostics.pulse)).toBe(true);
  });

  it('drops and logs a message that would carry a buffer', () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const leaky = { ...estimate, leak: new Uint8ClampedArray(4) } as Estimate;
    const s = setup({ estimate: () => ({ estimate: leaky, pulse: new Float64Array(0) }) });
    s.clock.t = 990;
    s.frame(0);
    s.clock.t = 998;
    s.frame(33);
    expect(s.estimateAt).toHaveBeenCalledTimes(1);
    expect(s.posted.some((m) => m.type === 'estimate')).toBe(false);
    expect(s.posted.some((m) => m.type === 'diagnostics')).toBe(true);
    expect(error).toHaveBeenCalledTimes(1);
  });

  it('closes the source every frame, also when a detector throws, and reports vision_failed once', () => {
    const s = setup({
      faces: () => {
        throw new Error('gpu lost');
      },
    });
    s.frame(0);
    s.frame(33);
    expect(s.close).toHaveBeenCalledTimes(2);
    expect(s.push).not.toHaveBeenCalled();
    expect(s.posted).toEqual([
      { type: 'error', code: 'vision_failed', message: 'gpu lost' },
      { type: 'frameDone' },
      { type: 'frameDone' },
    ]);
  });

  it('closes the source of a normal frame', () => {
    const s = setup();
    s.frame(0);
    expect(s.close).toHaveBeenCalledTimes(1);
  });

  it('measures motion against the previous landmarks and forgets them on reset', () => {
    let dx = 0;
    const s = setup({ faces: () => [makeFace(dx)] });
    s.frame(0);
    dx = 0.016;
    s.frame(33);
    expect(s.pushed[1]?.[SAMPLE.motion]).toBeCloseTo(0.1, 9); // 0.016 shift / 0.16 inter-ocular distance
    s.sensor.reset();
    expect(s.resetEngine).toHaveBeenCalledTimes(1);
    dx = 0.05;
    s.frame(66);
    expect(s.pushed[2]?.[SAMPLE.motion]).toBe(0);
  });

  it('carries the mouth opening of each frame', () => {
    const gaps = [0.02, 0.08];
    let i = 0;
    const s = setup({ faces: () => [makeFace(0, gaps[i++] ?? 0)] });
    s.frame(0);
    s.frame(33);
    expect(s.pushed[0]?.[SAMPLE.mouth]).toBeCloseTo(mouthOpening(makeFace(0, 0.02)), 12);
    expect(s.pushed[1]?.[SAMPLE.mouth]).toBeCloseTo(mouthOpening(makeFace(0, 0.08)), 12);
    expect(s.pushed[1]?.[SAMPLE.mouth]).toBeGreaterThan(s.pushed[0]?.[SAMPLE.mouth] ?? 1);
  });

  it('reports wasm_failed once when the engine throws, and still frees the frame', () => {
    const s = setup({
      push: () => {
        throw new Error('go panic');
      },
    });
    s.frame(0);
    s.frame(33);
    expect(s.close).toHaveBeenCalledTimes(2);
    expect(s.posted).toEqual([
      { type: 'error', code: 'wasm_failed', message: 'go panic' },
      { type: 'frameDone' },
      { type: 'frameDone' },
    ]);
  });
});

describe('landmarker options', () => {
  it('tracks one face in video mode from the assets base', () => {
    expect(faceOptions('/a', 'GPU')).toEqual({
      baseOptions: { modelAssetPath: '/a/mediapipe/face_landmarker.task', delegate: 'GPU' },
      runningMode: 'VIDEO',
      numFaces: 1,
    });
  });

  it('tracks one body with the lite pose model in video mode', () => {
    expect(poseOptions('/a', 'CPU')).toEqual({
      baseOptions: { modelAssetPath: '/a/mediapipe/pose_landmarker_lite.task', delegate: 'CPU' },
      runningMode: 'VIDEO',
      numPoses: 1,
    });
  });

  it('falls back to the CPU delegate when GPU creation fails', async () => {
    const create = vi.fn((d: 'GPU' | 'CPU') =>
      d === 'GPU' ? Promise.reject(new Error('no webgl')) : Promise.resolve(d),
    );
    await expect(withCpuFallback(create)).resolves.toBe('CPU');
    expect(create.mock.calls).toEqual([['GPU'], ['CPU']]);
  });

  it('keeps the GPU delegate when it works', async () => {
    const create = vi.fn((d: 'GPU' | 'CPU') => Promise.resolve(d));
    await expect(withCpuFallback(create)).resolves.toBe('GPU');
    expect(create).toHaveBeenCalledTimes(1);
  });
});

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const flush = (): Promise<void> => new Promise((r) => setTimeout(r, 0));

function handlerSetup(
  opts: { drawThrows?: () => boolean; noCanvas?: boolean; drawSize?: { width: number; height: number } } = {},
) {
  const clock = { t: 0 };
  const posted: FromWorker[] = [];
  const pushed: Float64Array[] = [];
  const engine: Engine = {
    push: (x) => {
      pushed.push(x.slice());
    },
    estimate: () => ({ estimate, pulse: new Float64Array(0) }),
    reset: () => undefined,
  };
  const closeDetectors = vi.fn();
  const detectors: Detectors = {
    face: {
      detect: () => {
        clock.t += 4;
        return [makeFace()];
      },
    },
    pose: { detect: () => POSE },
    close: closeDetectors,
  };
  const engineLoads: ReturnType<typeof deferred<Engine>>[] = [];
  const detectorLoads: ReturnType<typeof deferred<Detectors>>[] = [];
  const loadEngine = vi.fn(() => {
    const d = deferred<Engine>();
    engineLoads.push(d);
    return d.promise;
  });
  const loadDetectors = vi.fn(() => {
    const d = deferred<Detectors>();
    detectorLoads.push(d);
    return d.promise;
  });
  const px = image();
  const draw = vi.fn(() => {
    clock.t += 10;
    if (opts.drawThrows?.()) throw new DOMException('detached', 'InvalidStateError');
    return opts.drawSize;
  });
  const rects: Box[] = [];
  const canvas: FrameCanvas = {
    image: {} as VisionImageForTest,
    draw,
    pixels: (r) => {
      rects.push(r);
      return crop(px, r);
    },
  };
  const handle = createWorkerHandler({
    post: (m) => posted.push(m),
    canvas: opts.noCanvas ? null : canvas,
    loadEngine,
    loadDetectors,
    now: () => clock.t,
    width: WIDTH,
    height: HEIGHT,
  });
  const send = (m: ToWorker): void => {
    handle(m);
  };
  const frame = (tMs: number) => {
    const close = vi.fn();
    send({ type: 'frame', frame: { close } as unknown as VideoFrame, tMs });
    return close;
  };
  const ready = async (): Promise<void> => {
    send({ type: 'init', assetsBaseUrl: '/a' });
    engineLoads.at(-1)?.resolve(engine);
    detectorLoads.at(-1)?.resolve(detectors);
    await flush();
  };
  return {
    clock,
    posted,
    pushed,
    rects,
    engine,
    detectors,
    closeDetectors,
    engineLoads,
    detectorLoads,
    loadEngine,
    loadDetectors,
    send,
    frame,
    ready,
  };
}

type VisionImageForTest = FrameCanvas['image'];

describe('frameHeightFor', () => {
  it('keeps the camera aspect ratio at a fixed width', () => {
    expect(frameHeightFor(1280, 720)).toBe(360);
    expect(frameHeightFor(640, 480)).toBe(480);
    expect(frameHeightFor(0, 0)).toBe(480);
  });
});

describe('createWorkerHandler', () => {
  it('locates the skin region on the size the frame was drawn at', async () => {
    const h = handlerSetup({ drawSize: { width: WIDTH, height: 40 } });
    await h.ready();
    h.frame(0);
    expect(h.rects[0]?.y).toBe(8);
  });

  it('frees and acknowledges a frame that arrives before ready', () => {
    const h = handlerSetup();
    const close = h.frame(0);
    expect(close).toHaveBeenCalledTimes(1);
    expect(h.posted).toEqual([{ type: 'frameDone' }]);
  });

  it('loads engine and detectors from the assets base, then draws, closes and samples each frame', async () => {
    const h = handlerSetup();
    await h.ready();
    expect(h.loadEngine).toHaveBeenCalledWith('/a');
    expect(h.loadDetectors).toHaveBeenCalledWith('/a');
    expect(h.posted).toEqual([{ type: 'ready' }]);
    const close = h.frame(100);
    expect(close).toHaveBeenCalledTimes(1);
    expect(h.pushed).toHaveLength(1);
    expect(h.pushed[0]?.[SAMPLE.face]).toBe(1);
    expect(h.posted.at(-1)).toEqual({ type: 'frameDone' });
  });

  it('closes the frame and posts frameDone when drawing throws, reporting vision_failed once', async () => {
    const h = handlerSetup({ drawThrows: () => true });
    await h.ready();
    const c1 = h.frame(0);
    const c2 = h.frame(33);
    expect(c1).toHaveBeenCalledTimes(1);
    expect(c2).toHaveBeenCalledTimes(1);
    expect(h.pushed).toHaveLength(0);
    expect(h.posted).toEqual([
      { type: 'ready' },
      { type: 'error', code: 'vision_failed', message: 'detached' },
      { type: 'frameDone' },
      { type: 'frameDone' },
    ]);
  });

  it('times the whole frame handler, drawing included, for frameCostMs', async () => {
    const h = handlerSetup();
    await h.ready();
    h.clock.t = 900;
    h.frame(0); // draw 10 ms + detect 4 ms
    h.clock.t = 990;
    h.frame(33); // crosses 1 s and reports the previous frames
    const diag = h.posted.find((m) => m.type === 'diagnostics');
    expect(diag?.diagnostics.frameCostMs).toBe(14);
  });

  it('reports wasm_failed, closes loaded detectors and allows a retry', async () => {
    const h = handlerSetup();
    h.send({ type: 'init', assetsBaseUrl: '/a' });
    h.engineLoads[0]?.reject(new Error('bad wasm'));
    h.detectorLoads[0]?.resolve(h.detectors);
    await flush();
    expect(h.posted).toEqual([{ type: 'error', code: 'wasm_failed', message: 'bad wasm' }]);
    expect(h.closeDetectors).toHaveBeenCalledTimes(1);
    await h.ready();
    expect(h.loadEngine).toHaveBeenCalledTimes(2);
    expect(h.posted.at(-1)).toEqual({ type: 'ready' });
  });

  it('reports vision_failed and reuses the loaded engine on retry', async () => {
    const h = handlerSetup();
    h.send({ type: 'init', assetsBaseUrl: '/a' });
    h.engineLoads[0]?.resolve(h.engine);
    h.detectorLoads[0]?.reject(new Error('no model'));
    await flush();
    expect(h.posted).toEqual([{ type: 'error', code: 'vision_failed', message: 'no model' }]);
    h.send({ type: 'init', assetsBaseUrl: '/a' });
    h.detectorLoads[1]?.resolve(h.detectors);
    await flush();
    expect(h.loadEngine).toHaveBeenCalledTimes(1);
    expect(h.loadDetectors).toHaveBeenCalledTimes(2);
    expect(h.posted.at(-1)).toEqual({ type: 'ready' });
  });

  it('ignores a second init while loading and answers ready again once loaded', async () => {
    const h = handlerSetup();
    h.send({ type: 'init', assetsBaseUrl: '/a' });
    h.send({ type: 'init', assetsBaseUrl: '/a' });
    expect(h.loadEngine).toHaveBeenCalledTimes(1);
    h.engineLoads[0]?.resolve(h.engine);
    h.detectorLoads[0]?.resolve(h.detectors);
    await flush();
    h.send({ type: 'init', assetsBaseUrl: '/a' });
    await flush();
    expect(h.loadEngine).toHaveBeenCalledTimes(1);
    expect(h.posted).toEqual([{ type: 'ready' }, { type: 'ready' }]);
  });

  it('reports vision_failed without a 2d canvas and loads nothing', async () => {
    const h = handlerSetup({ noCanvas: true });
    h.send({ type: 'init', assetsBaseUrl: '/a' });
    await flush();
    expect(h.loadEngine).not.toHaveBeenCalled();
    expect(h.posted).toEqual([{ type: 'error', code: 'vision_failed', message: '2d context unavailable' }]);
  });
});

describe('allOrClose', () => {
  it('returns every value when all resolve', async () => {
    await expect(allOrClose([Promise.resolve({ v: 1, close: vi.fn() })])).resolves.toHaveLength(1);
  });

  it('closes the ones that loaded and rethrows the first failure', async () => {
    const loaded = { close: vi.fn() };
    await expect(allOrClose([Promise.resolve(loaded), Promise.reject(new Error('pose failed'))])).rejects.toThrow(
      'pose failed',
    );
    expect(loaded.close).toHaveBeenCalledTimes(1);
  });
});
