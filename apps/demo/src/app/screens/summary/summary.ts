import { Component, computed, inject } from '@angular/core';
import { breathingRateText, heartRateText } from '../../format';
import { VitalsService } from '../../vitals.service';

@Component({
  selector: 'app-summary',
  templateUrl: './summary.html',
})
export class Summary {
  protected readonly vitals = inject(VitalsService);
  protected readonly quality = computed(() => {
    const q = this.vitals.summary()?.qualityMean;
    return q === null || q === undefined ? '—' : `${String(Math.round(q * 100))}%`;
  });

  protected readonly heartRateStart = computed(() => heartRateText(this.vitals.summary()?.hrStart));
  protected readonly heartRateEnd = computed(() => heartRateText(this.vitals.summary()?.hrEnd));
  protected readonly breathingMean = computed(() => breathingRateText(this.vitals.summary()?.rrMean));
}
