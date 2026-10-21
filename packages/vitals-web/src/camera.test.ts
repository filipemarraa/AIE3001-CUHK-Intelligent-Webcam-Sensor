import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mapCameraError, openCamera, startFrameLoop } from './camera.js';
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
  afterEach(() => vi.unstubAllGlobals());

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
