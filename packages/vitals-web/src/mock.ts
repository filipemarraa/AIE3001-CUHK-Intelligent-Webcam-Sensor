import { round, summarize } from './summary.js';
import type {
  BreathPhase,
  Confidence,
  Estimate,
  ExerciseSummary,
  Method,
  Unsubscribe,
  Vitals,
  VitalsError,
  VitalsErrorCode,
  VitalsEvents,
  VitalsOptions,
} from './types.js';

export interface MockVitalsOptions extends Partial<VitalsOptions> {
  seed?: number;
  warmupS?: number;
  failWith?: VitalsErrorCode;
}

type HandlerSets = { [K in keyof VitalsEvents]: Set<(payload: VitalsEvents[K]) => void> };

const MOTION_EVERY_S = 45;
const MOTION_LENGTH_S = 5;
const BREATH_PERIOD_S = 10;
const INHALE_S = 4;

function mulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function confidenceFor(quality: number): Confidence {
  if (quality >= 0.75) return 'high';
  if (quality >= 0.6) return 'medium';
  return 'low';
}

export function createMockVitals(options: MockVitalsOptions = {}): Vitals {
  const method: Method = options.method ?? 'pos';
  const warmupS = options.warmupS ?? 10;
  if (!Number.isInteger(warmupS) || warmupS < 1) {
    throw new RangeError('warmupS must be a positive integer number of seconds');
  }
  const handlers: HandlerSets = {
    estimate: new Set(),
    diagnostics: new Set(),
    state: new Set(),
    error: new Set(),
  };
  let timer: ReturnType<typeof setInterval> | null = null;
  let running = false;
  let session = 0;
  let history: Estimate[] = [];
  let elapsedS = 0;
  let started = false;
  let random = mulberry32(options.seed ?? 1);

  function emit<K extends keyof VitalsEvents>(event: K, payload: VitalsEvents[K]): void {
    for (const handler of handlers[event]) handler(payload);
  }

  function warmupEstimate(s: number): Estimate {
    return {
      tMs: s * 1000,
      hrBpm: null,
      rrRpm: null,
      breathPhase: null,
      quality: round(0.2 + (0.4 * s) / warmupS, 2),
      confidence: 'invalid',
      reason: 'warming_up',
      method,
    };
  }

  function measuringEstimate(s: number): Estimate {
    const m = s - warmupS;
    if (m % MOTION_EVERY_S >= MOTION_EVERY_S - MOTION_LENGTH_S) {
      return {
        tMs: s * 1000,
        hrBpm: null,
        rrRpm: null,
        breathPhase: null,
        quality: round(0.15 + 0.1 * random(), 2),
        confidence: 'invalid',
        reason: 'too_much_motion',
        method,
      };
    }
    const breathPhase: BreathPhase = m % BREATH_PERIOD_S < INHALE_S ? 'in' : 'out';
    const hr = 64 + 8 * Math.exp(-m / 60) + 3 * Math.sin((2 * Math.PI * m) / BREATH_PERIOD_S) + (random() - 0.5) * 1.5;
    const quality = 0.55 + 0.4 * random();
    return {
      tMs: s * 1000,
      hrBpm: round(hr, 1),
      rrRpm: round(60 / BREATH_PERIOD_S + (random() - 0.5) * 0.3, 1),
      breathPhase,
      quality: round(quality, 2),
      confidence: confidenceFor(quality),
      reason: null,
      method,
    };
  }

  function tick(): void {
    const id = session;
    elapsedS += 1;
    const estimate = elapsedS <= warmupS ? warmupEstimate(elapsedS) : measuringEstimate(elapsedS);
    history.push(estimate);
    emit('estimate', estimate);
    if (id === session) emit('diagnostics', { frameCostMs: 0, droppedFrames: 0, pulse: [] });
    if (id === session && elapsedS === warmupS) emit('state', 'measuring');
  }

  return {
    start(): Promise<void> {
      if (running) return Promise.resolve();
      running = true;
      const id = ++session;
      emit('state', 'starting');
      if (id !== session) return Promise.resolve();
      if (options.failWith) {
        running = false;
        const error: VitalsError = { code: options.failWith };
        emit('error', error);
        emit('state', 'stopped');
        // VitalsError is the module's typed rejection contract, not an Error subclass.
        // eslint-disable-next-line @typescript-eslint/prefer-promise-reject-errors
        return Promise.reject(error);
      }
      started = true;
      history = [];
      elapsedS = 0;
      random = mulberry32(options.seed ?? 1);
      emit('state', 'calibrating');
      if (id === session) timer = setInterval(tick, 1000);
      return Promise.resolve();
    },

    stop(): void {
      if (!running) return;
      running = false;
      session += 1;
      if (timer !== null) clearInterval(timer);
      timer = null;
      emit('state', 'stopped');
    },

    on<K extends keyof VitalsEvents>(event: K, handler: (payload: VitalsEvents[K]) => void): Unsubscribe {
      handlers[event].add(handler);
      return () => {
        handlers[event].delete(handler);
      };
    },

    summary(): ExerciseSummary | null {
      return started ? summarize(history, elapsedS, method) : null;
    },
  };
}
