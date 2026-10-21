// Writes apps/demo/e2e/fake.y4m: 10 s, 640x480, 30 fps, plain mid-grey with a slow brightness ramp (no face, no person).
// Deterministic and dependency-free: the YUV4MPEG2 header and the 4:2:0 frames are written directly.
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

export const WIDTH = 640;
export const HEIGHT = 480;
export const FPS = 30;
export const SECONDS = 10;
const GREY_START = 110;
const GREY_END = 140;

export function buildY4m() {
  const frames = FPS * SECONDS;
  const luma = WIDTH * HEIGHT;
  const chroma = (WIDTH / 2) * (HEIGHT / 2);
  const header = Buffer.from(`YUV4MPEG2 W${WIDTH} H${HEIGHT} F${FPS}:1 Ip A1:1 C420jpeg\n`);
  const frameHeader = Buffer.from('FRAME\n');
  const chunks = [header];
  for (let i = 0; i < frames; i += 1) {
    const frame = Buffer.alloc(luma + 2 * chroma, 128);
    frame.fill(Math.round(GREY_START + ((GREY_END - GREY_START) * i) / (frames - 1)), 0, luma);
    chunks.push(frameHeader, frame);
  }
  return Buffer.concat(chunks);
}

if (process.argv[1] !== undefined && import.meta.url === new URL(`file://${process.argv[1]}`).href) {
  const out = fileURLToPath(new URL('../apps/demo/e2e/fake.y4m', import.meta.url));
  mkdirSync(dirname(out), { recursive: true });
  writeFileSync(out, buildY4m());
  console.log(`wrote ${out}`);
}
