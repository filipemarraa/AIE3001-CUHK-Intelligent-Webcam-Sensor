import { describe, expect, it } from 'vitest';
import { parseHeartRateMeasurement } from './heartrate';

const view = (...bytes: number[]): DataView => new DataView(new Uint8Array(bytes).buffer);

describe('parseHeartRateMeasurement (GATT 0x2A37)', () => {
  it('reads an 8-bit heart rate without sensor contact support', () => {
    expect(parseHeartRateMeasurement(view(0x00, 72))).toEqual({ hrBpm: 72, contact: null, rrMs: [] });
  });

  it('reads a 16-bit little-endian heart rate', () => {
    expect(parseHeartRateMeasurement(view(0x01, 0x2c, 0x01))).toEqual({ hrBpm: 300, contact: null, rrMs: [] });
  });

  it('reads the sensor contact status only when the sensor supports it', () => {
    expect(parseHeartRateMeasurement(view(0x06, 65)).contact).toBe(true);
    expect(parseHeartRateMeasurement(view(0x04, 65)).contact).toBe(false);
    expect(parseHeartRateMeasurement(view(0x02, 65)).contact).toBeNull();
  });

  it('reads RR intervals in 1/1024 s after the energy expended field', () => {
    // 8-bit HR 60, energy expended 0x0102, RR 1024 (1000 ms) and 512 (500 ms).
    expect(parseHeartRateMeasurement(view(0x18, 60, 0x02, 0x01, 0x00, 0x04, 0x00, 0x02))).toEqual({
      hrBpm: 60,
      contact: null,
      rrMs: [1000, 500],
    });
  });

  it('reads RR intervals after a 16-bit heart rate with contact detected', () => {
    // Polar H10 style: 16-bit HR 75, contact supported and detected, one RR of 819/1024 s.
    const m = parseHeartRateMeasurement(view(0x17, 75, 0x00, 0x33, 0x03));
    expect(m.hrBpm).toBe(75);
    expect(m.contact).toBe(true);
    expect(m.rrMs).toHaveLength(1);
    expect(m.rrMs[0]).toBeCloseTo((819 * 1000) / 1024, 6);
  });

  it('refuses a measurement shorter than its flags announce', () => {
    expect(() => parseHeartRateMeasurement(view())).toThrow(/too short/);
    expect(() => parseHeartRateMeasurement(view(0x01, 72))).toThrow(/too short/);
    expect(() => parseHeartRateMeasurement(view(0x08, 72, 0x01))).toThrow(/too short/);
  });

  it('ignores a trailing odd byte in the RR intervals', () => {
    expect(parseHeartRateMeasurement(view(0x10, 60, 0x00, 0x04, 0x01)).rrMs).toEqual([1000]);
  });
});
