// Flat float64 layouts shared with engine/session/sample.go and engine/session/encode.go.
export const SAMPLE = {
  timeMs: 0,
  r: 1,
  g: 2,
  b: 3,
  face: 4,
  motion: 5,
  light: 6,
  mouth: 7,
  shoulderY: 8,
  shoulderW: 9,
  length: 10,
} as const;

export const OUT = {
  timeMs: 0,
  hr: 1,
  rr: 2,
  phase: 3,
  quality: 4,
  confidence: 5,
  reason: 6,
  pulseLen: 7,
  pulse: 8,
  maxLength: 158,
} as const;
