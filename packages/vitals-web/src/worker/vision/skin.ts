import type { SkinRegion } from '../../types.js';
import type { Box, Point } from './roi.js';

// A landmark index, or the point at fraction t of the way from landmark a to landmark b.
type Vertex = number | readonly [a: number, b: number, t: number];

// Checked on the Face Landmarker output for a frontal face: every region lies on skin. The forehead stops half way to
// the top landmarks and above the brows, so a fringe or the brows stay out; the lower face moves when talking.
const REGIONS: Readonly<Record<SkinRegion, readonly Vertex[]>> = {
  forehead: [
    [104, 103, 0.5],
    [69, 67, 0.5],
    [108, 109, 0.5],
    [151, 10, 0.5],
    [337, 338, 0.5],
    [299, 297, 0.5],
    [333, 332, 0.5],
    333,
    [334, 333, 0.3],
    [296, 299, 0.3],
    [336, 337, 0.3],
    9,
    [107, 108, 0.3],
    [66, 69, 0.3],
    [105, 104, 0.3],
    104,
  ],
  glabella: [[107, 9, 0.25], 9, [336, 9, 0.25], [285, 8, 0.25], 8, [55, 8, 0.25]],
  malarRight: [116, 117, 118, 119, 100, 126, 142, 203, 205, 187, 147, 123],
  malarLeft: [345, 346, 347, 348, 329, 355, 371, 423, 425, 411, 376, 352],
};

// The order of the regions in a mask label and in the overlay message.
export const SKIN_REGIONS = Object.keys(REGIONS) as SkinRegion[];
const FACE_EDGE_RIGHT = 234;
const FACE_EDGE_LEFT = 454;
const REQUIRED_LANDMARKS = 468;
// Past this head turn the cheek on the far side faces away from the camera and mostly adds noise.
const MAX_CHEEK_YAW_DEG = 35;
const MIN_REGION_PIXELS = 20;

export interface Polygon {
  region: SkinRegion;
  points: Point[];
}

export interface SkinMask {
  rect: Box;
  // One byte per pixel of rect, row by row: 0 outside the skin, otherwise 1 + the index of the region.
  labels: Uint8Array;
  pixels: number;
}

// Positive when the side of the face on the right of the image turns away from the camera. Landmark z has about the
// scale of x, so the depth step between the face edges is compared with their distance in the image, in widths.
export function yawDegrees(face: Point[], width: number, height: number): number {
  const r = face[FACE_EDGE_RIGHT];
  const l = face[FACE_EDGE_LEFT];
  if (!r || !l || r.z === undefined || l.z === undefined) return 0;
  const across = Math.hypot(l.x - r.x, ((l.y - r.y) * height) / width);
  return (Math.atan2(l.z - r.z, across) * 180) / Math.PI;
}

export function skinPolygons(face: Point[], width: number, height: number): Polygon[] {
  if (face.length < REQUIRED_LANDMARKS) return [];
  const at = (v: Vertex): Point => {
    if (typeof v === 'number') return { x: (face[v] as Point).x * width, y: (face[v] as Point).y * height };
    const a = face[v[0]] as Point;
    const b = face[v[1]] as Point;
    return { x: (a.x + (b.x - a.x) * v[2]) * width, y: (a.y + (b.y - a.y) * v[2]) * height };
  };
  const yaw = yawDegrees(face, width, height);
  return SKIN_REGIONS.filter(
    (region) =>
      !(region === 'malarLeft' && yaw > MAX_CHEEK_YAW_DEG) && !(region === 'malarRight' && yaw < -MAX_CHEEK_YAW_DEG),
  ).map((region) => ({ region, points: REGIONS[region].map(at) }));
}

function bounds(polygons: Polygon[], width: number, height: number): Box | null {
  const points = polygons.flatMap((p) => p.points);
  if (points.length === 0) return null;
  const x0 = Math.max(0, Math.floor(Math.min(...points.map((p) => p.x))));
  const y0 = Math.max(0, Math.floor(Math.min(...points.map((p) => p.y))));
  const x1 = Math.min(width, Math.ceil(Math.max(...points.map((p) => p.x))));
  const y1 = Math.min(height, Math.ceil(Math.max(...points.map((p) => p.y))));
  if (x1 <= x0 || y1 <= y0) return null;
  return { x: x0, y: y0, w: x1 - x0, h: y1 - y0 };
}

// Even-odd scanline fill at pixel centres.
function fill(labels: Uint8Array, rect: Box, points: Point[], label: number): number {
  let count = 0;
  const xs: number[] = [];
  for (let row = 0; row < rect.h; row++) {
    const yc = rect.y + row + 0.5;
    xs.length = 0;
    for (let i = 0, j = points.length - 1; i < points.length; j = i++) {
      const a = points[i] as Point;
      const b = points[j] as Point;
      if (a.y <= yc !== b.y <= yc) xs.push(a.x + ((yc - a.y) * (b.x - a.x)) / (b.y - a.y));
    }
    xs.sort((p, q) => p - q);
    for (let k = 0; k + 1 < xs.length; k += 2) {
      const from = Math.max(0, Math.ceil((xs[k] as number) - 0.5) - rect.x);
      const to = Math.min(rect.w, Math.ceil((xs[k + 1] as number) - 0.5) - rect.x);
      for (let col = from; col < to; col++) {
        const o = row * rect.w + col;
        if (labels[o] === 0) count++;
        labels[o] = label;
      }
    }
  }
  return count;
}

export function rasterize(polygons: Polygon[], width: number, height: number): SkinMask | null {
  const rect = bounds(polygons, width, height);
  if (rect === null) return null;
  const labels = new Uint8Array(rect.w * rect.h);
  let pixels = 0;
  for (const polygon of polygons) {
    const label = SKIN_REGIONS.indexOf(polygon.region) + 1;
    const scratch = new Uint8Array(rect.w * rect.h);
    if (fill(scratch, rect, polygon.points, label) < MIN_REGION_PIXELS) continue;
    for (let o = 0; o < scratch.length; o++) {
      if (scratch[o] !== 0 && labels[o] === 0) {
        labels[o] = label;
        pixels++;
      }
    }
  }
  return pixels === 0 ? null : { rect, labels, pixels };
}

export interface PixelOptions {
  robust?: boolean;
}

const SATURATED = 250;
// Relative to the median brightness of each region, so dark skin keeps its pixels and brows and glare do not.
const BRIGHTNESS_BAND = 0.4;

const luma = (r: number, g: number, b: number): number => 0.299 * r + 0.587 * g + 0.114 * b;

function medianLumas(pixels: Uint8ClampedArray, labels: Uint8Array): number[] {
  const histograms = SKIN_REGIONS.map(() => new Uint32Array(256));
  const counts = SKIN_REGIONS.map(() => 0);
  for (let i = 0; i < labels.length; i++) {
    const label = labels[i] as number;
    if (label === 0) continue;
    const o = i * 4;
    const l = Math.min(255, Math.round(luma(pixels[o] as number, pixels[o + 1] as number, pixels[o + 2] as number)));
    const h = histograms[label - 1] as Uint32Array;
    h[l] = (h[l] as number) + 1;
    counts[label - 1] = (counts[label - 1] as number) + 1;
  }
  return histograms.map((h, k) => {
    let seen = 0;
    for (let l = 0; l < 256; l++) {
      seen += h[l] as number;
      if (seen * 2 >= (counts[k] as number) && seen > 0) return l;
    }
    return 0;
  });
}

// pixels holds the RGBA bytes of mask.rect, row by row.
export function maskedMeanRgb(
  pixels: Uint8ClampedArray,
  mask: SkinMask,
  options: PixelOptions = {},
): { r: number; g: number; b: number; luma: number } | null {
  const robust = options.robust === true;
  const medians = robust ? medianLumas(pixels, mask.labels) : [];
  let r = 0;
  let g = 0;
  let b = 0;
  let n = 0;
  for (let i = 0; i < mask.labels.length; i++) {
    const label = mask.labels[i] as number;
    if (label === 0) continue;
    const o = i * 4;
    const pr = pixels[o] as number;
    const pg = pixels[o + 1] as number;
    const pb = pixels[o + 2] as number;
    if (robust) {
      const median = medians[label - 1] as number;
      const l = luma(pr, pg, pb);
      if (
        Math.max(pr, pg, pb) >= SATURATED ||
        l < (1 - BRIGHTNESS_BAND) * median ||
        l > (1 + BRIGHTNESS_BAND) * median
      ) {
        continue;
      }
    }
    r += pr;
    g += pg;
    b += pb;
    n++;
  }
  if (n === 0) return robust ? maskedMeanRgb(pixels, mask) : null;
  r /= n;
  g /= n;
  b /= n;
  return { r, g, b, luma: luma(r, g, b) };
}
