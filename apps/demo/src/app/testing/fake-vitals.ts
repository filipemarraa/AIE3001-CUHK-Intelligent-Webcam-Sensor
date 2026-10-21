import { vi } from 'vitest';
import type { Estimate, ExerciseSummary, Unsubscribe, Vitals, VitalsEvents } from 'vitals-web';

type HandlerSets = { [K in keyof VitalsEvents]: Set<(payload: VitalsEvents[K]) => void> };

export class FakeVitals implements Vitals {
  readonly handlers: HandlerSets = {
    estimate: new Set(),
    diagnostics: new Set(),
    state: new Set(),
    error: new Set(),
  };
  summaryValue: ExerciseSummary | null = null;
  readonly start = vi.fn<Vitals['start']>(() => Promise.resolve());
  readonly stop = vi.fn(() => {
    this.emit('state', 'stopped');
  });
  readonly summary = vi.fn(() => this.summaryValue);

  on<K extends keyof VitalsEvents>(event: K, handler: (payload: VitalsEvents[K]) => void): Unsubscribe {
    this.handlers[event].add(handler);
    return () => {
      this.handlers[event].delete(handler);
    };
  }

  emit<K extends keyof VitalsEvents>(event: K, payload: VitalsEvents[K]): void {
    for (const handler of this.handlers[event]) handler(payload);
  }
}

export function estimate(overrides: Partial<Estimate> = {}): Estimate {
  return {
    tMs: 1000,
    hrBpm: 66,
    rrRpm: 6,
    breathPhase: 'in',
    quality: 0.8,
    confidence: 'high',
    reason: null,
    method: 'pos',
    ...overrides,
  };
}

export const WARMING_UP = estimate({
  hrBpm: null,
  rrRpm: null,
  breathPhase: null,
  quality: 0.3,
  confidence: 'invalid',
  reason: 'warming_up',
});
