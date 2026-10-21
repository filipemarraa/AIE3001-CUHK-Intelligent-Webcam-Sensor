import { openCamera, startFrameLoop } from './camera.js';
import { summarize } from './summary.js';
import type {
  Estimate,
  ExerciseSummary,
  Unsubscribe,
  Vitals,
  VitalsError,
  VitalsEvents,
  VitalsOptions,
} from './types.js';
import { isFromWorker, type FromWorker, type ToWorker } from './worker/protocol.js';

export interface VitalsDeps {
  createWorker(): Worker;
  media?: MediaDevices;
}

type HandlerSets = { [K in keyof VitalsEvents]: Set<(payload: VitalsEvents[K]) => void> };

interface Session {
  video: HTMLVideoElement;
  started: Promise<void>;
  resolve: () => void;
  reject: (e: VitalsError) => void;
  worker: Worker | null;
  stream: MediaStream | null;
  stopLoop: (() => void) | null;
  busy: boolean;
  dropped: number;
  measuring: boolean;
}

const defaultDeps: VitalsDeps = {
  createWorker: () => new Worker(new URL('./worker/sensor.worker.js', import.meta.url), { type: 'module' }),
};

function supported(): boolean {
  const g = globalThis as Record<string, unknown>;
  const video = g['HTMLVideoElement'] as { prototype?: { requestVideoFrameCallback?: unknown } } | undefined;
  return (
    typeof video?.prototype?.requestVideoFrameCallback === 'function' &&
    typeof g['VideoFrame'] === 'function' &&
    typeof g['OffscreenCanvas'] === 'function' &&
    typeof g['Worker'] === 'function'
  );
}

function absoluteBase(url: string): string {
  const page = (globalThis as { location?: { href: string } }).location?.href;
  return new URL(url, page).href.replace(/\/$/, '');
}

function pageWindow(): EventTarget | undefined {
  return (globalThis as { window?: EventTarget }).window;
}

export function createVitals(options: VitalsOptions, deps: VitalsDeps = defaultDeps): Vitals {
  const assetsBaseUrl = absoluteBase(options.assetsBaseUrl);
  const handlers: HandlerSets = {
    estimate: new Set(),
    diagnostics: new Set(),
    state: new Set(),
    error: new Set(),
  };
  let current: Session | null = null;
  let history: Estimate[] = [];
  let calibratingAtMs: number | null = null;
  let stoppedAtMs: number | null = null;

  function emit<K extends keyof VitalsEvents>(event: K, payload: VitalsEvents[K]): void {
    for (const handler of handlers[event]) handler(payload);
  }

  const onPageHide = (): void => {
    stop();
  };

  const onTrackEnded = (): void => {
    if (current !== null) fail(current, { code: 'camera_unavailable', message: 'camera disconnected' });
  };

  function endStream(stream: MediaStream, video: HTMLVideoElement): void {
    for (const track of stream.getTracks()) {
      track.removeEventListener('ended', onTrackEnded);
      track.stop();
    }
    if (video.srcObject === stream) video.srcObject = null;
  }

  function teardown(s: Session): void {
    current = null;
    pageWindow()?.removeEventListener('pagehide', onPageHide);
    s.stopLoop?.();
    s.worker?.terminate();
    if (s.stream !== null) endStream(s.stream, s.video);
    s.video.srcObject = null;
    if (calibratingAtMs !== null && stoppedAtMs === null) stoppedAtMs = performance.now();
  }

  function fail(s: Session, error: VitalsError): void {
    if (s !== current) {
      s.resolve();
      return;
    }
    teardown(s);
    emit('error', error);
    emit('state', 'stopped');
    s.reject(error);
  }

  function stop(): void {
    const s = current;
    if (s === null) return;
    teardown(s);
    emit('state', 'stopped');
    s.resolve();
  }

  function onWorkerMessage(s: Session, m: FromWorker, ready: () => void): void {
    switch (m.type) {
      case 'ready':
        ready();
        break;
      case 'frameDone':
        s.busy = false;
        break;
      case 'estimate':
        history.push(m.estimate);
        emit('estimate', m.estimate);
        if (s === current && !s.measuring && m.estimate.confidence !== 'invalid') {
          s.measuring = true;
          emit('state', 'measuring');
        }
        break;
      case 'diagnostics':
        emit('diagnostics', { ...m.diagnostics, droppedFrames: s.dropped });
        s.dropped = 0;
        break;
      case 'error':
        fail(s, { code: m.code, message: m.message });
        break;
    }
  }

  function spawnWorker(s: Session): Promise<void> {
    return new Promise((resolve) => {
      const worker = deps.createWorker();
      s.worker = worker;
      worker.onmessage = (e: MessageEvent<unknown>) => {
        if (s !== current || !isFromWorker(e.data)) return;
        onWorkerMessage(s, e.data, resolve);
      };
      // fail() rejects start(); this promise then never settles and the pending Promise.all is dropped.
      worker.onerror = () => {
        fail(s, { code: 'wasm_failed', message: 'sensor worker failed to load' });
      };
      worker.postMessage({ type: 'init', assetsBaseUrl } satisfies ToWorker);
    });
  }

  async function camera(s: Session): Promise<void> {
    const stream = await openCamera(s.video, deps.media);
    if (s !== current) {
      endStream(stream, s.video);
      return;
    }
    s.stream = stream;
    for (const track of stream.getTracks()) track.addEventListener('ended', onTrackEnded);
  }

  function frameLoop(s: Session): () => void {
    return startFrameLoop(
      s.video,
      (frame, tMs) => {
        s.worker?.postMessage({ type: 'frame', frame, tMs } satisfies ToWorker, [frame]);
        s.busy = true;
      },
      () => s.busy,
      () => {
        s.dropped += 1;
      },
    );
  }

  async function run(s: Session): Promise<void> {
    emit('state', 'starting');
    if (s !== current) {
      s.resolve();
      return;
    }
    if (!supported()) {
      fail(s, {
        code: 'unsupported_browser',
        message: 'needs requestVideoFrameCallback, VideoFrame, OffscreenCanvas, Worker',
      });
      return;
    }
    pageWindow()?.addEventListener('pagehide', onPageHide);
    try {
      await Promise.all([camera(s), spawnWorker(s)]);
    } catch (e) {
      fail(s, e as VitalsError);
      return;
    }
    if (s !== current) {
      s.resolve();
      return;
    }
    history = [];
    calibratingAtMs = performance.now();
    stoppedAtMs = null;
    s.stopLoop = frameLoop(s);
    emit('state', 'calibrating');
    s.resolve();
  }

  return {
    start({ video }): Promise<void> {
      if (current !== null) return current.started;
      let resolve!: () => void;
      let reject!: (e: VitalsError) => void;
      const started = new Promise<void>((res, rej) => {
        resolve = res;
        reject = rej;
      });
      const s: Session = {
        video,
        started,
        resolve,
        reject,
        worker: null,
        stream: null,
        stopLoop: null,
        busy: false,
        dropped: 0,
        measuring: false,
      };
      current = s;
      void run(s);
      return started;
    },

    stop,

    on<K extends keyof VitalsEvents>(event: K, handler: (payload: VitalsEvents[K]) => void): Unsubscribe {
      handlers[event].add(handler);
      return () => {
        handlers[event].delete(handler);
      };
    },

    summary(): ExerciseSummary | null {
      if (calibratingAtMs === null) return null;
      const durationS = Math.floor(((stoppedAtMs ?? performance.now()) - calibratingAtMs) / 1000);
      return summarize(history, durationS, 'pos');
    },
  };
}
