import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createFrameClock, lockExposureWhenStable, mapCameraError, openCamera, startFrameLoop } from './camera.js';
import { FakeStream, FakeVideo, FakeVideoFrame, domError, fakeMedia } from './test/media.js';

describe('mapCameraError', () => {
  it.each([
    ['NotAllowedError', 'camera_denied'],
    ['SecurityError', 'camera_denied'],
    ['NotFoundError', 'camera_unavailable'],
    ['NotReadableError', 'camera_unavailable'],
    ['OverconstrainedError', 'camera_unavailable'],
    ['AbortError', 'camera_unavailable'],
    ['TypeError', 'camera_unavailable'],
  ])('maps %s to %s', (name, code) => {
    expect(mapCameraError(domError(name))).toEqual({ code, message: `${name} raised` });
  });

  it('maps a non-error value to camera_unavailable', () => {
    expect(mapCameraError('boom').code).toBe('camera_unavailable');
  });
});

describe('openCamera', () => {
  it('asks for the front camera at 640x480 and 30 fps and plays it muted inline', async () => {
    const stream = new FakeStream();
    const { media, getUserMedia } = fakeMedia(() => Promise.resolve(stream));
    const video = new FakeVideo();

    await expect(openCamera(video.asElement(), media)).resolves.toBe(stream);

    expect(getUserMedia).toHaveBeenCalledWith({
      video: { width: { ideal: 640 }, height: { ideal: 480 }, facingMode: 'user', frameRate: { ideal: 30 } },
      audio: false,
    });
    expect(video.srcObject).toBe(stream);
    expect(video.muted).toBe(true);
    expect(video.playsInline).toBe(true);
    expect(video.play).toHaveBeenCalledOnce();
  });

  it('throws a VitalsError when the camera is refused', async () => {
    const { media } = fakeMedia(() => Promise.reject(domError('NotAllowedError')));
    await expect(openCamera(new FakeVideo().asElement(), media)).rejects.toMatchObject({ code: 'camera_denied' });
  });

  it('ends the tracks and detaches the stream when play fails', async () => {
    const stream = new FakeStream();
    const { media } = fakeMedia(() => Promise.resolve(stream));
    const video = new FakeVideo();
    video.play.mockReturnValueOnce(Promise.reject(domError('AbortError')));

    await expect(openCamera(video.asElement(), media)).rejects.toMatchObject({ code: 'camera_unavailable' });
    expect(stream.tracks.every((t) => t.readyState === 'ended')).toBe(true);
    expect(video.srcObject).toBeNull();
  });
});

describe('startFrameLoop', () => {
  beforeEach(() => {
    FakeVideoFrame.created = [];
    vi.stubGlobal('VideoFrame', FakeVideoFrame);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('sends one frame per presented video frame with its capture time', () => {
    const video = new FakeVideo();
    const send = vi.fn();
    startFrameLoop(video.asElement(), send, () => false, vi.fn());

    video.present(1234);
    video.present(1267);

    expect(send.mock.calls.map(([, t]) => t as number)).toEqual([1234, 1267]);
    expect(send.mock.calls[0]?.[0]).toBe(FakeVideoFrame.created[0]);
    expect(FakeVideoFrame.created[0]?.source).toBe(video);
  });

  it('falls back to performance.now() without a capture time', () => {
    vi.spyOn(performance, 'now').mockReturnValue(99);
    const video = new FakeVideo();
    const send = vi.fn();
    startFrameLoop(video.asElement(), send, () => false, vi.fn());

    video.present();

    expect(send).toHaveBeenCalledWith(FakeVideoFrame.created[0], 99);
  });

  it('places frames on a fixed session clock from mediaTime when there is no capture time', () => {
    const now = vi.spyOn(performance, 'now').mockReturnValue(1000);
    const video = new FakeVideo();
    const send = vi.fn();
    startFrameLoop(video.asElement(), send, () => false, vi.fn());

    video.present({ mediaTime: 0.5 });
    now.mockReturnValue(1100);
    video.present({ mediaTime: 0.533 });
    now.mockReturnValue(1500);
    video.present({ mediaTime: 0.567 });

    expect(send.mock.calls.map(([, t]) => t as number)).toEqual([1000, 1033, 1067]);
  });

  it('falls back to performance.now() while mediaTime does not advance', () => {
    const now = vi.spyOn(performance, 'now').mockReturnValue(500);
    const video = new FakeVideo();
    const send = vi.fn();
    startFrameLoop(video.asElement(), send, () => false, vi.fn());

    video.present({ mediaTime: 0 });
    now.mockReturnValue(533);
    video.present({ mediaTime: 0 });

    expect(send.mock.calls.map(([, t]) => t as number)).toEqual([500, 533]);
  });

  it('never goes back in time when it falls back from mediaTime to the callback time', () => {
    const now = [1000, 1010, 1020, 1030];
    let i = 0;
    const clock = createFrameClock(() => now[i++] ?? 1040);
    const out = [clock({ mediaTime: 0.5 }), clock({ mediaTime: 0.533 }), clock({ mediaTime: 0.533 }), clock({})];
    for (let k = 1; k < out.length; k++) expect(out[k]).toBeGreaterThan(out[k - 1] ?? 0);
  });

  it('counts frames the page never saw as dropped', () => {
    const video = new FakeVideo();
    const onDrop = vi.fn();
    startFrameLoop(video.asElement(), vi.fn(), () => false, onDrop);

    video.present({ captureTime: 1000, presentedFrames: 10 });
    video.present({ captureTime: 1033, presentedFrames: 11 });
    video.present({ captureTime: 1133, presentedFrames: 14 });

    expect(onDrop).toHaveBeenCalledTimes(2);
  });

  it('closes and drops frames while the worker is busy', () => {
    const video = new FakeVideo();
    const send = vi.fn();
    const onDrop = vi.fn();
    let busy = true;
    startFrameLoop(video.asElement(), send, () => busy, onDrop);

    video.present(1);
    video.present(2);
    busy = false;
    video.present(3);

    expect(send).toHaveBeenCalledOnce();
    expect(send).toHaveBeenCalledWith(FakeVideoFrame.created[2], 3);
    expect(onDrop).toHaveBeenCalledTimes(2);
    expect(FakeVideoFrame.created[0]?.close).toHaveBeenCalledOnce();
    expect(FakeVideoFrame.created[1]?.close).toHaveBeenCalledOnce();
    expect(FakeVideoFrame.created[2]?.close).not.toHaveBeenCalled();
  });

  it('closes and drops a frame that could not be sent', () => {
    const video = new FakeVideo();
    const onDrop = vi.fn();
    startFrameLoop(
      video.asElement(),
      () => {
        throw new Error('post failed');
      },
      () => false,
      onDrop,
    );

    video.present(1);

    expect(FakeVideoFrame.created[0]?.close).toHaveBeenCalledOnce();
    expect(onDrop).toHaveBeenCalledOnce();
    expect(video.callbacks.size).toBe(1);
  });

  it('stops requesting frames once cancelled', () => {
    const video = new FakeVideo();
    const send = vi.fn();
    const cancel = startFrameLoop(video.asElement(), send, () => false, vi.fn());
    video.present(1);

    cancel();
    video.present(2);

    expect(video.cancelVideoFrameCallback).toHaveBeenCalledOnce();
    expect(video.callbacks.size).toBe(0);
    expect(send).toHaveBeenCalledOnce();
  });
});

describe('lockExposureWhenStable', () => {
  function track(exposures: number[], caps: Record<string, unknown>, apply = vi.fn(() => Promise.resolve())) {
    let i = 0;
    return {
      track: {
        getCapabilities: () => caps,
        getSettings: () => ({ exposureTime: exposures[Math.min(i++, exposures.length - 1)], colorTemperature: 4600 }),
        applyConstraints: apply,
      } as unknown as MediaStreamTrack,
      apply,
    };
  }
  const manual = { exposureMode: ['continuous', 'manual'], whiteBalanceMode: ['continuous', 'manual'] };
  // A fake clock that advances by the requested wait, so the 1 s and 6 s rules are exercised without real time.
  function clock(): { wait: (ms: number) => Promise<void>; now: () => number } {
    let t = 0;
    return {
      wait: (ms) => {
        t += ms;
        return Promise.resolve();
      },
      now: () => t,
    };
  }

  it('measures the deadline on the clock, not by counting waits', async () => {
    let t = 0;
    const slowWait = (): Promise<void> => {
      t += 1000;
      return Promise.resolve();
    };
    const x = track(
      Array.from({ length: 100 }, (_, k) => 200 + k),
      manual,
    );
    await expect(lockExposureWhenStable(x.track, slowWait, () => t)).resolves.toBe(false);
    expect(t).toBeLessThanOrEqual(7000);
    expect(x.apply).not.toHaveBeenCalled();
  });

  it('needs a full second of steady exposure on the clock before locking', async () => {
    let t = 0;
    const wait = (): Promise<void> => {
      t += 600;
      return Promise.resolve();
    };
    const x = track([250, 250, 250, 250], manual);
    await expect(lockExposureWhenStable(x.track, wait, () => t)).resolves.toBe(true);
    expect(t).toBeGreaterThanOrEqual(1000);
  });

  it('locks exposure and white balance at the current values once exposure is steady', async () => {
    const t = track([300, 280, 250, 250, 250, 250, 250], manual);
    const c = clock();
    await expect(lockExposureWhenStable(t.track, c.wait, c.now)).resolves.toBe(true);
    expect(t.apply).toHaveBeenCalledWith({
      advanced: [{ exposureMode: 'manual', exposureTime: 250, whiteBalanceMode: 'manual', colorTemperature: 4600 }],
    });
  });

  it('leaves the camera alone when it has no manual exposure', async () => {
    const t = track([250], { exposureMode: ['continuous'] });
    const c = clock();
    await expect(lockExposureWhenStable(t.track, c.wait, c.now)).resolves.toBe(false);
    expect(t.apply).not.toHaveBeenCalled();
  });

  it('gives up without locking when exposure never settles', async () => {
    const t = track(
      Array.from({ length: 100 }, (_, k) => 200 + k),
      manual,
    );
    const c = clock();
    await expect(lockExposureWhenStable(t.track, c.wait, c.now)).resolves.toBe(false);
    expect(t.apply).not.toHaveBeenCalled();
  });

  it('reports false when the browser refuses the constraints', async () => {
    const t = track(
      [250],
      manual,
      vi.fn(() => Promise.reject(new Error('OverconstrainedError'))),
    );
    const c = clock();
    await expect(lockExposureWhenStable(t.track, c.wait, c.now)).resolves.toBe(false);
  });
});
