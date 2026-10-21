import { describe, expect, it } from 'vitest';
import type { Reason } from 'vitals-web';
import { reasonText } from './reason-text';

describe('reasonText', () => {
  const cases: [Reason | null, string][] = [
    ['no_face', 'Move into the frame'],
    ['too_much_motion', 'Hold still'],
    ['low_light', 'More light, please'],
    ['talking', 'Try not to talk'],
    ['warming_up', 'Getting a steady signal…'],
    [null, ''],
  ];

  for (const [reason, text] of cases) {
    it(`${String(reason)} reads "${text}"`, () => {
      expect(reasonText(reason)).toBe(text);
    });
  }
});
