import { Component, inject } from '@angular/core';
import { VitalsService } from '../vitals.service';

@Component({
  selector: 'app-shell',
  templateUrl: './shell.html',
})
export class Shell {
  protected readonly vitals = inject(VitalsService);
}
