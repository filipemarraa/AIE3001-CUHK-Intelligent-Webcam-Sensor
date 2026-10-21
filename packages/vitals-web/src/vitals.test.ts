import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Diagnostics, Estimate, VitalsError, VitalsOptions, VitalsState } from './types.js';
import { createVitals } from './vitals.js';
import type { FromWorker, ToWorker } from './worker/protocol.js';
import { FakeStream, FakeVideo, FakeVideoFrame, deferred, domError, fakeMedia } from './test/media.js';

class FakeWorker {
  readonly posted: { msg: ToWorker; transfer: Transferable[] | undefined }[] = [];
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: ((e: Event) => void) | null = null;
  readonly terminate = vi.fn();

  postMessage(msg: ToWorker, transfer?: Transferable[]): void {
    this.posted.push({ msg, transfer });
  }

  send(data: unknown): void {
    this.onmessage?.({ data } as MessageEvent);
  }
}

function estimate(over: Partial<Estimate> = {}): Estimate {
  return {
    tMs: 0,
    hrBpm: null,
    rrRpm: null,
    breathPhase: null,
    quality: 0.3,
    confidence: 'invalid',
    reason: 'warming_up',
    method: 'pos',
    ...over,
  };
}

function valid(hrBpm: number, quality: number, rrRpm = 12): Estimate {
  return estimate({ hrBpm, rrRpm, breathPhase: 'in', quality, confidence: 'high', reason: null });
}

const flush = (): Promise<void> => new Promise((r) => setTimeout(r, 0));

function setup(
  camera: () => Promise<FakeStream> = () => Promise.resolve(new FakeStream()),
  extra: Partial<VitalsOptions> = {},
) {
  const workers: FakeWorker[] = [];
  const { media, getUserMedia } = fakeMedia(camera);
  const vitals = createVitals(
    { assetsBaseUrl: '/vitals/assets/', ...extra },
    {
      createWorker: () => {
        const w = new FakeWorker();
        workers.push(w);
        return w as unknown as Worker;
      },
      media,
    },
  );
  const video = new FakeVideo();
  const states: VitalsState[] = [];
  const errors: VitalsError[] = [];
  const estimates: Estimate[] = [];
  const diagnostics: Diagnostics[] = [];
  vitals.on('state', (s) => states.push(s));
  vitals.on('error', (e) => errors.push(e));
  vitals.on('estimate', (e) => estimates.push(e));
  vitals.on('diagnostics', (d) => diagnostics.push(d));
  const worker = (): FakeWorker => {
    const w = workers[0];
    if (w === undefined) throw new Error('no worker created');
    return w;
  };
  return { vitals, video, workers, worker, getUserMedia, states, errors, estimates, diagnostics };
}

async function running(camera?: () => Promise<FakeStream>) {
  const t = setup(camera);
  const started = t.vitals.start({ video: t.video.asElement() });
  await flush();
  t.worker().send({ type: 'ready' } satisfies FromWorker);
  await started;
  return t;
}

describe('createVitals', () => {
  let win: EventTarget;

  beforeEach(() => {
    FakeVideoFrame.created = [];
    win = new EventTarget();
    vi.stubGlobal('window', win);
    vi.stubGlobal('location', { href: 'http://localhost:4200/demo/' });
    vi.stubGlobal('VideoFrame', FakeVideoFrame);
    vi.stubGlobal('OffscreenCanvas', vi.fn());
    vi.stubGlobal('Worker', FakeWorker);
    vi.stubGlobal('HTMLVideoElement', { prototype: { requestVideoFrameCallback: vi.fn() } });
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('opens the camera, inits the worker with an absolute assets URL and calibrates once ready', async () => {
    const t = setup();
    const started = t.vitals.start({ video: t.video.asElement() });
    await flush();

    expect(t.states).toEqual(['starting']);
    expect(t.getUserMedia).toHaveBeenCalledOnce();
    expect(t.worker().posted.map((p) => p.msg)).toEqual([
      { type: 'init', assetsBaseUrl: 'http://localhost:4200/vitals/assets' },
    ]);

    t.worker().send({ type: 'ready' });
    await started;
    expect(t.states).toEqual(['starting', 'calibrating']);
  });

  it('asks the worker to record only with onSamples, and hands the rows over', async () => {
    const onSamples = vi.fn();
    const t = setup(undefined, { onSamples });
    const started = t.vitals.start({ video: t.video.asElement() });
    await flush();
    expect(t.worker().posted[0]?.msg).toEqual({
      type: 'init',
      assetsBaseUrl: 'http://localhost:4200/vitals/assets',
      record: true,
    });
    t.worker().send({ type: 'ready' });
    await started;
    t.worker().send({ type: 'samples', rows: [[1, 2, 3]] });
    expect(onSamples).toHaveBeenCalledWith([[1, 2, 3]]);
  });

  it('measures on the first estimate that is not invalid', async () => {
    const t = await running();
    t.worker().send({ type: 'estimate', estimate: estimate() });
    expect(t.states.at(-1)).toBe('calibrating');

    t.worker().send({ type: 'estimate', estimate: valid(70, 0.8) });
    t.worker().send({ type: 'estimate', estimate: valid(71, 0.8) });
    expect(t.states).toEqual(['starting', 'calibrating', 'measuring']);
    expect(t.estimates).toHaveLength(3);
  });

  it('transfers each frame to the worker and drops frames until the worker is done', async () => {
    const t = await running();
    t.video.present(10);
    t.video.present(20);
    t.video.present(30);
    t.worker().send({ type: 'frameDone' });
    t.video.present(40);

    const frames = t.worker().posted.filter((p) => p.msg.type === 'frame');
    expect(frames.map((p) => p.msg)).toEqual([
      { type: 'frame', frame: FakeVideoFrame.created[0], tMs: 10 },
      { type: 'frame', frame: FakeVideoFrame.created[3], tMs: 40 },
    ]);
    expect(frames.map((p) => p.transfer)).toEqual([[FakeVideoFrame.created[0]], [FakeVideoFrame.created[3]]]);
    expect(FakeVideoFrame.created[1]?.close).toHaveBeenCalledOnce();
    expect(FakeVideoFrame.created[2]?.close).toHaveBeenCalledOnce();
  });

  it('re-emits diagnostics with the frames dropped on the main thread since the last report', async () => {
    const t = await running();
    t.video.present(10);
    t.video.present(20);
    t.video.present(30);
    t.worker().send({ type: 'diagnostics', diagnostics: { frameCostMs: 12, droppedFrames: 0, pulse: [0.1, 0.2] } });
    t.worker().send({ type: 'diagnostics', diagnostics: { frameCostMs: 11, droppedFrames: 0, pulse: [] } });

    expect(t.diagnostics).toEqual([
      { frameCostMs: 12, droppedFrames: 2, pulse: [0.1, 0.2] },
      { frameCostMs: 11, droppedFrames: 0, pulse: [] },
    ]);
  });

  it('drops a message that is not part of the protocol', async () => {
    const t = await running();
    t.worker().send({ type: 'pixels', data: new Uint8Array(4) });
    t.worker().send({ type: 'estimate', estimate: estimate(), frame: new ArrayBuffer(8) });
    t.worker().send(null);
    expect(t.estimates).toEqual([]);
    expect(t.states).toEqual(['starting', 'calibrating']);
  });

  it('reports a denied camera, stops and rejects start', async () => {
    const t = setup(() => Promise.reject(domError('NotAllowedError')));
    await expect(t.vitals.start({ video: t.video.asElement() })).rejects.toMatchObject({ code: 'camera_denied' });

    expect(t.errors).toEqual([{ code: 'camera_denied', message: 'NotAllowedError raised' }]);
    expect(t.states).toEqual(['starting', 'stopped']);
    for (const w of t.workers) expect(w.terminate).toHaveBeenCalled();
  });

  it('reports a worker failure during start and rejects start', async () => {
    const stream = new FakeStream();
    const t = setup(() => Promise.resolve(stream));
    const started = t.vitals.start({ video: t.video.asElement() });
    await flush();
    t.worker().send({ type: 'error', code: 'wasm_failed', message: 'no wasm' });

    await expect(started).rejects.toEqual({ code: 'wasm_failed', message: 'no wasm' });
    expect(t.errors).toEqual([{ code: 'wasm_failed', message: 'no wasm' }]);
    expect(t.states).toEqual(['starting', 'stopped']);
    expect(t.worker().terminate).toHaveBeenCalledOnce();
    expect(stream.tracks[0]?.readyState).toBe('ended');
  });

  it('treats a worker that fails to load as wasm_failed', async () => {
    const t = setup();
    const started = t.vitals.start({ video: t.video.asElement() });
    await flush();
    t.worker().onerror?.(new Event('error'));

    await expect(started).rejects.toEqual({ code: 'wasm_failed', message: 'sensor worker failed to load' });
    expect(t.states).toEqual(['starting', 'stopped']);
  });

  it('stops everything on a worker error mid-session', async () => {
    const stream = new FakeStream();
    const t = await running(() => Promise.resolve(stream));
    t.worker().send({ type: 'error', code: 'vision_failed', message: 'detector crashed' });

    expect(t.errors).toEqual([{ code: 'vision_failed', message: 'detector crashed' }]);
    expect(t.states.at(-1)).toBe('stopped');
    expect(t.worker().terminate).toHaveBeenCalledOnce();
    expect(stream.tracks[0]?.readyState).toBe('ended');
    expect(t.video.callbacks.size).toBe(0);
  });

  it('stop ends every track, terminates the worker and cancels the frame loop', async () => {
    const stream = new FakeStream();
    const t = await running(() => Promise.resolve(stream));
    t.video.present(10);

    t.vitals.stop();
    t.vitals.stop();
    t.worker().send({ type: 'estimate', estimate: valid(70, 0.8) });
    t.video.present(20);

    expect(stream.tracks.every((tr) => tr.readyState === 'ended')).toBe(true);
    expect(t.worker().terminate).toHaveBeenCalledOnce();
    expect(t.video.cancelVideoFrameCallback).toHaveBeenCalled();
    expect(t.video.callbacks.size).toBe(0);
    expect(t.video.srcObject).toBeNull();
    expect(t.states).toEqual(['starting', 'calibrating', 'stopped']);
    expect(t.estimates).toEqual([]);
    expect(FakeVideoFrame.created).toHaveLength(1);
  });

  it('reports camera_unavailable when the camera is disconnected mid-session (Review Focus 1)', async () => {
    const stream = new FakeStream();
    const t = await running(() => Promise.resolve(stream));
    stream.tracks[0]?.unplug();

    expect(t.errors).toEqual([{ code: 'camera_unavailable', message: 'camera disconnected' }]);
    expect(t.states).toEqual(['starting', 'calibrating', 'stopped']);
    expect(t.worker().terminate).toHaveBeenCalledOnce();
    expect(t.video.callbacks.size).toBe(0);
  });

  it('opens a single camera when start is called twice (Review Focus 4)', async () => {
    const t = setup();
    const a = t.vitals.start({ video: t.video.asElement() });
    const b = t.vitals.start({ video: t.video.asElement() });
    await flush();
    t.worker().send({ type: 'ready' });
    await Promise.all([a, b]);

    expect(t.getUserMedia).toHaveBeenCalledOnce();
    expect(t.workers).toHaveLength(1);
    expect(t.states).toEqual(['starting', 'calibrating']);
  });

  it('ends the late stream and leaves no worker when stopped while starting (Review Focus 4)', async () => {
    const stream = new FakeStream();
    const camera = deferred<FakeStream>();
    const t = setup(() => camera.promise);
    const started = t.vitals.start({ video: t.video.asElement() });
    await flush();

    t.vitals.stop();
    camera.resolve(stream);
    await started;
    await flush();

    expect(stream.tracks.every((tr) => tr.readyState === 'ended')).toBe(true);
    expect(t.video.srcObject).toBeNull();
    for (const w of t.workers) expect(w.terminate).toHaveBeenCalledOnce();
    expect(t.states).toEqual(['starting', 'stopped']);
    expect(t.errors).toEqual([]);
  });

  it('never lets a stale stream touch the video of a newer session (Review Focus 4)', async () => {
    const first = deferred<FakeStream>();
    const second = deferred<FakeStream>();
    const cameras = [first, second];
    let calls = 0;
    const t = setup(() => {
      const d = cameras[calls++];
      if (d === undefined) throw new Error('unexpected getUserMedia');
      return d.promise;
    });
    const a = t.vitals.start({ video: t.video.asElement() });
    await flush();
    t.vitals.stop();
    await a;
    const b = t.vitals.start({ video: t.video.asElement() });
    const stream2 = new FakeStream();
    second.resolve(stream2);
    await flush();
    t.workers[1]?.send({ type: 'ready' });
    await b;

    const stream1 = new FakeStream();
    first.resolve(stream1);
    await flush();

    expect(t.video.srcObject).toBe(stream2);
    expect(t.video.play).toHaveBeenCalledOnce();
    expect(stream1.tracks.every((tr) => tr.readyState === 'ended')).toBe(true);
    expect(stream2.tracks.every((tr) => tr.readyState === 'live')).toBe(true);
    expect(t.states).toEqual(['starting', 'stopped', 'starting', 'calibrating']);
  });

  it('ends the tracks at once when stopped while the video is starting to play', async () => {
    const stream = new FakeStream();
    const t = setup(() => Promise.resolve(stream));
    const play = deferred<undefined>();
    t.video.play.mockReturnValueOnce(play.promise);
    const started = t.vitals.start({ video: t.video.asElement() });
    await flush();

    t.vitals.stop();
    expect(stream.tracks.every((tr) => tr.readyState === 'ended')).toBe(true);
    expect(t.video.srcObject).toBeNull();

    play.reject(domError('AbortError'));
    await started;
    await flush();
    expect(t.errors).toEqual([]);
    expect(t.states).toEqual(['starting', 'stopped']);
  });

  it('fails with vision_failed when the worker never becomes ready', async () => {
    vi.useFakeTimers();
    try {
      const stream = new FakeStream();
      const t = setup(() => Promise.resolve(stream));
      const started = t.vitals.start({ video: t.video.asElement() });
      const outcome = expect(started).rejects.toEqual({
        code: 'vision_failed',
        message: 'worker did not become ready',
      });

      await vi.advanceTimersByTimeAsync(29_999);
      expect(t.states).toEqual(['starting']);
      await vi.advanceTimersByTimeAsync(1);

      await outcome;
      expect(t.errors).toEqual([{ code: 'vision_failed', message: 'worker did not become ready' }]);
      expect(t.states).toEqual(['starting', 'stopped']);
      expect(t.worker().terminate).toHaveBeenCalledOnce();
      expect(stream.tracks[0]?.readyState).toBe('ended');
    } finally {
      vi.useRealTimers();
    }
  });

  it('does not time out once the worker is ready', async () => {
    vi.useFakeTimers();
    try {
      const t = setup();
      const started = t.vitals.start({ video: t.video.asElement() });
      await vi.advanceTimersByTimeAsync(0);
      t.worker().send({ type: 'ready' });
      await started;
      await vi.advanceTimersByTimeAsync(60_000);
      expect(t.errors).toEqual([]);
      expect(t.states).toEqual(['starting', 'calibrating']);
    } finally {
      vi.useRealTimers();
    }
  });

  it('reports a worker crash after ready as wasm_failed without calling it a load failure', async () => {
    const t = await running();
    t.worker().onerror?.(new Event('error'));
    expect(t.errors).toEqual([{ code: 'wasm_failed', message: 'sensor worker crashed' }]);
    expect(t.states.at(-1)).toBe('stopped');
  });

  it('has no summary after a failed start', async () => {
    const t = setup(() => Promise.reject(domError('NotAllowedError')));
    await expect(t.vitals.start({ video: t.video.asElement() })).rejects.toMatchObject({ code: 'camera_denied' });
    expect(t.vitals.summary()).toBeNull();
  });

  it('stops on pagehide', async () => {
    const stream = new FakeStream();
    const t = await running(() => Promise.resolve(stream));
    win.dispatchEvent(new Event('pagehide'));

    expect(t.states.at(-1)).toBe('stopped');
    expect(stream.tracks[0]?.readyState).toBe('ended');
    expect(t.worker().terminate).toHaveBeenCalledOnce();

    win.dispatchEvent(new Event('pagehide'));
    expect(t.states.filter((s) => s === 'stopped')).toHaveLength(1);
  });

  it.each(['requestVideoFrameCallback', 'VideoFrame', 'OffscreenCanvas', 'Worker'])(
    'reports unsupported_browser without %s, before touching the camera',
    async (missing) => {
      if (missing === 'requestVideoFrameCallback') vi.stubGlobal('HTMLVideoElement', { prototype: {} });
      else vi.stubGlobal(missing, undefined);
      const t = setup();

      await expect(t.vitals.start({ video: t.video.asElement() })).rejects.toMatchObject({
        code: 'unsupported_browser',
      });
      expect(t.getUserMedia).not.toHaveBeenCalled();
      expect(t.workers).toHaveLength(0);
      expect(t.states).toEqual(['starting', 'stopped']);
    },
  );

  it('summarises the session like the mock', async () => {
    const now = vi.spyOn(performance, 'now').mockReturnValue(1_000);
    const t = setup();
    expect(t.vitals.summary()).toBeNull();
    const started = t.vitals.start({ video: t.video.asElement() });
    await flush();
    t.worker().send({ type: 'ready' });
    await started;

    t.worker().send({ type: 'estimate', estimate: estimate({ quality: 0.1 }) });
    t.worker().send({ type: 'estimate', estimate: valid(72, 0.8, 11) });
    t.worker().send({ type: 'estimate', estimate: estimate({ quality: 0.2, reason: 'too_much_motion' }) });
    t.worker().send({ type: 'estimate', estimate: valid(66, 0.6, 12) });
    now.mockReturnValue(43_900);
    t.vitals.stop();
    now.mockReturnValue(99_000);

    expect(t.vitals.summary()).toEqual({
      hrStart: 72,
      hrEnd: 66,
      rrMean: 11.5,
      qualityMean: 0.53,
      durationS: 42,
      method: 'pos',
    });
  });
});
