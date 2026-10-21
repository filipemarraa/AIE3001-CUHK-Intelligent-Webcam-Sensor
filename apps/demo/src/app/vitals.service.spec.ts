import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { FakeVitals, WARMING_UP, estimate } from './testing/fake-vitals';
import { CALIBRATION_MAX_S, EXERCISE_S, NO_FACE_TIMEOUT_S, VITALS_FACTORY, VitalsService } from './vitals.service';

describe('VitalsService', () => {
  let fake: FakeVitals;
  let service: VitalsService;
  const video = document.createElement('video');

  beforeEach(() => {
    fake = new FakeVitals();
    TestBed.configureTestingModule({ providers: [{ provide: VITALS_FACTORY, useValue: () => fake }] });
    service = TestBed.inject(VitalsService);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe('calibration limit', () => {
    it('exits like Exit after CALIBRATION_MAX_S without reaching the exercise', async () => {
      vi.useFakeTimers();
      await service.consentAndStart(video);
      fake.emit('estimate', WARMING_UP);
      fake.stop.mockClear();

      vi.advanceTimersByTime(CALIBRATION_MAX_S * 1000 - 1);
      expect(service.screen()).toBe('calibration');
      vi.advanceTimersByTime(1);

      expect(fake.stop).toHaveBeenCalledTimes(1);
      expect(service.screen()).toBe('welcome');
      expect(service.cameraOn()).toBe(false);
      expect(service.estimate()).toBeNull();
    });

    it('is cleared on entering the exercise', async () => {
      vi.useFakeTimers();
      await service.consentAndStart(video);
      vi.advanceTimersByTime(CALIBRATION_MAX_S * 1000 - 1);
      fake.emit('estimate', estimate());
      fake.stop.mockClear();

      vi.advanceTimersByTime(CALIBRATION_MAX_S * 1000 - 1);

      expect(service.screen()).toBe('exercise');
      expect(fake.stop).not.toHaveBeenCalled();
    });

    it('is cleared by Exit', async () => {
      vi.useFakeTimers();
      await service.consentAndStart(video);
      service.exit();
      fake.stop.mockClear();

      vi.advanceTimersByTime(CALIBRATION_MAX_S * 1000 * 2);

      expect(fake.stop).not.toHaveBeenCalled();
    });
  });

  it('does not start the camera before consent', () => {
    expect(fake.start).not.toHaveBeenCalled();
    expect(service.cameraOn()).toBe(false);
    expect(service.screen()).toBe('welcome');
  });

  it('starts once on consent and shows calibration', async () => {
    await service.consentAndStart(video);

    expect(fake.start).toHaveBeenCalledTimes(1);
    expect(fake.start).toHaveBeenCalledWith({ video });
    expect(service.cameraOn()).toBe(true);
    expect(service.screen()).toBe('calibration');
  });

  it('moves to the exercise on the first estimate that is not invalid', async () => {
    await service.consentAndStart(video);

    fake.emit('estimate', WARMING_UP);
    expect(service.screen()).toBe('calibration');
    expect(service.estimate()).toEqual(WARMING_UP);

    fake.emit('estimate', estimate());
    expect(service.screen()).toBe('exercise');
  });

  it('keeps diagnostics from the module', async () => {
    await service.consentAndStart(video);
    fake.emit('diagnostics', { frameCostMs: 12, droppedFrames: 1, pulse: [0.1] });

    expect(service.diagnostics()).toEqual({ frameCostMs: 12, droppedFrames: 1, pulse: [0.1] });
  });

  it('finishing the exercise stops the camera and keeps the summary', async () => {
    fake.summaryValue = { hrStart: 70, hrEnd: 64, rrMean: 6, qualityMean: 0.8, durationS: 120, method: 'pos' };
    await service.consentAndStart(video);
    fake.emit('estimate', estimate());

    service.finishExercise();

    expect(fake.stop).toHaveBeenCalledTimes(1);
    expect(service.cameraOn()).toBe(false);
    expect(service.screen()).toBe('summary');
    expect(service.summary()).toEqual(fake.summaryValue);
  });

  describe('exit', () => {
    const reachScreen: Record<string, () => Promise<void>> = {
      welcome: () => Promise.resolve(),
      calibration: () => service.consentAndStart(video),
      exercise: async () => {
        await service.consentAndStart(video);
        fake.emit('estimate', estimate());
        fake.emit('diagnostics', { frameCostMs: 10, droppedFrames: 0, pulse: [] });
      },
      summary: async () => {
        fake.summaryValue = { hrStart: 70, hrEnd: 64, rrMean: 6, qualityMean: 0.8, durationS: 60, method: 'pos' };
        await service.consentAndStart(video);
        fake.emit('estimate', estimate());
        service.finishExercise();
      },
    };

    for (const [screen, reach] of Object.entries(reachScreen)) {
      it(`from ${screen} stops the module and clears every reading`, async () => {
        await reach();
        expect(service.screen()).toBe(screen);
        fake.stop.mockClear();

        service.exit();

        expect(fake.stop).toHaveBeenCalledTimes(1);
        expect(service.screen()).toBe('welcome');
        expect(service.cameraOn()).toBe(false);
        expect(service.estimate()).toBeNull();
        expect(service.summary()).toBeNull();
        expect(service.diagnostics()).toBeNull();
        expect(service.error()).toBeNull();
      });
    }

    it('during a pending start keeps the welcome screen when start resolves', async () => {
      let resolveStart!: () => void;
      fake.start.mockImplementationOnce(
        () =>
          new Promise<void>((resolve) => {
            resolveStart = resolve;
          }),
      );
      const pending = service.consentAndStart(video);

      service.exit();
      resolveStart();
      await pending;

      expect(service.screen()).toBe('welcome');
      expect(service.cameraOn()).toBe(false);
    });

    it('ignores estimates that arrive after it', async () => {
      await service.consentAndStart(video);
      service.exit();

      fake.emit('estimate', estimate());

      expect(service.estimate()).toBeNull();
      expect(service.screen()).toBe('welcome');
    });
  });

  it('an error event returns to welcome with the error and the camera off', async () => {
    await service.consentAndStart(video);
    fake.emit('estimate', estimate());

    fake.emit('error', { code: 'camera_unavailable', message: 'camera disconnected' });
    fake.emit('state', 'stopped');

    expect(service.screen()).toBe('welcome');
    expect(service.error()).toEqual({ code: 'camera_unavailable', message: 'camera disconnected' });
    expect(service.cameraOn()).toBe(false);
    expect(service.estimate()).toBeNull();
  });

  it('a rejected start keeps the error and does not show calibration', async () => {
    fake.start.mockImplementationOnce(() => {
      fake.emit('error', { code: 'camera_denied' });
      fake.emit('state', 'stopped');
      return Promise.reject(new Error('camera_denied'));
    });

    await service.consentAndStart(video);

    expect(service.screen()).toBe('welcome');
    expect(service.error()).toEqual({ code: 'camera_denied' });
    expect(service.cameraOn()).toBe(false);
  });

  it('a new consent clears the previous error', async () => {
    fake.emit('error', { code: 'camera_denied' });

    await service.consentAndStart(video);

    expect(service.error()).toBeNull();
  });

  it('a start that rejects without an error event still turns the camera off', async () => {
    fake.start.mockImplementationOnce(() => Promise.reject(new Error('boom')));

    await service.consentAndStart(video);

    expect(service.screen()).toBe('welcome');
    expect(service.cameraOn()).toBe(false);
  });

  describe('exhibition mode', () => {
    const NO_FACE = estimate({
      hrBpm: null,
      rrRpm: null,
      breathPhase: null,
      confidence: 'invalid',
      reason: 'no_face',
    });

    function emitMany(count: number, e = NO_FACE): void {
      for (let i = 0; i < count; i += 1) fake.emit('estimate', e);
    }

    it('60 consecutive no_face estimates act like Exit', async () => {
      await service.consentAndStart(video);
      fake.stop.mockClear();

      emitMany(59);
      expect(service.screen()).toBe('calibration');
      emitMany(1);

      expect(fake.stop).toHaveBeenCalledTimes(1);
      expect(service.screen()).toBe('welcome');
      expect(service.cameraOn()).toBe(false);
      expect(service.estimate()).toBeNull();
    });

    it('any other estimate in between restarts the count', async () => {
      await service.consentAndStart(video);

      emitMany(59);
      fake.emit('estimate', WARMING_UP);
      emitMany(59);

      expect(service.screen()).toBe('calibration');
    });

    it('counts during the exercise too', async () => {
      await service.consentAndStart(video);
      fake.emit('estimate', estimate());
      expect(service.screen()).toBe('exercise');

      emitMany(60);

      expect(service.screen()).toBe('welcome');
    });

    it('a new session starts counting from zero', async () => {
      await service.consentAndStart(video);
      emitMany(59);
      service.exit();
      await service.consentAndStart(video);

      emitMany(59);

      expect(service.screen()).toBe('calibration');
    });

    it('ends the exercise by itself after EXERCISE_S and shows the summary', async () => {
      vi.useFakeTimers();
      fake.summaryValue = { hrStart: 70, hrEnd: 64, rrMean: 6, qualityMean: 0.8, durationS: 120, method: 'pos' };
      await service.consentAndStart(video);
      fake.emit('estimate', estimate());
      fake.stop.mockClear();

      vi.advanceTimersByTime(EXERCISE_S * 1000 - 1);
      expect(service.screen()).toBe('exercise');
      vi.advanceTimersByTime(1);

      expect(fake.stop).toHaveBeenCalledTimes(1);
      expect(service.screen()).toBe('summary');
      expect(service.summary()).toEqual(fake.summaryValue);
      expect(service.cameraOn()).toBe(false);
    });

    it('reads the summary before stopping the module', async () => {
      const order: string[] = [];
      fake.summary.mockImplementation(() => {
        order.push('summary');
        return null;
      });
      fake.stop.mockImplementation(() => {
        order.push('stop');
      });
      await service.consentAndStart(video);
      fake.emit('estimate', estimate());

      service.finishExercise();

      expect(order).toEqual(['summary', 'stop']);
    });

    it('does not end the exercise after Exit', async () => {
      vi.useFakeTimers();
      await service.consentAndStart(video);
      fake.emit('estimate', estimate());
      service.exit();
      fake.stop.mockClear();

      vi.advanceTimersByTime(EXERCISE_S * 1000 * 2);

      expect(fake.stop).not.toHaveBeenCalled();
      expect(service.screen()).toBe('welcome');
    });

    it('Start again from the summary returns to welcome with everything cleared', async () => {
      fake.summaryValue = { hrStart: 70, hrEnd: 64, rrMean: 6, qualityMean: 0.8, durationS: 120, method: 'pos' };
      await service.consentAndStart(video);
      fake.emit('estimate', estimate());
      service.showNumbers.set(true);
      service.finishExercise();
      fake.stop.mockClear();

      service.startAgain();

      expect(fake.stop).not.toHaveBeenCalled();
      expect(service.screen()).toBe('welcome');
      expect(service.summary()).toBeNull();
      expect(service.estimate()).toBeNull();
      expect(service.showNumbers()).toBe(false);
    });

    it('returns to welcome after NO_FACE_TIMEOUT_S idle on the summary', async () => {
      vi.useFakeTimers();
      await service.consentAndStart(video);
      fake.emit('estimate', estimate());
      service.finishExercise();

      vi.advanceTimersByTime(NO_FACE_TIMEOUT_S * 1000 - 1);
      expect(service.screen()).toBe('summary');
      vi.advanceTimersByTime(1);

      expect(service.screen()).toBe('welcome');
      expect(service.summary()).toBeNull();
    });

    it('does not come back to welcome later if the summary was left earlier', async () => {
      vi.useFakeTimers();
      await service.consentAndStart(video);
      fake.emit('estimate', estimate());
      service.finishExercise();
      service.startAgain();
      await service.consentAndStart(video);

      vi.advanceTimersByTime(NO_FACE_TIMEOUT_S * 1000);

      expect(service.screen()).toBe('calibration');
    });

    it('an error event during the exercise cancels the exercise timer', async () => {
      vi.useFakeTimers();
      await service.consentAndStart(video);
      fake.emit('estimate', estimate());
      fake.emit('error', { code: 'camera_unavailable' });
      fake.summary.mockClear();

      vi.advanceTimersByTime(EXERCISE_S * 1000 * 2);

      expect(fake.summary).not.toHaveBeenCalled();
      expect(service.screen()).toBe('welcome');
    });

    it('destroying the service cancels the exercise timer', async () => {
      vi.useFakeTimers();
      await service.consentAndStart(video);
      fake.emit('estimate', estimate());
      TestBed.resetTestingModule();
      fake.summary.mockClear();

      vi.advanceTimersByTime(EXERCISE_S * 1000 * 2);

      expect(fake.summary).not.toHaveBeenCalled();
      expect(service.screen()).toBe('exercise');
    });

    it('destroying the service cancels the summary timer', async () => {
      vi.useFakeTimers();
      await service.consentAndStart(video);
      fake.emit('estimate', estimate());
      service.finishExercise();
      TestBed.resetTestingModule();

      vi.advanceTimersByTime(NO_FACE_TIMEOUT_S * 1000 * 2);

      expect(service.screen()).toBe('summary');
    });
  });
});
