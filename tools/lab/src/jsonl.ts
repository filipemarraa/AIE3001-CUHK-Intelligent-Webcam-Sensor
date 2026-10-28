import type { CaptureOptions, Diagnostics, Estimate } from 'vitals-web';

// Shared with engine/eval/replay.go: change both and the version together.
const VERSION = 1;
const LAYOUT = ['timeMs', 'r', 'g', 'b', 'face', 'motion', 'light', 'mouth', 'shoulderY', 'shoulderW'];
const REGIONS = ['boxes', 'anatomical'];
const SKIN_VALUES = 3 * REGIONS.length;
const MIN_BPM = 30;
const MAX_BPM = 220;

export interface Header {
  subject: string;
  condition: string;
  options: CaptureOptions;
  camera: { label: string; width?: number; height?: number; frameRate?: number };
  startedAt: string;
  // Skin tone on the Monk scale (1 to 10), only when the participant volunteers it.
  monk?: number;
}

export function headerLine(h: Header): string {
  return JSON.stringify({
    kind: 'header',
    version: VERSION,
    subject: h.subject,
    condition: h.condition,
    regions: REGIONS,
    layout: LAYOUT,
    options: h.options,
    camera: h.camera,
    startedAt: h.startedAt,
    ...(h.monk === undefined ? {} : { monk: h.monk }),
  });
}

export function sampleLines(rows: number[][], skins?: number[][]): string[] {
  return rows.map((v, i) => {
    const s = skins?.[i] ?? new Array<number>(SKIN_VALUES).fill(0);
    if (v.length !== LAYOUT.length) throw new Error(`sample with ${String(v.length)} values, want 10 values`);
    if (s.length !== SKIN_VALUES) throw new Error(`sample with ${String(s.length)} skin colours, want 6 skin colours`);
    return JSON.stringify({ kind: 'sample', v, skins: s });
  });
}

export function estimateLine(e: Estimate): string {
  return JSON.stringify({ kind: 'estimate', e });
}

export function diagnosticsLine(d: Diagnostics, tMs: number): string {
  return JSON.stringify({ kind: 'diagnostics', tMs, frameCostMs: d.frameCostMs, droppedFrames: d.droppedFrames });
}

// A reference read from a Bluetooth heart rate sensor; without one, the reference was typed from a watch.
export interface BluetoothSource {
  source: 'ble';
  device: string;
  rrMs: number[];
}

export function referenceLine(tMs: number, hrBpm: number, ble?: BluetoothSource): string {
  if (ble === undefined) return JSON.stringify({ kind: 'reference', tMs, hrBpm, source: 'watch' });
  return JSON.stringify({
    kind: 'reference',
    tMs,
    hrBpm,
    source: 'ble',
    device: ble.device,
    ...(ble.rrMs.length === 0 ? {} : { rrMs: ble.rrMs }),
  });
}

export function parseHeartRate(text: string): number | null {
  const trimmed = text.trim().replace(',', '.');
  if (!/^\d+(\.\d+)?$/.test(trimmed)) return null;
  const bpm = Number(trimmed);
  return bpm >= MIN_BPM && bpm <= MAX_BPM ? bpm : null;
}
