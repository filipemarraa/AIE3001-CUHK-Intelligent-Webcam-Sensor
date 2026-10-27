import { describe, expect, it } from 'vitest';
import { diagnosticsLine, estimateLine, headerLine, parseHeartRate, referenceLine, sampleLines } from './jsonl';

const parse = (line: string): unknown => JSON.parse(line);

describe('jsonl', () => {
  it('writes a version 1 header with the sample layout and the order of the skin regions', () => {
    expect(
      parse(
        headerLine({
          subject: 'P1',
          condition: 'good light',
          options: { skin: 'anatomical' },
          camera: { label: 'cam', width: 640, height: 480, frameRate: 30 },
          startedAt: '2026-10-24T10:00:00.000Z',
        }),
      ),
    ).toEqual({
      kind: 'header',
      version: 1,
      subject: 'P1',
      condition: 'good light',
      regions: ['boxes', 'anatomical'],
      layout: ['timeMs', 'r', 'g', 'b', 'face', 'motion', 'light', 'mouth', 'shoulderY', 'shoulderW'],
      options: { skin: 'anatomical' },
      camera: { label: 'cam', width: 640, height: 480, frameRate: 30 },
      startedAt: '2026-10-24T10:00:00.000Z',
    });
  });

  it('writes one sample line per row with its skin colours', () => {
    const rows = [Array.from({ length: 10 }, (_, i) => i), Array.from({ length: 10 }, (_, i) => i + 10)];
    const skins = [
      [1, 2, 3, 4, 5, 6],
      [7, 8, 9, 10, 11, 12],
    ];
    expect(sampleLines(rows, skins).map(parse)).toEqual([
      { kind: 'sample', v: rows[0], skins: skins[0] },
      { kind: 'sample', v: rows[1], skins: skins[1] },
    ]);
  });

  it('writes zero skin colours when the worker sent none', () => {
    const row = Array.from({ length: 10 }, () => 1);
    expect(parse(sampleLines([row])[0] as string)).toEqual({ kind: 'sample', v: row, skins: [0, 0, 0, 0, 0, 0] });
  });

  it('refuses rows that do not follow the layout', () => {
    expect(() => sampleLines([[1, 2]])).toThrow(/10 values/);
    expect(() => sampleLines([Array.from({ length: 10 }, () => 1)], [[1, 2, 3]])).toThrow(/6 skin colours/);
  });

  it('writes estimates, diagnostics without the pulse, and references', () => {
    const e = {
      tMs: 1000,
      hrBpm: null,
      rrRpm: 9.5,
      breathPhase: 'in' as const,
      quality: 0.4,
      confidence: 'invalid' as const,
      reason: 'warming_up' as const,
      method: 'pos' as const,
    };
    expect(parse(estimateLine(e))).toEqual({ kind: 'estimate', e });
    expect(parse(diagnosticsLine({ frameCostMs: 21, droppedFrames: 2, pulse: [0.1, 0.2] }, 5000))).toEqual({
      kind: 'diagnostics',
      tMs: 5000,
      frameCostMs: 21,
      droppedFrames: 2,
    });
    expect(parse(referenceLine(12_345, 72))).toEqual({ kind: 'reference', tMs: 12_345, hrBpm: 72, source: 'watch' });
  });

  it('reads a heart rate typed from the watch only within 30 to 220 bpm', () => {
    expect(parseHeartRate(' 72 ')).toBe(72);
    expect(parseHeartRate('72,5')).toBe(72.5);
    expect(parseHeartRate('29')).toBeNull();
    expect(parseHeartRate('221')).toBeNull();
    expect(parseHeartRate('abc')).toBeNull();
    expect(parseHeartRate('')).toBeNull();
  });
});
