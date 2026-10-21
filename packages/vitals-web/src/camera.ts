// VitalsError is the module's typed rejection contract, not an Error subclass.
/* eslint-disable @typescript-eslint/only-throw-error */
import type { VitalsError } from './types.js';

const DENIED: ReadonlySet<string> = new Set(['NotAllowedError', 'SecurityError']);

const CONSTRAINTS: MediaStreamConstraints = {
  video: { width: { ideal: 640 }, height: { ideal: 480 }, facingMode: 'user', frameRate: { ideal: 30 } },
  audio: false,
};

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

export function startFrameLoop(
  video: HTMLVideoElement,
  send: (frame: VideoFrame, tMs: number) => void,
  isBusy: () => boolean,
  onDrop: () => void,
): () => void {
  let handle: number | null = null;

  const onFrame = (_now: DOMHighResTimeStamp, metadata: VideoFrameCallbackMetadata): void => {
    handle = video.requestVideoFrameCallback(onFrame);
    let frame: VideoFrame;
    try {
      frame = new VideoFrame(video);
    } catch {
      onDrop();
      return;
    }
    if (isBusy()) {
      frame.close();
      onDrop();
      return;
    }
    try {
      send(frame, metadata.captureTime ?? performance.now());
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
