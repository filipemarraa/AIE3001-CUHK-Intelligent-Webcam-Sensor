export interface Point {
  x: number;
  y: number;
}

export interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
}

const FOREHEAD_LEFT = 109;
const FOREHEAD_RIGHT = 338;
const FOREHEAD_TOP = 10;
const FOREHEAD_BOTTOM = 151;
const CHEEK_LEFT = 50;
const CHEEK_RIGHT = 280;
const FACE_EDGE_LEFT = 234;
const FACE_EDGE_RIGHT = 454;
const REQUIRED_LANDMARKS = 455;
const CHEEK_SIDE_PER_FACE_WIDTH = 0.12;

function clip(x0: number, y0: number, x1: number, y1: number, width: number, height: number): Box | null {
  const left = Math.max(0, Math.round(Math.min(x0, x1)));
  const top = Math.max(0, Math.round(Math.min(y0, y1)));
  const right = Math.min(width, Math.round(Math.max(x0, x1)));
  const bottom = Math.min(height, Math.round(Math.max(y0, y1)));
  if (right <= left || bottom <= top) return null;
  return { x: left, y: top, w: right - left, h: bottom - top };
}

export function roiBoxes(faces: Point[][], width: number, height: number): Box[] {
  const face = faces[0];
  if (!face || face.length < REQUIRED_LANDMARKS) return [];
  const p = (i: number): Point => face[i] as Point;
  const faceWidth = Math.abs(p(FACE_EDGE_LEFT).x - p(FACE_EDGE_RIGHT).x) * width;
  const half = (CHEEK_SIDE_PER_FACE_WIDTH * faceWidth) / 2;
  const boxes = [
    clip(
      p(FOREHEAD_LEFT).x * width,
      p(FOREHEAD_TOP).y * height,
      p(FOREHEAD_RIGHT).x * width,
      p(FOREHEAD_BOTTOM).y * height,
      width,
      height,
    ),
  ];
  for (const i of [CHEEK_LEFT, CHEEK_RIGHT]) {
    const cx = p(i).x * width;
    const cy = p(i).y * height;
    boxes.push(clip(cx - half, cy - half, cx + half, cy + half, width, height));
  }
  return boxes.filter((b): b is Box => b !== null);
}

export function meanRgb(
  pixels: Uint8ClampedArray,
  width: number,
  boxes: Box[],
): { r: number; g: number; b: number; luma: number } | null {
  const rows = Math.floor(pixels.length / 4 / width);
  let r = 0;
  let g = 0;
  let b = 0;
  let n = 0;
  for (const box of boxes) {
    const x1 = Math.min(width, box.x + box.w);
    const y1 = Math.min(rows, box.y + box.h);
    for (let y = Math.max(0, box.y); y < y1; y++) {
      for (let x = Math.max(0, box.x); x < x1; x++) {
        const o = (y * width + x) * 4;
        r += pixels[o] as number;
        g += pixels[o + 1] as number;
        b += pixels[o + 2] as number;
        n++;
      }
    }
  }
  if (n === 0) return null;
  r /= n;
  g /= n;
  b /= n;
  return { r, g, b, luma: 0.299 * r + 0.587 * g + 0.114 * b };
}
