import { Component, computed, inject } from '@angular/core';
import { instructionText } from '../../reason-text';
import { VitalsService } from '../../vitals.service';

@Component({
  selector: 'app-calibration',
  templateUrl: './calibration.html',
})
export class Calibration {
  private readonly vitals = inject(VitalsService);
  protected readonly text = computed(() => instructionText(this.vitals.estimate()));
}
