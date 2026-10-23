import { describe, expect, it } from 'vitest';
import { faceLandmarks } from '../../test/face';
import { roiBoxes } from './roi';
import { maskedMeanRgb, rasterize, skinPolygons, yawDegrees, type SkinMask } from './skin';

const W = 640;
const H = 480;

function mask(pose: Parameters<typeof faceLandmarks>[0]): SkinMask | null {
  return rasterize(skinPolygons(faceLandmarks(pose), W, H), W, H);
}

function regionsIn(m: SkinMask | null): string[] {
  const names = ['forehead', 'glabella', 'malarRight', 'malarLeft'];
  return [...new Set(m?.labels ?? [])]
    .filter((l) => l > 0)
    .map((l) => names[l - 1] as string)
    .sort();
}

function frameOf(m: SkinMask, inside: [number, number, number], outside: [number, number, number]): Uint8ClampedArray {
  const px = new Uint8ClampedArray(m.rect.w * m.rect.h * 4);
  m.labels.forEach((label, i) => {
    px.set([...(label === 0 ? outside : inside), 255], i * 4);
  });
  return px;
}

describe('skin regions', () => {
  it('cover at least 2.5 times the pixels of the three boxes on a frontal face', () => {
    const face = faceLandmarks({ widthPx: 150 });
    const boxes = roiBoxes([face], W, H).reduce((sum, b) => sum + b.w * b.h, 0);
    expect(mask({ widthPx: 150 })?.pixels ?? 0).toBeGreaterThan(2.5 * boxes);
  });

  it('turn with the head: a 30° tilt keeps the area within 10%', () => {
    const upright = mask({ widthPx: 150 })?.pixels ?? 0;
    for (const rollDeg of [-30, 30]) {
      expect(Math.abs((mask({ widthPx: 150, rollDeg })?.pixels ?? 0) / upright - 1)).toBeLessThan(0.1);
    }
  });

  it('estimate the head turn from the depth of the face edges, also when tilted', () => {
    for (const yawDeg of [-40, -20, 0, 20, 40]) {
      expect(yawDegrees(faceLandmarks({ yawDeg }), W, H)).toBeCloseTo(yawDeg, 0);
      expect(yawDegrees(faceLandmarks({ yawDeg, rollDeg: 25 }), W, H)).toBeCloseTo(yawDeg, 0);
    }
    expect(yawDegrees([{ x: 0, y: 0 }], W, H)).toBe(0);
  });

  it('leave out the cheek that turns away past 35°', () => {
    expect(regionsIn(mask({ yawDeg: 25 }))).toEqual(['forehead', 'glabella', 'malarLeft', 'malarRight']);
    expect(regionsIn(mask({ yawDeg: 45 }))).toEqual(['forehead', 'glabella', 'malarRight']);
    expect(regionsIn(mask({ yawDeg: -45 }))).toEqual(['forehead', 'glabella', 'malarLeft']);
  });

  it('leave out a region smaller than 20 pixels', () => {
    const small = mask({ widthPx: 40 });
    expect(regionsIn(small)).not.toContain('glabella');
    expect(small?.pixels ?? 0).toBeGreaterThan(0);
  });

  it('clip regions to the frame', () => {
    const m = mask({ widthPx: 150, centre: { x: 20, y: 240 } });
    expect(m).not.toBeNull();
    expect(m?.rect.x).toBe(0);
    expect((m?.rect.x ?? 0) + (m?.rect.w ?? 0)).toBeLessThanOrEqual(W);
    expect(m?.labels.length).toBe((m?.rect.w ?? 0) * (m?.rect.h ?? 0));
  });

  it('give nothing without enough landmarks or with the face outside the frame', () => {
    expect(skinPolygons(faceLandmarks().slice(0, 400), W, H)).toEqual([]);
    expect(mask({ centre: { x: -500, y: 240 } })).toBeNull();
  });
});

describe('maskedMeanRgb', () => {
  it('averages only the pixels inside the mask', () => {
    const m = mask({ widthPx: 150 }) as SkinMask;
    expect(maskedMeanRgb(frameOf(m, [200, 150, 120], [0, 0, 255]), m)).toEqual({
      r: 200,
      g: 150,
      b: 120,
      luma: 0.299 * 200 + 0.587 * 150 + 0.114 * 120,
    });
  });

  it('with robust pixels, leaves out glare and pixels far from the median brightness of their region', () => {
    const m = mask({ widthPx: 150 }) as SkinMask;
    const px = frameOf(m, [90, 60, 45], [0, 0, 0]);
    let spoiled = 0;
    m.labels.forEach((label, i) => {
      if (label !== 0 && spoiled < m.pixels / 10) {
        px.set(spoiled % 2 === 0 ? [255, 255, 255] : [10, 8, 6], i * 4);
        spoiled++;
      }
    });
    expect(maskedMeanRgb(px, m, { robust: true })).toMatchObject({ r: 90, g: 60, b: 45 });
    expect(maskedMeanRgb(px, m)?.r).not.toBe(90);
  });

  it('falls back to every masked pixel when the robust filter leaves none', () => {
    const m = mask({ widthPx: 150 }) as SkinMask;
    const px = frameOf(m, [255, 255, 255], [0, 0, 0]);
    expect(maskedMeanRgb(px, m, { robust: true })).toMatchObject({ r: 255, g: 255, b: 255 });
  });
});
