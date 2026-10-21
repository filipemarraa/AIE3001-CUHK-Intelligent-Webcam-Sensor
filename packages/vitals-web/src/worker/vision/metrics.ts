import type { Point } from './roi.js';

const EYE_OUTER_LEFT = 33;
const EYE_OUTER_RIGHT = 263;
const LIP_UPPER = 13;
const LIP_LOWER = 14;
const FOREHEAD_TOP = 10;
const CHIN = 152;
const SHOULDER_LEFT = 11;
const SHOULDER_RIGHT = 12;

const dist = (a: Point, b: Point): number => Math.hypot(a.x - b.x, a.y - b.y);

export function motion(prev: Point[] | null, curr: Point[]): number {
  if (!prev || prev.length === 0 || prev.length !== curr.length) return 0;
  const l = curr[EYE_OUTER_LEFT];
  const r = curr[EYE_OUTER_RIGHT];
  if (!l || !r) return 0;
  const iod = dist(l, r);
  if (iod === 0) return 0;
  let sum = 0;
  for (let i = 0; i < curr.length; i++) sum += dist(prev[i] as Point, curr[i] as Point);
  return sum / curr.length / iod;
}

export function mouthOpening(face: Point[]): number {
  const up = face[LIP_UPPER];
  const low = face[LIP_LOWER];
  const top = face[FOREHEAD_TOP];
  const chin = face[CHIN];
  if (!up || !low || !top || !chin) return 0;
  const height = dist(top, chin);
  return height === 0 ? 0 : dist(up, low) / height;
}

export function shoulders(pose: Point[] | null): { y: number; w: number } | null {
  const l = pose?.[SHOULDER_LEFT];
  const r = pose?.[SHOULDER_RIGHT];
  if (!l || !r) return null;
  return { y: (l.y + r.y) / 2, w: Math.abs(l.x - r.x) };
}
