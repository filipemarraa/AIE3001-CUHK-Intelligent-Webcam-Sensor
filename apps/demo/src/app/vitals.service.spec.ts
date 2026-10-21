import { TestBed } from '@angular/core/testing';
import { beforeEach, describe, expect, it } from 'vitest';
import { FakeVitals, WARMING_UP, estimate } from './testing/fake-vitals';
import { VITALS_FACTORY, VitalsService } from './vitals.service';

describe('VitalsService', () => {
  let fake: FakeVitals;
  let service: VitalsService;
  const video = document.createElement('video');

  beforeEach(() => {
    fake = new FakeVitals();
    TestBed.configureTestingModule({ providers: [{ provide: VITALS_FACTORY, useValue: () => fake }] });
    service = TestBed.inject(VitalsService);
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
});
