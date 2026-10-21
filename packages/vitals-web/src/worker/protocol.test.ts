import { describe, expect, it, vi } from 'vitest';
import type { Estimate } from '../types.js';
import { isFromWorker } from './protocol.js';

const estimate: Estimate = {
  tMs: 1000,
  hrBpm: 72,
  rrRpm: 14,
  breathPhase: 'in',
  quality: 0.8,
  confidence: 'high',
  reason: null,
  method: 'pos',
};

describe('isFromWorker', () => {
  it('accepts every valid message', () => {
    expect(isFromWorker({ type: 'ready' })).toBe(true);
    expect(isFromWorker({ type: 'frameDone' })).toBe(true);
    expect(isFromWorker({ type: 'estimate', estimate })).toBe(true);
    expect(
      isFromWorker({ type: 'diagnostics', diagnostics: { frameCostMs: 3, droppedFrames: 0, pulse: [0.1, 0.2] } }),
    ).toBe(true);
    expect(isFromWorker({ type: 'error', code: 'wasm_failed', message: 'x' })).toBe(true);
    expect(isFromWorker({ type: 'samples', rows: [[1, 2, 3]] })).toBe(true);
  });

  it('rejects recorded samples that carry a typed array', () => {
    expect(isFromWorker({ type: 'samples', rows: [new Float64Array(3)] })).toBe(false);
  });

  it('accepts recorded samples only as rows of finite numbers', () => {
    expect(isFromWorker({ type: 'samples' })).toBe(false);
    expect(isFromWorker({ type: 'samples', rows: 'x' })).toBe(false);
    expect(isFromWorker({ type: 'samples', rows: [['1', 2]] })).toBe(false);
    expect(isFromWorker({ type: 'samples', rows: [[{ r: 1 }]] })).toBe(false);
    expect(isFromWorker({ type: 'samples', rows: [[1, Number.NaN]] })).toBe(false);
    expect(isFromWorker({ type: 'samples', rows: [[1, Number.POSITIVE_INFINITY]] })).toBe(false);
    expect(isFromWorker({ type: 'samples', rows: [] })).toBe(true);
  });

  it('rejects unknown type and non-objects', () => {
    expect(isFromWorker({ type: 'nope' })).toBe(false);
    expect(isFromWorker({})).toBe(false);
    expect(isFromWorker(null)).toBe(false);
    expect(isFromWorker('ready')).toBe(false);
  });

  it('rejects typed arrays at any depth', () => {
    expect(isFromWorker({ type: 'estimate', estimate, extra: new Uint8Array(4) })).toBe(false);
    expect(isFromWorker({ type: 'estimate', estimate: { ...estimate, nested: { deep: [new Float32Array(2)] } } })).toBe(
      false,
    );
    expect(isFromWorker({ type: 'ready', view: new DataView(new ArrayBuffer(2)) })).toBe(false);
  });

  it('rejects ArrayBuffer and Blob', () => {
    expect(isFromWorker({ type: 'ready', buf: new ArrayBuffer(8) })).toBe(false);
    expect(isFromWorker({ type: 'ready', blob: new Blob(['x']) })).toBe(false);
  });

  it.each(['VideoFrame', 'ImageBitmap', 'ImageData', 'OffscreenCanvas'])(
    'rejects %s when the global exists',
    (name) => {
      const Fake = function Fake() {} as unknown as new () => object;
      vi.stubGlobal(name, Fake);
      try {
        expect(isFromWorker({ type: 'ready', x: { y: new Fake() } })).toBe(false);
      } finally {
        vi.unstubAllGlobals();
      }
    },
  );

  it('survives cyclic objects', () => {
    const a: Record<string, unknown> = { type: 'ready' };
    a['self'] = a;
    expect(isFromWorker(a)).toBe(true);
  });
});
