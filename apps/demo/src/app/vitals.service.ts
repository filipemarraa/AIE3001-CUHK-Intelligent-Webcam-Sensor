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

export const EXERCISE_S = 120;
export const TARGET_RPM = 6;
export const NO_FACE_TIMEOUT_S = 60;
export const CALIBRATION_MAX_S = 120;
const NO_FACE_ESTIMATES = 60;

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
  private noFaceRun = 0;
  private exerciseTimer: ReturnType<typeof setTimeout> | null = null;
  private summaryTimer: ReturnType<typeof setTimeout> | null = null;
  private calibrationTimer: ReturnType<typeof setTimeout> | null = null;

  private readonly screenState = signal<Screen>('welcome');
  private readonly estimateState = signal<Estimate | null>(null);
  private readonly diagnosticsState = signal<Diagnostics | null>(null);
  private readonly summaryState = signal<ExerciseSummary | null>(null);
  private readonly cameraOnState = signal(false);
  private readonly errorState = signal<VitalsError | null>(null);

  readonly showNumbers = signal(false);
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
      this.clearTimers();
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
      // The module usually reports the failure through an error event that already reset the screen; when it does
      // not, this session is still current and the camera state is cleared here.
      if (session === this.session) this.cameraOnState.set(false);
      return;
    }
    if (session !== this.session) return;
    this.screenState.set('calibration');
    this.calibrationTimer = setTimeout(() => {
      this.exit();
    }, CALIBRATION_MAX_S * 1000);
  }

  finishExercise(): void {
    this.session += 1;
    this.clearTimers();
    const summary = this.vitals.summary();
    this.vitals.stop();
    this.cameraOnState.set(false);
    this.summaryState.set(summary);
    this.screenState.set('summary');
    this.summaryTimer = setTimeout(() => {
      this.reset();
    }, NO_FACE_TIMEOUT_S * 1000);
  }

  startAgain(): void {
    this.reset();
    this.errorState.set(null);
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
    this.noFaceRun = e.reason === 'no_face' ? this.noFaceRun + 1 : 0;
    if (this.noFaceRun >= NO_FACE_ESTIMATES) {
      this.exit();
      return;
    }
    // Breathing has its own test in the engine, and a weak pulse will not get better by waiting in calibration.
    const readable = e.confidence !== 'invalid' || e.rrRpm !== null || e.reason === 'weak_pulse';
    if (this.screenState() === 'calibration' && readable) this.enterExercise();
  }

  private enterExercise(): void {
    this.clearCalibrationTimer();
    this.screenState.set('exercise');
    this.exerciseTimer = setTimeout(() => {
      this.finishExercise();
    }, EXERCISE_S * 1000);
  }

  private clearCalibrationTimer(): void {
    if (this.calibrationTimer !== null) clearTimeout(this.calibrationTimer);
    this.calibrationTimer = null;
  }

  private clearTimers(): void {
    this.clearCalibrationTimer();
    if (this.exerciseTimer !== null) clearTimeout(this.exerciseTimer);
    if (this.summaryTimer !== null) clearTimeout(this.summaryTimer);
    this.exerciseTimer = null;
    this.summaryTimer = null;
  }

  private reset(): void {
    this.session += 1;
    this.clearTimers();
    this.noFaceRun = 0;
    this.showNumbers.set(false);
    this.screenState.set('welcome');
    this.cameraOnState.set(false);
    this.estimateState.set(null);
    this.diagnosticsState.set(null);
    this.summaryState.set(null);
  }
}
