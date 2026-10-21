import { Component, computed, inject } from '@angular/core';
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

  protected value(n: number | null | undefined): string {
    return n === null || n === undefined ? '—' : String(n);
  }
}
