export type Method = 'pos' | 'chrom';

export type Confidence = 'high' | 'medium' | 'low' | 'invalid';

export type Reason = 'no_face' | 'too_much_motion' | 'low_light' | 'talking' | 'warming_up' | 'weak_pulse';

export type BreathPhase = 'in' | 'out' | 'hold';

export type SkinRegion = 'forehead' | 'glabella' | 'malarRight' | 'malarLeft';

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

// Capture improvements under evaluation; each stays off until it beats the current pipeline on recorded sessions.
export interface CaptureOptions {
  // Find face landmarks every N frames and read the skin colour on every frame.
  landmarkEvery?: number;
  // Leave out saturated pixels and pixels far from the median brightness of each skin region.
  robustPixels?: boolean;
  // Lock exposure and white balance once the camera has settled, where the camera allows it.
  lockExposure?: boolean;
  // Where the skin colour is read: three boxes (forehead and cheeks) or polygons of the forehead, glabella and cheeks
  // that follow the head, leaving out a cheek turned away. Default 'boxes'.
  skin?: 'boxes' | 'anatomical';
  // Lowest score for the face detector to report a face; the detector's default is 0.5.
  minFaceConfidence?: number;
}

// The skin regions of one frame, in pixels of the frame the worker analyses (width × height), for drawing an overlay.
export interface RoiOverlay {
  width: number;
  height: number;
  boxes: { x: number; y: number; w: number; h: number }[];
  polygons: { region: SkinRegion; points: { x: number; y: number }[] }[];
}

export interface VitalsOptions {
  assetsBaseUrl: string;
  method?: Method;
  capture?: CaptureOptions;
  // For validation recordings: each second, the numeric samples (layout SAMPLE) behind the estimate. Never pixels.
  // skins, row by row: the mean R, G, B of the boxes, then of the anatomical regions (0 without a face).
  onSamples?: (rows: number[][], skins?: number[][]) => void;
  // For a laboratory overlay: the skin regions of every analysed frame. Only coordinates, never pixels.
  onRoi?: (roi: RoiOverlay) => void;
}

export interface Diagnostics {
  frameCostMs: number;
  droppedFrames: number;
  pulse: number[];
  exposureLocked?: boolean;
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
