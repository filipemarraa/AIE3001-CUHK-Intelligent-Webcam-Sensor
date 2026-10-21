export type Method = 'pos' | 'chrom';

export type Confidence = 'high' | 'medium' | 'low' | 'invalid';

export type Reason = 'no_face' | 'too_much_motion' | 'low_light' | 'talking' | 'warming_up';

export type BreathPhase = 'in' | 'out' | 'hold';

export type VitalsState = 'starting' | 'calibrating' | 'measuring' | 'paused' | 'stopped';

export type VitalsErrorCode =
  'camera_denied' | 'camera_unavailable' | 'wasm_failed' | 'vision_failed' | 'unsupported_browser';

export interface Estimate {
  tMs: number;
  hrBpm: number | null;
  rrRpm: number | null;
  breathPhase: BreathPhase | null;
  quality: number;
  confidence: Confidence;
  reason: Reason | null;
  method: Method;
}

export interface VitalsError {
  code: VitalsErrorCode;
  message?: string;
}

export interface ExerciseSummary {
  hrStart: number | null;
  hrEnd: number | null;
  rrMean: number | null;
  qualityMean: number | null;
  durationS: number;
  method: Method;
}

export interface VitalsOptions {
  assetsBaseUrl: string;
  method?: Method;
  // For validation recordings: each second, the numeric samples (layout SAMPLE) behind the estimate. Never pixels.
  onSamples?: (rows: number[][]) => void;
}

export interface Diagnostics {
  frameCostMs: number;
  droppedFrames: number;
  pulse: number[];
}

export interface VitalsEvents {
  estimate: Estimate;
  diagnostics: Diagnostics;
  state: VitalsState;
  error: VitalsError;
}

export type Unsubscribe = () => void;

export interface Vitals {
  start(input: { video: HTMLVideoElement }): Promise<void>;
  stop(): void;
  on<K extends keyof VitalsEvents>(event: K, handler: (payload: VitalsEvents[K]) => void): Unsubscribe;
  summary(): ExerciseSummary | null;
}
