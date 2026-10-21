import type { BreathPhase, Confidence, Estimate, Reason } from '../types.js';
import { OUT } from './layout.js';

const PHASES: readonly (BreathPhase | null)[] = [null, 'in', 'out', 'hold'];
const CONFIDENCES: readonly Confidence[] = ['high', 'medium', 'low', 'invalid'];
const REASONS: readonly (Reason | null)[] = [null, 'no_face', 'too_much_motion', 'low_light', 'talking', 'warming_up'];

export interface Decoded {
  estimate: Estimate;
  pulse: Float64Array;
}

function finiteOrNull(v: number | undefined): number | null {
  return v !== undefined && Number.isFinite(v) ? v : null;
}

export function decodeEstimate(out: Float64Array): Decoded {
  const confidence = CONFIDENCES[out[OUT.confidence] ?? 3] ?? 'invalid';
  const valid = confidence !== 'invalid';
  const pulseLen = Math.max(0, Math.min(out[OUT.pulseLen] ?? 0, out.length - OUT.pulse));
  return {
    estimate: {
      tMs: out[OUT.timeMs] ?? 0,
      hrBpm: valid ? finiteOrNull(out[OUT.hr]) : null,
      rrRpm: valid ? finiteOrNull(out[OUT.rr]) : null,
      breathPhase: valid ? (PHASES[out[OUT.phase] ?? 0] ?? null) : null,
      quality: out[OUT.quality] ?? 0,
      confidence,
      reason: REASONS[out[OUT.reason] ?? 0] ?? null,
      method: 'pos',
    },
    pulse: out.slice(OUT.pulse, OUT.pulse + pulseLen),
  };
}
