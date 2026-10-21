import { describe, expect, it } from 'vitest';
import { meanRgb, roiBoxes, type Point } from './roi.js';

function makeFace(over: Record<number, Point> = {}): Point[] {
  const face: Point[] = Array.from({ length: 478 }, () => ({ x: 0.5, y: 0.5 }));
  const base: Record<number, Point> = {
    109: { x: 0.4, y: 0.2 },
    338: { x: 0.6, y: 0.2 },
    10: { x: 0.5, y: 0.2 },
    151: { x: 0.5, y: 0.4 },
    50: { x: 0.3, y: 0.6 },
    280: { x: 0.7, y: 0.6 },
    234: { x: 0.2, y: 0.5 },
    454: { x: 0.8, y: 0.5 },
  };
  for (const [i, p] of Object.entries({ ...base, ...over })) face[Number(i)] = p;
  return face;
}

describe('roiBoxes', () => {
  it('returns forehead and two cheek boxes in pixels', () => {
    expect(roiBoxes([makeFace()], 1000, 500)).toEqual([
      { x: 400, y: 100, w: 200, h: 100 },
      { x: 264, y: 264, w: 72, h: 72 },
      { x: 664, y: 264, w: 72, h: 72 },
    ]);
  });

  it('uses only the first face', () => {
    const faceA = makeFace();
    const faceB = makeFace({ 50: { x: 0.9, y: 0.9 }, 10: { x: 0.1, y: 0.1 } });
    expect(roiBoxes([faceA, faceB], 1000, 500)).toEqual(roiBoxes([faceA], 1000, 500));
  });

  it('returns no boxes without a face', () => {
    expect(roiBoxes([], 1000, 500)).toEqual([]);
  });

  it('returns no boxes when landmarks are missing', () => {
    expect(roiBoxes([[{ x: 0.5, y: 0.5 }]], 1000, 500)).toEqual([]);
  });

  it('clips boxes to the frame', () => {
    const boxes = roiBoxes([makeFace({ 50: { x: 0.02, y: 0.98 } })], 1000, 500);
    expect(boxes.length).toBeGreaterThan(0);
    for (const b of boxes) {
      expect(b.w).toBeGreaterThan(0);
      expect(b.h).toBeGreaterThan(0);
      expect(b.x).toBeGreaterThanOrEqual(0);
      expect(b.y).toBeGreaterThanOrEqual(0);
      expect(b.x + b.w).toBeLessThanOrEqual(1000);
      expect(b.y + b.h).toBeLessThanOrEqual(500);
    }
    expect(boxes[1]).toEqual({ x: 0, y: 454, w: 56, h: 46 });
  });
});

describe('meanRgb', () => {
  function image(): Uint8ClampedArray {
    const px = new Uint8ClampedArray(4 * 4 * 4);
    for (let i = 0; i < 16; i++) px[i * 4 + 3] = 255;
    for (const [x, y] of [
      [1, 1],
      [2, 1],
      [1, 2],
      [2, 2],
    ] as const) {
      px[(y * 4 + x) * 4] = 255;
    }
    return px;
  }

  it('averages the pixels inside the box', () => {
    const m = meanRgb(image(), 4, [{ x: 1, y: 1, w: 2, h: 2 }]);
    expect(m).not.toBeNull();
    expect(m?.r).toBe(255);
    expect(m?.g).toBe(0);
    expect(m?.b).toBe(0);
    expect(m?.luma).toBeCloseTo(76.245, 3);
  });

  it('weights by pixel count across boxes', () => {
    const m = meanRgb(image(), 4, [
      { x: 1, y: 1, w: 2, h: 2 },
      { x: 0, y: 0, w: 4, h: 1 },
    ]);
    expect(m?.r).toBeCloseTo((255 * 4) / 8, 6);
  });

  it('returns null without pixels', () => {
    expect(meanRgb(image(), 4, [])).toBeNull();
    expect(meanRgb(image(), 4, [{ x: 0, y: 0, w: 0, h: 3 }])).toBeNull();
  });
});

describe('meanRgb with robust pixels', () => {
  function row(colours: (readonly [number, number, number])[]): Uint8ClampedArray {
    const px = new Uint8ClampedArray(colours.length * 4);
    colours.forEach(([r, g, b], i) => {
      px.set([r, g, b, 255], i * 4);
    });
    return px;
  }
  const skin = [180, 140, 120] as const;

  it('leaves out saturated pixels and pixels far from the median brightness of the region', () => {
    const px = row([skin, skin, skin, skin, skin, skin, [30, 25, 20], [35, 28, 22], [255, 250, 245], [255, 200, 180]]);
    const box = [{ x: 0, y: 0, w: 10, h: 1 }];
    expect(meanRgb(px, 10, box)?.r).not.toBeCloseTo(180, 0);
    expect(meanRgb(px, 10, box, { robust: true })).toMatchObject({ r: 180, g: 140, b: 120 });
  });

  it('keeps every pixel of dark skin with a normal spread', () => {
    const px = row([
      [62, 41, 31],
      [58, 39, 29],
      [60, 40, 30],
      [64, 42, 32],
      [56, 38, 28],
    ]);
    const box = [{ x: 0, y: 0, w: 5, h: 1 }];
    expect(meanRgb(px, 5, box, { robust: true })).toEqual(meanRgb(px, 5, box));
  });

  it('falls back to every pixel when the filter would leave none', () => {
    const px = row([
      [255, 255, 255],
      [255, 255, 255],
    ]);
    const box = [{ x: 0, y: 0, w: 2, h: 1 }];
    expect(meanRgb(px, 2, box, { robust: true })).toEqual(meanRgb(px, 2, box));
  });
});
