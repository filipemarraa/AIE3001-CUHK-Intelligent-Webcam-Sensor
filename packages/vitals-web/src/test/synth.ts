import { SAMPLE } from '../engine/layout';

const CHANNELS = [
  { skin: 180, bloodVolume: 0.33 },
  { skin: 140, bloodVolume: 0.77 },
  { skin: 120, bloodVolume: 0.53 },
] as const;

export interface SynthOptions {
  bpm: number;
  rpm?: number;
  durationS: number;
  fps?: number;
  seed?: number;
}

// Mulberry32 plus Box-Muller, so runs are reproducible.
function gaussianNoise(seed: number): () => number {
  let a = seed >>> 0;
  const uniform = (): number => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
  return () => Math.sqrt(-2 * Math.log(1 - uniform())) * Math.cos(2 * Math.PI * uniform());
}

// Colour model of engine/rppg/synth_test.go and shoulder model of engine/resp/resp_test.go, one array per sample.
export function synthSamples({ bpm, rpm = 15, durationS, fps = 30, seed = 1 }: SynthOptions): Float64Array[] {
  const noise = gaussianNoise(seed);
  const f = bpm / 60;
  const samples: Float64Array[] = [];
  for (let i = 0; i < Math.floor(durationS * fps); i++) {
    const t = i / fps;
    const pulse = Math.sin(2 * Math.PI * f * t) + 0.4 * Math.sin(4 * Math.PI * f * t + 0.5);
    const light = 1 + 0.02 * Math.sin(2 * Math.PI * 0.05 * t);
    const x = new Float64Array(SAMPLE.length);
    x[SAMPLE.timeMs] = t * 1000;
    CHANNELS.forEach(({ skin, bloodVolume }, k) => {
      x[SAMPLE.r + k] = skin * light * (1 + 0.008 * bloodVolume * pulse) + 0.1 * noise();
    });
    x[SAMPLE.face] = 1;
    x[SAMPLE.motion] = 0.01;
    x[SAMPLE.light] = 130;
    x[SAMPLE.mouth] = 0.02;
    x[SAMPLE.shoulderY] = 0.6 - 0.004 * Math.sin(2 * Math.PI * (rpm / 60) * t);
    x[SAMPLE.shoulderW] = 0.3;
    samples.push(x);
  }
  return samples;
}
