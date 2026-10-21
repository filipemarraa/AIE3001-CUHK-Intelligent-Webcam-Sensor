import { vi } from 'vitest';

export class FakeVideoFrame {
  static created: FakeVideoFrame[] = [];
  readonly close = vi.fn();
  constructor(readonly source: unknown) {
    FakeVideoFrame.created.push(this);
  }
}

export class FakeTrack extends EventTarget {
  readyState: MediaStreamTrackState = 'live';
  readonly stop = vi.fn(() => {
    this.readyState = 'ended';
  });

  // A pulled USB camera: the browser ends the track and fires 'ended'.
  unplug(): void {
    this.readyState = 'ended';
    this.dispatchEvent(new Event('ended'));
  }
}

export class FakeStream {
  readonly tracks = [new FakeTrack()];
  getTracks(): FakeTrack[] {
    return this.tracks;
  }
}

export interface FakeFrameMetadata {
  captureTime?: number;
  mediaTime?: number;
  presentedFrames?: number;
}

type FrameCallback = (now: number, metadata: FakeFrameMetadata) => void;

export class FakeVideo {
  srcObject: unknown = null;
  muted = false;
  playsInline = false;
  readonly play = vi.fn(() => Promise.resolve());
  private next = 1;
  readonly callbacks = new Map<number, FrameCallback>();
  readonly requestVideoFrameCallback = vi.fn((cb: FrameCallback) => {
    const id = this.next++;
    this.callbacks.set(id, cb);
    return id;
  });
  readonly cancelVideoFrameCallback = vi.fn((id: number) => {
    this.callbacks.delete(id);
  });

  // Presents one video frame: runs the callbacks registered so far, like the browser does.
  present(at?: number | FakeFrameMetadata): void {
    const pending = [...this.callbacks.values()];
    this.callbacks.clear();
    const metadata: FakeFrameMetadata =
      typeof at === 'object' ? at : at === undefined ? { mediaTime: 0 } : { captureTime: at, mediaTime: 0 };
    for (const cb of pending) cb(0, metadata);
  }

  asElement(): HTMLVideoElement {
    return this as unknown as HTMLVideoElement;
  }
}

export function fakeMedia(result: () => Promise<FakeStream>): {
  media: MediaDevices;
  getUserMedia: ReturnType<typeof vi.fn>;
} {
  const getUserMedia = vi.fn(result);
  return { media: { getUserMedia } as unknown as MediaDevices, getUserMedia };
}

export function domError(name: string): Error {
  const e = new Error(`${name} raised`);
  e.name = name;
  return e;
}

export function deferred<T>(): { promise: Promise<T>; resolve: (v: T) => void; reject: (e: unknown) => void } {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}
