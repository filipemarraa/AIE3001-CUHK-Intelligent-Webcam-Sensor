import type { Diagnostics, Estimate } from '../types.js';

export type ToWorker =
  | {
      type: 'init';
      assetsBaseUrl: string;
      record?: boolean;
      capture?: { landmarkEvery?: number; robustPixels?: boolean };
    }
  | { type: 'frame'; frame: VideoFrame; tMs: number }
  | { type: 'reset' };

export type FromWorker =
  | { type: 'ready' }
  | { type: 'frameDone' }
  | { type: 'estimate'; estimate: Estimate }
  | { type: 'diagnostics'; diagnostics: Diagnostics }
  | { type: 'samples'; rows: number[][] }
  | { type: 'error'; code: 'wasm_failed' | 'vision_failed'; message: string };

const FROM_WORKER_TYPES: ReadonlySet<string> = new Set([
  'ready',
  'frameDone',
  'estimate',
  'diagnostics',
  'samples',
  'error',
]);
const FORBIDDEN_GLOBALS = ['ImageBitmap', 'VideoFrame', 'ImageData', 'OffscreenCanvas', 'Blob'] as const;

function isForbidden(v: object): boolean {
  if (v instanceof ArrayBuffer || ArrayBuffer.isView(v)) return true;
  const g = globalThis as Record<string, unknown>;
  return FORBIDDEN_GLOBALS.some((name) => {
    const ctor = g[name];
    return typeof ctor === 'function' && v instanceof (ctor as new () => object);
  });
}

function hasForbidden(v: unknown, seen: WeakSet<object>): boolean {
  if (typeof v !== 'object' || v === null) return false;
  if (seen.has(v)) return false;
  seen.add(v);
  if (isForbidden(v)) return true;
  return Object.values(v).some((child) => hasForbidden(child, seen));
}

function isRowsOfNumbers(rows: unknown): boolean {
  return (
    Array.isArray(rows) &&
    rows.every((row) => Array.isArray(row) && row.every((v) => typeof v === 'number' && Number.isFinite(v)))
  );
}

export function isFromWorker(x: unknown): x is FromWorker {
  if (typeof x !== 'object' || x === null) return false;
  const type = (x as { type?: unknown }).type;
  if (typeof type !== 'string' || !FROM_WORKER_TYPES.has(type)) return false;
  if (type === 'samples' && !isRowsOfNumbers((x as { rows?: unknown }).rows)) return false;
  return !hasForbidden(x, new WeakSet());
}
