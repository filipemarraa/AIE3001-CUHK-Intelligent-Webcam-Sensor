import { TestBed } from '@angular/core/testing';
import { afterEach, describe, expect, it } from 'vitest';
import type { VitalsError, VitalsState } from 'vitals-web';
import { VITALS_FACTORY, createDefaultVitals } from './vitals.service';

function states(search: string): { states: VitalsState[]; started: Promise<unknown> } {
  const vitals = createDefaultVitals(search);
  const seen: VitalsState[] = [];
  vitals.on('state', (s) => seen.push(s));
  const started = vitals.start({ video: document.createElement('video') }).then(
    () => 'resolved',
    (e: unknown) => (e as VitalsError).code,
  );
  return {
    states: seen,
    started: started.finally(() => {
      vitals.stop();
    }),
  };
}

describe('default Vitals factory', () => {
  afterEach(() => {
    history.replaceState(null, '', '/');
  });

  it('uses the mock module when the page has mock=1', async () => {
    const run = states('?mock=1');

    expect(await run.started).toBe('resolved');
    expect(run.states).toContain('calibrating');
  });

  it('uses the real module otherwise (jsdom lacks the camera APIs)', async () => {
    expect(await states('').started).toBe('unsupported_browser');
  });

  it('the injection token reads mock=1 from the page address', async () => {
    history.replaceState(null, '', '/?mock=1');
    const vitals = TestBed.inject(VITALS_FACTORY)();

    await expect(vitals.start({ video: document.createElement('video') })).resolves.toBeUndefined();
    vitals.stop();
  });
});
