import { describe, expect, it } from 'vitest';
import { breathingRateText, heartRateText } from './format';

describe('format', () => {
  it('rounds the heart rate to an integer', () => {
    expect(heartRateText(66.4)).toBe('66');
    expect(heartRateText(66.5)).toBe('67');
  });

  it('rounds the breathing rate to one decimal', () => {
    expect(breathingRateText(6)).toBe('6.0');
    expect(breathingRateText(6.04)).toBe('6.0');
    expect(breathingRateText(6.26)).toBe('6.3');
  });

  it('shows a dash for a missing value', () => {
    expect(heartRateText(null)).toBe('—');
    expect(heartRateText(undefined)).toBe('—');
    expect(breathingRateText(null)).toBe('—');
    expect(breathingRateText(undefined)).toBe('—');
  });
});
