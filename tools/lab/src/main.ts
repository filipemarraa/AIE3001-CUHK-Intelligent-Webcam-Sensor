import './lab.css';
import { createVitals, type CaptureOptions, type Estimate, type RoiOverlay, type Vitals } from 'vitals-web';
import { diagnosticsLine, estimateLine, headerLine, parseHeartRate, referenceLine, sampleLines } from './jsonl.js';

function el<T extends HTMLElement>(id: string, type: new () => T): T {
  const found = document.getElementById(id);
  if (!(found instanceof type)) throw new Error(`#${id} is missing`);
  return found;
}

const video = el('video', HTMLVideoElement);
const overlay = el('overlay', HTMLCanvasElement);
const subject = el('subject', HTMLInputElement);
const condition = el('condition', HTMLSelectElement);
const skin = el('skin', HTMLSelectElement);
const threshold = el('threshold', HTMLSelectElement);
const robust = el('robust', HTMLInputElement);
const exposure = el('exposure', HTMLInputElement);
const startButton = el('start', HTMLButtonElement);
const stopButton = el('stop', HTMLButtonElement);
const downloadButton = el('download', HTMLButtonElement);
const status = el('status', HTMLParagraphElement);
const watch = el('watch', HTMLInputElement);
const references = el('references', HTMLOListElement);

let vitals: Vitals | null = null;
let lines: string[] = [];
let samples = 0;
let faces = 0;
let header: string | null = null;
let readingStartedAt: number | null = null;
let startedAt = 0;

function clearReading(): void {
  watch.value = '';
  readingStartedAt = null;
}

function captureOptions(): CaptureOptions {
  return {
    skin: skin.value === 'anatomical' ? 'anatomical' : 'boxes',
    minFaceConfidence: Number(threshold.value),
    robustPixels: robust.checked,
    lockExposure: exposure.checked,
  };
}

function show(id: string, text: string): void {
  el(id, HTMLElement).textContent = text;
}

function showEstimate(e: Estimate): void {
  show('hr', e.hrBpm === null ? '-' : `${e.hrBpm.toFixed(1)} bpm`);
  show('rr', e.rrRpm === null ? '-' : `${e.rrRpm.toFixed(1)} /min, ${e.breathPhase ?? '-'}`);
  show('confidence', e.reason === null ? e.confidence : `${e.confidence} (${e.reason})`);
}

function drawOverlay(roi: RoiOverlay): void {
  const width = video.clientWidth;
  const height = video.clientHeight;
  if (overlay.width !== width) overlay.width = width;
  if (overlay.height !== height) overlay.height = height;
  const ctx = overlay.getContext('2d');
  if (ctx === null) return;
  const sx = width / roi.width;
  const sy = height / roi.height;
  ctx.clearRect(0, 0, width, height);
  ctx.lineWidth = 2;
  ctx.strokeStyle = '#f5b301';
  for (const b of roi.boxes) ctx.strokeRect(b.x * sx, b.y * sy, b.w * sx, b.h * sy);
  ctx.strokeStyle = '#21c07a';
  ctx.fillStyle = 'rgba(33, 192, 122, 0.25)';
  for (const p of roi.polygons) {
    ctx.beginPath();
    p.points.forEach((q, i) => {
      if (i === 0) ctx.moveTo(q.x * sx, q.y * sy);
      else ctx.lineTo(q.x * sx, q.y * sy);
    });
    ctx.closePath();
    ctx.fill();
    ctx.stroke();
  }
}

function cameraOf(v: HTMLVideoElement): { label: string; width?: number; height?: number; frameRate?: number } {
  const track = v.srcObject instanceof MediaStream ? v.srcObject.getVideoTracks()[0] : undefined;
  const s = track?.getSettings() ?? {};
  return {
    label: track?.label ?? '',
    ...(s.width === undefined ? {} : { width: s.width }),
    ...(s.height === undefined ? {} : { height: s.height }),
    ...(s.frameRate === undefined ? {} : { frameRate: s.frameRate }),
  };
}

async function start(): Promise<void> {
  const options = captureOptions();
  lines = [];
  samples = 0;
  faces = 0;
  header = null;
  clearReading();
  references.replaceChildren();
  startButton.disabled = true;
  downloadButton.disabled = true;
  const v = createVitals(
    {
      assetsBaseUrl: '/',
      capture: options,
      onSamples: (rows, skins) => {
        lines.push(...sampleLines(rows, skins));
        samples += rows.length;
        faces += rows.filter((r) => r[4] === 1).length;
        show('samples', `${String(samples)} / ${String(Math.round((100 * faces) / Math.max(1, samples)))}%`);
      },
      onRoi: drawOverlay,
    },
    { createWorker: () => new Worker(new URL('./sensor.worker.ts', import.meta.url), { type: 'module' }) },
  );
  vitals = v;
  v.on('estimate', (e) => {
    lines.push(estimateLine(e));
    showEstimate(e);
  });
  v.on('diagnostics', (d) => {
    lines.push(diagnosticsLine(d, performance.now()));
    show('cost', `${d.frameCostMs.toFixed(1)} ms, ${String(d.droppedFrames)} dropped`);
  });
  v.on('state', (s) => {
    status.textContent = s;
  });
  v.on('error', (e) => {
    status.textContent = `error: ${e.code}${e.message === undefined ? '' : ` (${e.message})`}`;
  });
  try {
    await v.start({ video });
  } catch {
    startButton.disabled = false;
    return;
  }
  header = headerLine({
    subject: subject.value.trim() || 'P?',
    condition: condition.value,
    options,
    camera: cameraOf(video),
    startedAt: new Date().toISOString(),
  });
  startedAt = performance.now();
  stopButton.disabled = false;
  watch.focus();
}

function stop(): void {
  vitals?.stop();
  vitals = null;
  clearReading();
  overlay.getContext('2d')?.clearRect(0, 0, overlay.width, overlay.height);
  stopButton.disabled = true;
  startButton.disabled = false;
  downloadButton.disabled = header === null;
}

function download(): void {
  if (header === null) return;
  const text = [header, ...lines].join('\n') + '\n';
  const url = URL.createObjectURL(new Blob([text], { type: 'application/x-ndjson' }));
  const a = document.createElement('a');
  const stamp = new Date().toISOString().slice(0, 19).replaceAll(':', '-');
  a.href = url;
  a.download = `lab-${subject.value.trim() || 'P'}-${condition.value.replaceAll(' ', '-')}-${stamp}.jsonl`;
  a.click();
  setTimeout(() => {
    URL.revokeObjectURL(url);
  }, 1000);
}

// The reading is timed when typing starts: the watch shows it at that moment, not when Enter is pressed.
watch.addEventListener('input', () => {
  if (watch.value.trim() === '') readingStartedAt = null;
  else readingStartedAt ??= performance.now();
});

el('reference', HTMLFormElement).addEventListener('submit', (event) => {
  event.preventDefault();
  const bpm = parseHeartRate(watch.value);
  if (vitals === null || header === null) {
    clearReading();
    return;
  }
  if (bpm === null) {
    watch.select();
    return;
  }
  const tMs = readingStartedAt ?? performance.now();
  lines.push(referenceLine(tMs, bpm));
  const item = document.createElement('li');
  item.textContent = `${bpm.toFixed(0)} bpm at ${((tMs - startedAt) / 1000).toFixed(0)} s`;
  references.prepend(item);
  watch.value = '';
  readingStartedAt = null;
});

startButton.addEventListener('click', () => {
  void start();
});
stopButton.addEventListener('click', stop);
downloadButton.addEventListener('click', download);
