import type { Decoded } from './decode';
import { decodeEstimate } from './decode';
import { OUT, SAMPLE } from './layout';
import { instantiateStreaming } from '../worker/assets';

export interface Engine {
  push(sample: Float64Array): void;
  estimate(nowMs: number): Decoded;
  reset(): void;
}

export type Instantiate = (
  wasmUrl: string,
  imports: WebAssembly.Imports,
) => Promise<WebAssembly.WebAssemblyInstantiatedSource>;

interface GoRuntime {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

interface RawEngine {
  push(bytes: Uint8Array): void;
  estimate(nowMs: number, out: Uint8Array): number;
  reset(): void;
}

type GlobalWithGo = typeof globalThis & { Go?: new () => GoRuntime; vitalsEngine?: RawEngine };

export async function loadEngine(
  wasmUrl: string,
  wasmExecUrl: string,
  instantiate?: Instantiate,
): Promise<Engine> {
  const scope = globalThis as GlobalWithGo;
  // wasm_exec.js is a classic script that sets globalThis.Go; a module worker cannot importScripts it, so import it.
  if (scope.Go === undefined) await import(/* @vite-ignore */ wasmExecUrl);
  if (scope.Go === undefined) throw new Error('wasm_exec.js did not define Go');
  const go = new scope.Go();
  const load = instantiate ?? instantiateStreaming;
  const { instance } = await load(wasmUrl, go.importObject);
  // run() resolves only when the Go program exits; main registers vitalsEngine and then blocks.
  void go.run(instance);
  const raw = scope.vitalsEngine;
  if (raw === undefined) throw new Error('engine.wasm did not register vitalsEngine');

  const sample = new Float64Array(SAMPLE.length);
  const sampleBytes = new Uint8Array(sample.buffer, sample.byteOffset, sample.byteLength);
  const out = new Float64Array(OUT.maxLength);
  const outBytes = new Uint8Array(out.buffer, out.byteOffset, out.byteLength);

  return {
    push(x) {
      if (x.length !== SAMPLE.length) return;
      sample.set(x);
      raw.push(sampleBytes);
    },
    estimate(nowMs) {
      out.fill(Number.NaN);
      const n = raw.estimate(nowMs, outBytes);
      return decodeEstimate(out.subarray(0, n));
    },
    reset() {
      raw.reset();
    },
  };
}
