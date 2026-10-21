import { describe, expect, it } from 'vitest';
import type { Reason } from 'vitals-web';
import { estimate } from './testing/fake-vitals';
import { instructionText, reasonText } from './reason-text';

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

describe('instructionText', () => {
  it('is empty without an estimate', () => {
    expect(instructionText(null)).toBe('');
  });

  it('shows the warming-up copy for an invalid estimate without a reason', () => {
    expect(instructionText(estimate({ confidence: 'invalid', reason: null }))).toBe('Getting a steady signal…');
  });

  it('keeps the reason when there is one', () => {
    expect(instructionText(estimate({ confidence: 'invalid', reason: 'low_light' }))).toBe('More light, please');
  });

  it('is empty for a valid estimate without a reason', () => {
    expect(instructionText(estimate({ confidence: 'high', reason: null }))).toBe('');
  });
});
