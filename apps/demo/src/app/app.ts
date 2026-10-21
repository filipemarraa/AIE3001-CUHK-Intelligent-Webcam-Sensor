import { Component, inject } from '@angular/core';
import { Calibration } from './screens/calibration/calibration';
import { Exercise } from './screens/exercise/exercise';
import { Summary } from './screens/summary/summary';
import { Welcome } from './screens/welcome/welcome';
import { Shell } from './shell/shell';
import { TransparencyPanel } from './shell/transparency-panel';
import { VitalsService } from './vitals.service';

@Component({
  selector: 'app-root',
  imports: [Shell, Welcome, Calibration, Exercise, Summary, TransparencyPanel],
  templateUrl: './app.html',
})
export class App {
  protected readonly vitals = inject(VitalsService);
}
