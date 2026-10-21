import { Component, inject } from '@angular/core';
import { Welcome } from './screens/welcome/welcome';
import { Shell } from './shell/shell';
import { VitalsService } from './vitals.service';

@Component({
  selector: 'app-root',
  imports: [Shell, Welcome],
  templateUrl: './app.html',
})
export class App {
  protected readonly vitals = inject(VitalsService);
}
