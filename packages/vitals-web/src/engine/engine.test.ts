// @vitest-environment node
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { beforeAll, describe, expect, it } from 'vitest';
import { synthSamples } from '../test/synth';
import { loadEngine, type Engine, type Instantiate } from './engine';
import { SAMPLE } from './layout';

const wasmUrl = new URL('../../assets/engine.wasm', import.meta.url).href;
const wasmExecUrl = new URL('../../assets/wasm_exec.js', import.meta.url).href;

const instantiateFromDisk: Instantiate = (url, imports) =>
  WebAssembly.instantiate(readFileSync(fileURLToPath(url)), imports);

const load = (): Promise<Engine> => loadEngine(wasmUrl, wasmExecUrl, instantiateFromDisk);

function pushAll(engine: Engine, samples: Float64Array[]): number {
  for (const x of samples) engine.push(x);
  return samples.at(-1)?.[SAMPLE.timeMs] ?? 0;
}

describe('engine over real WebAssembly', () => {
  let engine: Engine;
  beforeAll(async () => {
    engine = await load();
  });

  it('estimates 72 bpm from 15 s of synthetic samples', () => {
    engine.reset();
    const now = pushAll(engine, synthSamples({ bpm: 72, durationS: 15 }));
    const { estimate, pulse } = engine.estimate(now);
    expect(estimate.hrBpm).not.toBeNull();
    expect(Math.abs((estimate.hrBpm ?? 0) - 72)).toBeLessThanOrEqual(2);
    expect(estimate.confidence).not.toBe('invalid');
    expect(estimate.method).toBe('pos');
    expect(pulse.length).toBeGreaterThan(0);
  });

  it('estimates breathing and a phase from 30 s of samples', () => {
    engine.reset();
    const now = pushAll(engine, synthSamples({ bpm: 72, rpm: 15, durationS: 30 }));
    const { estimate } = engine.estimate(now);
    expect(Math.abs((estimate.rrRpm ?? 0) - 15)).toBeLessThanOrEqual(2);
    expect(estimate.breathPhase).not.toBeNull();
  });

  it('is invalid with warming_up on an empty session and after reset', () => {
    engine.reset();
    expect(engine.estimate(0).estimate).toMatchObject({ confidence: 'invalid', reason: 'warming_up', hrBpm: null });
    pushAll(engine, synthSamples({ bpm: 72, durationS: 15 }));
    engine.reset();
    expect(engine.estimate(20_000).estimate).toMatchObject({ confidence: 'invalid', hrBpm: null });
  });

  it('drops malformed samples without throwing', () => {
    engine.reset();
    const bad = new Float64Array(SAMPLE.length);
    bad.fill(Number.NaN);
    expect(() => {
      engine.push(bad);
      engine.push(new Float64Array(3));
      engine.push(new Float64Array(0));
    }).not.toThrow();
    expect(engine.estimate(0).estimate.confidence).toBe('invalid');
  });

  it('keeps push and estimate within the round-trip budget', () => {
    engine.reset();
    const samples = synthSamples({ bpm: 72, durationS: 40, seed: 5 });
    const warm = samples.slice(0, 200);
    const timed = samples.slice(200);
    for (const x of warm) engine.push(x);
    for (let i = 0; i < 20; i++) engine.estimate(20_000);

    const t0 = performance.now();
    for (const x of timed) engine.push(x);
    const pushMs = (performance.now() - t0) / timed.length;

    const t1 = performance.now();
    for (let i = 0; i < 50; i++) engine.estimate(30_000);
    const estimateMs = (performance.now() - t1) / 50;

    console.info(`push mean ${pushMs.toFixed(4)} ms, estimate mean ${estimateMs.toFixed(3)} ms`);
    // Wall-clock bounds flake on shared runners: CI only checks sanity. The 2 ms product target is verified on the
    // exhibition laptop (docs/exhibition-checklist.md).
    const ci = process.env['CI'] !== undefined;
    expect(pushMs).toBeLessThan(ci ? 0.5 : 0.05);
    expect(estimateMs).toBeLessThan(ci ? 10 : 2);
  });
});
