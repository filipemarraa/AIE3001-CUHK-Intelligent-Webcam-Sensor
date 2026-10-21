import type { Estimate, ExerciseSummary, Method } from './types.js';

export function round(x: number, decimals: number): number {
  const f = 10 ** decimals;
  return Math.round(x * f) / f;
}

function average(xs: number[]): number | null {
  return xs.length === 0 ? null : xs.reduce((a, b) => a + b, 0) / xs.length;
}

export function summarize(history: Estimate[], durationS: number, method: Method): ExerciseSummary {
  const valid = history.filter((e) => e.hrBpm !== null);
  const measured = history.filter((e) => e.reason !== 'warming_up');
  const rrMean = average(valid.flatMap((e) => (e.rrRpm === null ? [] : [e.rrRpm])));
  const qualityMean = average(measured.map((e) => e.quality));
  return {
    hrStart: valid[0]?.hrBpm ?? null,
    hrEnd: valid.at(-1)?.hrBpm ?? null,
    rrMean: rrMean === null ? null : round(rrMean, 1),
    qualityMean: qualityMean === null ? null : round(qualityMean, 2),
    durationS,
    method,
  };
}
