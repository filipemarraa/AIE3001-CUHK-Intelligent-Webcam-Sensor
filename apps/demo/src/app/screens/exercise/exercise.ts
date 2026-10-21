import { Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { breathingRateText, heartRateText } from '../../format';
import { instructionText } from '../../reason-text';
import { TARGET_RPM, VitalsService } from '../../vitals.service';

const INHALE_SHARE = 0.4;
const TICK_MS = 250;

export function guideLabel(elapsedS: number, periodS: number): 'Breathe in' | 'Breathe out' {
  return elapsedS % periodS < periodS * INHALE_SHARE ? 'Breathe in' : 'Breathe out';
}

@Component({
  selector: 'app-exercise',
  templateUrl: './exercise.html',
})
export class Exercise {
  protected readonly vitals = inject(VitalsService);
  protected readonly periodS = 60 / TARGET_RPM;
  private readonly elapsedS = signal(0);
  protected readonly label = computed(() => guideLabel(this.elapsedS(), this.periodS));
  protected readonly estimate = this.vitals.estimate;
  protected readonly invalid = computed(() => this.estimate()?.confidence === 'invalid');
  protected readonly reason = computed(() => instructionText(this.estimate()));

  constructor() {
    const startedAt = Date.now();
    const timer = setInterval(() => {
      this.elapsedS.set((Date.now() - startedAt) / 1000);
    }, TICK_MS);
    inject(DestroyRef).onDestroy(() => {
      clearInterval(timer);
    });
  }

  protected readonly heartRate = computed(() => heartRateText(this.estimate()?.hrBpm));
  protected readonly breathingRate = computed(() => breathingRateText(this.estimate()?.rrRpm));

  protected toggleNumbers(): void {
    this.vitals.showNumbers.update((v) => !v);
  }
}
