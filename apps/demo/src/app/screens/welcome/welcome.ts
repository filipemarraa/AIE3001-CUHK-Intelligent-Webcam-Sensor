import { Component, computed, inject, input } from '@angular/core';
import type { VitalsErrorCode } from 'vitals-web';
import { VitalsService } from '../../vitals.service';

const ERROR_TEXT: Record<VitalsErrorCode, string> = {
  camera_denied: 'The camera permission was refused. Allow the camera for this page to try again.',
  camera_unavailable: 'The camera is not available or was disconnected.',
  wasm_failed: 'The measuring engine failed to load. Reload the page to try again.',
  vision_failed: 'The face tracker failed to load. Reload the page to try again.',
  unsupported_browser: 'This browser is not supported. Use a recent Chrome on a laptop.',
};

@Component({
  selector: 'app-welcome',
  templateUrl: './welcome.html',
})
export class Welcome {
  protected readonly vitals = inject(VitalsService);
  readonly video = input.required<HTMLVideoElement>();
  protected readonly errorText = computed(() => {
    const error = this.vitals.error();
    return error === null ? null : ERROR_TEXT[error.code];
  });

  protected turnOnCamera(): void {
    void this.vitals.consentAndStart(this.video());
  }
}
