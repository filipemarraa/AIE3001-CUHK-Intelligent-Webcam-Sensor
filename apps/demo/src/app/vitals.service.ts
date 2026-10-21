import { DestroyRef, Injectable, InjectionToken, inject, signal } from '@angular/core';
import {
  createMockVitals,
  createVitals,
  type Diagnostics,
  type Estimate,
  type ExerciseSummary,
  type Vitals,
  type VitalsError,
} from 'vitals-web';

export type Screen = 'welcome' | 'calibration' | 'exercise' | 'summary';

export const ASSETS_BASE_URL = '/assets/vitals';

// The worker is an entry of this app so that the Angular build bundles it and @mediapipe/tasks-vision with it.
export function createDefaultVitals(search: string): Vitals {
  if (new URLSearchParams(search).get('mock') === '1') return createMockVitals({ assetsBaseUrl: ASSETS_BASE_URL });
  return createVitals(
    { assetsBaseUrl: ASSETS_BASE_URL },
    { createWorker: () => new Worker(new URL('../sensor.worker', import.meta.url), { type: 'module' }) },
  );
}

export const VITALS_FACTORY = new InjectionToken<() => Vitals>('VITALS_FACTORY', {
  providedIn: 'root',
  factory: () => () => createDefaultVitals(location.search),
});

@Injectable({ providedIn: 'root' })
export class VitalsService {
  private readonly vitals = inject(VITALS_FACTORY)();
  // Bumped by exit() and by errors so that a start() still resolving cannot reopen a screen.
  private session = 0;

  private readonly screenState = signal<Screen>('welcome');
  private readonly estimateState = signal<Estimate | null>(null);
  private readonly diagnosticsState = signal<Diagnostics | null>(null);
  private readonly summaryState = signal<ExerciseSummary | null>(null);
  private readonly cameraOnState = signal(false);
  private readonly errorState = signal<VitalsError | null>(null);

  readonly screen = this.screenState.asReadonly();
  readonly estimate = this.estimateState.asReadonly();
  readonly diagnostics = this.diagnosticsState.asReadonly();
  readonly summary = this.summaryState.asReadonly();
  readonly cameraOn = this.cameraOnState.asReadonly();
  readonly error = this.errorState.asReadonly();

  constructor() {
    const unsubscribe = [
      this.vitals.on('estimate', (e) => {
        this.onEstimate(e);
      }),
      this.vitals.on('diagnostics', (d) => {
        if (this.measuring()) this.diagnosticsState.set(d);
      }),
      this.vitals.on('state', (s) => {
        if (s === 'stopped') this.cameraOnState.set(false);
      }),
      this.vitals.on('error', (e) => {
        this.reset();
        this.errorState.set(e);
      }),
    ];
    inject(DestroyRef).onDestroy(() => {
      for (const off of unsubscribe) off();
      this.vitals.stop();
    });
  }

  async consentAndStart(video: HTMLVideoElement): Promise<void> {
    const session = ++this.session;
    this.errorState.set(null);
    this.cameraOnState.set(true);
    try {
      await this.vitals.start({ video });
    } catch {
      // The module reports the failure through its error event, which already reset the screen.
      return;
    }
    if (session === this.session) this.screenState.set('calibration');
  }

  finishExercise(): void {
    this.session += 1;
    this.vitals.stop();
    this.cameraOnState.set(false);
    this.summaryState.set(this.vitals.summary());
    this.screenState.set('summary');
  }

  exit(): void {
    this.vitals.stop();
    this.reset();
    this.errorState.set(null);
  }

  private measuring(): boolean {
    const screen = this.screenState();
    return screen === 'calibration' || screen === 'exercise';
  }

  private onEstimate(e: Estimate): void {
    if (!this.measuring()) return;
    this.estimateState.set(e);
    if (this.screenState() === 'calibration' && e.confidence !== 'invalid') this.screenState.set('exercise');
  }

  private reset(): void {
    this.session += 1;
    this.screenState.set('welcome');
    this.cameraOnState.set(false);
    this.estimateState.set(null);
    this.diagnosticsState.set(null);
    this.summaryState.set(null);
  }
}
