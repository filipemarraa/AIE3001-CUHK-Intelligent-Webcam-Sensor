import { describe, expect, it } from 'vitest';
import { motion, mouthOpening, shoulders } from './metrics.js';
import type { Point } from './roi.js';

function face(over: Record<number, Point> = {}): Point[] {
  const f: Point[] = Array.from({ length: 478 }, (_, i) => ({ x: (i % 10) / 10, y: Math.floor(i / 10) / 100 }));
  f[33] = { x: 0.4, y: 0.4 };
  f[263] = { x: 0.6, y: 0.4 };
  f[13] = { x: 0.5, y: 0.7 };
  f[14] = { x: 0.5, y: 0.7 };
  f[10] = { x: 0.5, y: 0.2 };
  f[152] = { x: 0.5, y: 0.9 };
  for (const [i, p] of Object.entries(over)) f[Number(i)] = p;
  return f;
}

describe('motion', () => {
  it('is zero for identical landmarks', () => {
    expect(motion(face(), face())).toBe(0);
  });

  it('is one when every point moves one inter-ocular distance', () => {
    const prev = face();
    const curr = prev.map((p) => ({ x: p.x + 0.2, y: p.y }));
    expect(motion(prev, curr)).toBeCloseTo(1, 9);
  });

  it('is zero without a previous frame', () => {
    expect(motion(null, face())).toBe(0);
  });

  it('is zero when landmarks are missing', () => {
    expect(motion([{ x: 0, y: 0 }], [{ x: 1, y: 1 }])).toBe(0);
  });

  it('is zero when the inter-ocular distance is zero', () => {
    const f = face({ 33: { x: 0.5, y: 0.4 }, 263: { x: 0.5, y: 0.4 } });
    expect(
      motion(
        f,
        f.map((p) => ({ x: p.x + 0.1, y: p.y })),
      ),
    ).toBe(0);
  });
});

describe('mouthOpening', () => {
  it('is zero when the mouth is closed', () => {
    expect(mouthOpening(face())).toBe(0);
  });

  it('is lip gap over face height', () => {
    expect(mouthOpening(face({ 14: { x: 0.5, y: 0.77 } }))).toBeCloseTo(0.07 / 0.7, 9);
  });

  it('is zero when landmarks are missing', () => {
    expect(mouthOpening([{ x: 0, y: 0 }])).toBe(0);
  });

  it('is zero when the face height is zero', () => {
    expect(mouthOpening(face({ 152: { x: 0.5, y: 0.2 } }))).toBe(0);
  });
});

describe('shoulders', () => {
  it('is null without a pose', () => {
    expect(shoulders(null)).toBeNull();
  });

  it('is null when landmarks are missing', () => {
    expect(shoulders([{ x: 0, y: 0 }])).toBeNull();
  });

  it('returns mean y and horizontal width', () => {
    const pose: Point[] = Array.from({ length: 33 }, () => ({ x: 0, y: 0 }));
    pose[11] = { x: 0.7, y: 0.5 };
    pose[12] = { x: 0.3, y: 0.7 };
    const s = shoulders(pose);
    expect(s?.y).toBeCloseTo(0.6, 12);
    expect(s?.w).toBeCloseTo(0.4, 12);
  });
});
