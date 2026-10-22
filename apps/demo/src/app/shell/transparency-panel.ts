import { Component, computed, inject, signal } from '@angular/core';
import { VitalsService } from '../vitals.service';

const WIDTH = 300;
const HEIGHT = 60;

export function pulsePoints(pulse: readonly number[]): string {
  if (pulse.length === 0) return '';
  const min = Math.min(...pulse);
  const range = Math.max(...pulse) - min;
  const round = (v: number): number => Math.round(v * 100) / 100;
  return pulse
    .map((v, i) => {
      const x = pulse.length === 1 ? 0 : (i / (pulse.length - 1)) * WIDTH;
      const y = range === 0 ? HEIGHT / 2 : HEIGHT - ((v - min) / range) * HEIGHT;
      return `${String(round(x))},${String(round(y))}`;
    })
    .join(' ');
}

@Component({
  selector: 'app-transparency-panel',
  templateUrl: './transparency-panel.html',
})
export class TransparencyPanel {
  protected readonly vitals = inject(VitalsService);
  protected readonly open = signal(false);
  protected readonly viewBox = `0 0 ${String(WIDTH)} ${String(HEIGHT)}`;
  protected readonly qualityPercent = computed(() => {
    const q = this.vitals.estimate()?.quality ?? 0;
    return Math.round(Math.min(1, Math.max(0, q)) * 100);
  });
  protected readonly points = computed(() => pulsePoints(this.vitals.diagnostics()?.pulse ?? []));

  protected toggle(): void {
    this.open.update((v) => !v);
  }
}
