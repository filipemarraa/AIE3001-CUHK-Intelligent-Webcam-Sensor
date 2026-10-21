import { describe, expect, it } from 'vitest';
import { decodeEstimate } from './decode';
import { OUT } from './layout';

function header(values: Partial<Record<keyof typeof OUT, number>>): Float64Array {
  const out = new Float64Array(OUT.maxLength);
  out[OUT.timeMs] = values.timeMs ?? 1000;
  out[OUT.hr] = values.hr ?? Number.NaN;
  out[OUT.rr] = values.rr ?? Number.NaN;
  out[OUT.phase] = values.phase ?? 0;
  out[OUT.quality] = values.quality ?? 0;
  out[OUT.confidence] = values.confidence ?? 0;
  out[OUT.reason] = values.reason ?? 0;
  out[OUT.pulseLen] = values.pulseLen ?? 0;
  return out;
}

describe('decodeEstimate', () => {
  it('decodes a valid estimate with a NaN respiratory rate', () => {
    const out = header({ hr: 72.4, phase: 1, quality: 0.8, pulseLen: 3 });
    out.set([0.1, -0.2, 0.3], OUT.pulse);
    const { estimate, pulse } = decodeEstimate(out);
    expect(estimate).toEqual({
      tMs: 1000,
      hrBpm: 72.4,
      rrRpm: null,
      breathPhase: 'in',
      quality: 0.8,
      confidence: 'high',
      reason: null,
      method: 'pos',
    });
    expect(Array.from(pulse)).toEqual([0.1, -0.2, 0.3]);
  });

  it('nulls hr, rr and phase when invalid', () => {
    const out = header({ hr: 70, rr: 15, phase: 2, confidence: 3, reason: 2 });
    const { estimate, pulse } = decodeEstimate(out);
    expect(estimate).toMatchObject({
      hrBpm: null,
      rrRpm: null,
      breathPhase: null,
      confidence: 'invalid',
      reason: 'too_much_motion',
    });
    expect(pulse.length).toBe(0);
  });

  it.each([
    [0, null],
    [1, 'in'],
    [2, 'out'],
    [3, 'hold'],
  ])('maps phase code %d to %s', (code, phase) => {
    expect(decodeEstimate(header({ hr: 70, rr: 15, phase: code, confidence: 1 })).estimate.breathPhase).toBe(phase);
  });

  it.each([
    [1, 'no_face'],
    [2, 'too_much_motion'],
    [3, 'low_light'],
    [4, 'talking'],
    [5, 'warming_up'],
  ])('maps reason code %d to %s', (code, reason) => {
    expect(decodeEstimate(header({ confidence: 3, reason: code })).estimate.reason).toBe(reason);
  });

  it('copies the pulse so the output buffer can be reused', () => {
    const out = header({ hr: 70, pulseLen: 2 });
    out.set([1, 2], OUT.pulse);
    const { pulse } = decodeEstimate(out);
    out.fill(0);
    expect(Array.from(pulse)).toEqual([1, 2]);
  });

  it('clamps a pulse length larger than the buffer', () => {
    const out = header({ hr: 70, pulseLen: 9999 });
    expect(decodeEstimate(out).pulse.length).toBe(OUT.maxLength - OUT.pulse);
  });
});
