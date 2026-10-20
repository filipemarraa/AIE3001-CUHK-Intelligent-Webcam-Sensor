import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createMockVitals } from './mock.js';
import type { Estimate, VitalsError, VitalsState } from './types.js';

const video = {} as HTMLVideoElement;

function record(seed = 1, warmupS = 10) {
  const vitals = createMockVitals({ seed, warmupS });
  const states: VitalsState[] = [];
  const estimates: Estimate[] = [];
  vitals.on('state', (s) => states.push(s));
  vitals.on('estimate', (e) => estimates.push(e));
  return { vitals, states, estimates };
}

describe('createMockVitals', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('goes from starting to calibrating to measuring after the warm-up', async () => {
    const { vitals, states } = record();
    await vitals.start({ video });
    expect(states).toEqual(['starting', 'calibrating']);

    vi.advanceTimersByTime(9_000);
    expect(states).toEqual(['starting', 'calibrating']);

    vi.advanceTimersByTime(1_000);
    expect(states).toEqual(['starting', 'calibrating', 'measuring']);
  });

  it('emits one estimate per second with no value during the warm-up', async () => {
    const { vitals, estimates } = record();
    await vitals.start({ video });
    vi.advanceTimersByTime(5_000);

    expect(estimates.map((e) => e.tMs)).toEqual([1_000, 2_000, 3_000, 4_000, 5_000]);
    for (const e of estimates) {
      expect(e.hrBpm).toBeNull();
      expect(e.rrRpm).toBeNull();
      expect(e.confidence).toBe('invalid');
      expect(e.reason).toBe('warming_up');
    }
  });

  it('only reports values with a confidence and only nulls with a reason', async () => {
    const { vitals, estimates } = record();
    await vitals.start({ video });
    vi.advanceTimersByTime(120_000);

    const measuring = estimates.slice(10);
    expect(measuring.length).toBe(110);
    for (const e of measuring) {
      expect(e.quality).toBeGreaterThanOrEqual(0);
      expect(e.quality).toBeLessThanOrEqual(1);
      if (e.hrBpm === null) {
        expect(e.confidence).toBe('invalid');
        expect(e.reason).not.toBeNull();
      } else {
        expect(e.confidence).not.toBe('invalid');
        expect(e.hrBpm).toBeGreaterThanOrEqual(40);
        expect(e.hrBpm).toBeLessThanOrEqual(180);
        expect(e.rrRpm).toBeGreaterThanOrEqual(4);
        expect(e.rrRpm).toBeLessThanOrEqual(30);
        expect(['in', 'out', 'hold']).toContain(e.breathPhase);
      }
    }
    expect(measuring.some((e) => e.reason === 'too_much_motion')).toBe(true);
    expect(measuring.some((e) => e.confidence === 'high')).toBe(true);
  });

  it('is deterministic for a given seed', async () => {
    const a = record(42);
    const b = record(42);
    await a.vitals.start({ video });
    await b.vitals.start({ video });
    vi.advanceTimersByTime(30_000);
    expect(a.estimates).toEqual(b.estimates);
  });

  it('stops emitting after stop and summarises the session', async () => {
    const { vitals, states, estimates } = record();
    expect(vitals.summary()).toBeNull();

    await vitals.start({ video });
    vi.advanceTimersByTime(60_000);
    vitals.stop();
    const count = estimates.length;
    vi.advanceTimersByTime(10_000);

    expect(estimates.length).toBe(count);
    expect(states.at(-1)).toBe('stopped');
    const summary = vitals.summary();
    expect(summary).not.toBeNull();
    expect(summary?.durationS).toBe(60);
    expect(summary?.method).toBe('pos');
    expect(summary?.hrStart).toBeTypeOf('number');
    expect(summary?.hrEnd).toBeTypeOf('number');
    expect(summary?.rrMean).toBeTypeOf('number');
    expect(summary?.qualityMean).toBeTypeOf('number');
  });

  it('stops calling a handler after unsubscribe', async () => {
    const vitals = createMockVitals({ seed: 1 });
    const handler = vi.fn();
    const off = vitals.on('estimate', handler);
    await vitals.start({ video });
    vi.advanceTimersByTime(2_000);
    off();
    vi.advanceTimersByTime(2_000);
    expect(handler).toHaveBeenCalledTimes(2);
  });

  it('ignores a second start while running', async () => {
    const { vitals, states, estimates } = record();
    await vitals.start({ video });
    await vitals.start({ video });
    vi.advanceTimersByTime(3_000);
    expect(states).toEqual(['starting', 'calibrating']);
    expect(estimates.length).toBe(3);
  });

  it('can simulate a start failure', async () => {
    const vitals = createMockVitals({ failWith: 'camera_denied' });
    const errors: VitalsError[] = [];
    const states: VitalsState[] = [];
    vitals.on('error', (e) => errors.push(e));
    vitals.on('state', (s) => states.push(s));

    await expect(vitals.start({ video })).rejects.toEqual({ code: 'camera_denied' });
    expect(errors).toEqual([{ code: 'camera_denied' }]);
    expect(states).toEqual(['starting', 'stopped']);
  });
});
