// VitalsError is the module's typed rejection contract, not an Error subclass.
/* eslint-disable @typescript-eslint/only-throw-error */
import type { VitalsError } from './types.js';

const DENIED: ReadonlySet<string> = new Set(['NotAllowedError', 'SecurityError']);

const CONSTRAINTS: MediaStreamConstraints = {
  video: { width: { ideal: 640 }, height: { ideal: 480 }, facingMode: 'user', frameRate: { ideal: 30 } },
  audio: false,
};

const EXPOSURE_POLL_MS = 250;
const EXPOSURE_STABLE_MS = 1000;
const EXPOSURE_TIMEOUT_MS = 6000;

type ExposureCapabilities = { exposureMode?: string[]; whiteBalanceMode?: string[] };
type ExposureSettings = { exposureTime?: number; colorTemperature?: number };

const sleep = (ms: number): Promise<void> =>
  new Promise((resolve) => {
    setTimeout(resolve, ms);
  });

// Auto exposure and white balance shift the skin colour by more than the pulse. Lock them at the values the camera
// settled on, once exposure has stayed the same for a while; a camera that never settles is left on automatic.
export async function lockExposureWhenStable(
  track: MediaStreamTrack,
  wait: (ms: number) => Promise<void> = sleep,
  now: () => number = () => performance.now(),
): Promise<boolean> {
  // Not every browser implements getCapabilities on camera tracks.
  const capable = track as Partial<Pick<MediaStreamTrack, 'getCapabilities'>>;
  const caps = (capable.getCapabilities?.() ?? {}) as ExposureCapabilities;
  if (caps.exposureMode?.includes('manual') !== true) return false;
  // Timers can run late, so stability and the deadline are measured on the clock, not by counting waits.
  const start = now();
  let last: number | undefined;
  let since = start;
  for (;;) {
    const exposure = (track.getSettings() as ExposureSettings).exposureTime;
    if (exposure === undefined) return false;
    const t = now();
    if (exposure !== last) {
      last = exposure;
      since = t;
    }
    if (t - start >= EXPOSURE_TIMEOUT_MS) return false;
    if (t - since >= EXPOSURE_STABLE_MS) break;
    await wait(EXPOSURE_POLL_MS);
  }
  const settings = track.getSettings() as ExposureSettings;
  const lock: Record<string, unknown> = { exposureMode: 'manual', exposureTime: settings.exposureTime };
  if (caps.whiteBalanceMode?.includes('manual') === true) {
    lock['whiteBalanceMode'] = 'manual';
    if (settings.colorTemperature !== undefined) lock['colorTemperature'] = settings.colorTemperature;
  }
  try {
    await track.applyConstraints({ advanced: [lock] });
    return true;
  } catch {
    return false;
  }
}

export function mapCameraError(e: unknown): VitalsError {
  const { name, message } = (typeof e === 'object' && e !== null ? e : {}) as { name?: unknown; message?: unknown };
  const code = typeof name === 'string' && DENIED.has(name) ? 'camera_denied' : 'camera_unavailable';
  return typeof message === 'string' ? { code, message } : { code };
}

function endStream(stream: MediaStream): void {
  for (const track of stream.getTracks()) track.stop();
}

export async function requestCamera(media?: MediaDevices): Promise<MediaStream> {
  const devices = media ?? (globalThis as { navigator?: { mediaDevices?: MediaDevices } }).navigator?.mediaDevices;
  if (devices === undefined) throw { code: 'camera_unavailable', message: 'mediaDevices unavailable' } as VitalsError;
  try {
    return await devices.getUserMedia(CONSTRAINTS);
  } catch (e) {
    throw mapCameraError(e);
  }
}

// On failure the stream is ended; the video is detached only if it still shows this stream.
export async function attachCamera(video: HTMLVideoElement, stream: MediaStream): Promise<void> {
  video.srcObject = stream;
  video.muted = true;
  video.playsInline = true;
  try {
    await video.play();
  } catch (e) {
    endStream(stream);
    if (video.srcObject === stream) video.srcObject = null;
    throw mapCameraError(e);
  }
}

export async function openCamera(video: HTMLVideoElement, media?: MediaDevices): Promise<MediaStream> {
  const stream = await requestCamera(media);
  await attachCamera(video, stream);
  return stream;
}

interface FrameTiming {
  captureTime?: number;
  mediaTime?: number;
  presentedFrames?: number;
}

// The callback can run late, so its own time would jitter the samples. captureTime is the camera's clock; failing
// that, mediaTime (the frame's position in the stream) on a base fixed once per session; the callback time is the
// last resort.
export function createFrameClock(now: () => number = () => performance.now()): (m: FrameTiming) => number {
  let base: number | null = null;
  let lastMedia = Number.NEGATIVE_INFINITY;
  let lastOut = Number.NEGATIVE_INFINITY;
  // Switching between time sources must never step back: the engine drops samples that are not newer.
  const forward = (t: number): number => {
    lastOut = Math.max(t, lastOut + 0.001);
    return lastOut;
  };
  return (m) => {
    if (m.captureTime !== undefined && Number.isFinite(m.captureTime)) return forward(m.captureTime);
    const media = m.mediaTime;
    const t = now();
    if (media === undefined || !Number.isFinite(media) || media <= lastMedia) {
      base = null;
      lastMedia = media ?? Number.NEGATIVE_INFINITY;
      return forward(t);
    }
    lastMedia = media;
    if (base === null || Math.abs(base + media * 1000 - t) > 1000) base = t - media * 1000;
    return forward(base + media * 1000);
  };
}

export function startFrameLoop(
  video: HTMLVideoElement,
  send: (frame: VideoFrame, tMs: number) => void,
  isBusy: () => boolean,
  onDrop: () => void,
): () => void {
  let handle: number | null = null;
  const clock = createFrameClock();
  let lastPresented: number | null = null;

  const onFrame = (_now: DOMHighResTimeStamp, metadata: VideoFrameCallbackMetadata): void => {
    handle = video.requestVideoFrameCallback(onFrame);
    const presented = (metadata as FrameTiming).presentedFrames;
    if (presented !== undefined && lastPresented !== null) {
      for (let missed = presented - lastPresented - 1; missed > 0; missed--) onDrop();
    }
    if (presented !== undefined) lastPresented = presented;
    const tMs = clock(metadata);
    // At most one frame in flight: a frame is dropped only while the worker still holds the previous one.
    if (isBusy()) {
      onDrop();
      return;
    }
    let frame: VideoFrame;
    try {
      frame = new VideoFrame(video);
    } catch {
      onDrop();
      return;
    }
    try {
      send(frame, tMs);
    } catch {
      frame.close();
      onDrop();
    }
  };

  handle = video.requestVideoFrameCallback(onFrame);
  return () => {
    if (handle !== null) video.cancelVideoFrameCallback(handle);
    handle = null;
  };
}
