export function heartRateText(bpm: number | null | undefined): string {
  return bpm === null || bpm === undefined ? '—' : String(Math.round(bpm));
}

export function breathingRateText(rpm: number | null | undefined): string {
  return rpm === null || rpm === undefined ? '—' : rpm.toFixed(1);
}
