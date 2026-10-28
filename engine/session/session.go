package session

import (
	"math"
	"slices"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/quality"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/resp"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/rppg"
)

// MaxPulseLen is 5 s of pulse on the 30 Hz grid.
const MaxPulseLen = 150

const (
	maxFps       = 60
	mouthWindow  = 2000
	motionWindow = 2000
)

type Estimate struct {
	TimeMs       float64
	HRBpm, RRRpm float64
	Phase        resp.Phase
	Quality      float64
	SNRdB        float64
	Confidence   quality.Confidence
	Reason       quality.Reason
	Pulse        []float64
}

type Config struct {
	BufferS, WarmupS, HRWindowS float64
	RPPG                        rppg.Config
	Resp                        resp.Config
}

func DefaultConfig() Config {
	return Config{BufferS: 30, WarmupS: 10, HRWindowS: 15, RPPG: rppg.DefaultConfig(), Resp: resp.DefaultConfig()}
}

type Session struct {
	cfg        Config
	buf        []Sample
	head, n    int
	lastMs     float64
	lastFaceMs float64
	prevPhase  resp.Phase
	hr         *tracker

	ts, r, g, b, y, w, stat []float64
	idx                     []int
}

func New(cfg Config) *Session {
	capacity := max(2, int(math.Ceil(cfg.BufferS*maxFps)))
	s := &Session{cfg: cfg, buf: make([]Sample, capacity), idx: make([]int, 0, capacity), hr: newTracker()}
	for _, p := range []*[]float64{&s.ts, &s.r, &s.g, &s.b, &s.y, &s.w, &s.stat} {
		*p = make([]float64, 0, capacity)
	}
	s.Reset()
	return s
}

func (s *Session) Reset() {
	s.head, s.n = 0, 0
	s.lastMs = math.Inf(-1)
	s.lastFaceMs = math.Inf(-1)
	s.prevPhase = resp.PhaseUnknown
	s.hr.reset()
}

// Push drops samples that are out of order or not finite, so the resampler never sees them.
func (s *Session) Push(x Sample) {
	if !x.finite() || x.TimeMs <= s.lastMs {
		return
	}
	s.lastMs = x.TimeMs
	x.speed = 0
	if x.Face {
		// Motion is the landmark displacement since the previous face frame; over the elapsed time it is a speed.
		if !math.IsInf(s.lastFaceMs, -1) {
			x.speed = x.Motion / ((x.TimeMs - s.lastFaceMs) / 1000)
		}
		s.lastFaceMs = x.TimeMs
	}
	s.buf[s.head] = x
	s.head = (s.head + 1) % len(s.buf)
	s.n = min(s.n+1, len(s.buf))
}

// at returns the i-th oldest sample.
func (s *Session) at(i int) *Sample {
	return &s.buf[(s.head-s.n+i+len(s.buf))%len(s.buf)]
}

// since returns the index of the oldest sample at or after ms.
func (s *Session) since(ms float64) int {
	i := s.n
	for i > 0 && s.at(i-1).TimeMs >= ms {
		i--
	}
	return i
}

// run returns, oldest first, the indices of the trailing contiguous run of samples in [from, n) that pass keep: no two
// kept samples more than maxGapMs apart, and the last one within maxGapMs of ref.
func (s *Session) run(from int, ref, maxGapMs float64, keep func(*Sample) bool) []int {
	idx := s.idx[:0]
	next := ref
	for i := s.n - 1; i >= from; i-- {
		x := s.at(i)
		if !keep(x) {
			continue
		}
		if next-x.TimeMs > maxGapMs {
			break
		}
		idx = append(idx, i)
		next = x.TimeMs
	}
	slices.Reverse(idx)
	s.idx = idx
	return idx
}

func (s *Session) Estimate(nowMs float64) Estimate {
	ref := s.lastMs
	if isFinite(nowMs) && nowMs > ref {
		ref = nowMs
	}
	if !isFinite(ref) {
		ref = 0
	}
	e := Estimate{TimeMs: ref, HRBpm: math.NaN(), RRRpm: math.NaN(), SNRdB: math.NaN(), Confidence: quality.ConfidenceInvalid}
	hrFrom := s.since(ref - s.cfg.HRWindowS*1000)
	if hrFrom == s.n {
		s.prevPhase = resp.PhaseUnknown
		s.hr.reset()
		e.Reason = quality.ReasonWarmingUp
		return e
	}

	in := s.qualityInputs(hrFrom, ref)
	hr, spanS, hrErr := s.heartRate(hrFrom, ref, s.hr.prior(ref))
	// A window that yields no pulse after the warm-up is still settling, and the reason must say so.
	in.WarmingUp = spanS < s.cfg.WarmupS || hrErr != nil
	in.HasHR = hrErr == nil
	if in.HasHR && isFinite(hr.SNRdB) {
		in.SNRdB = hr.SNRdB
	}
	breath, breathErr := s.breathing(ref)

	res := quality.Assess(in)
	e.Quality, e.Confidence, e.Reason = res.Quality, res.Confidence, res.Reason
	if len(hr.Pulse) > 0 {
		e.Pulse = hr.Pulse[max(0, len(hr.Pulse)-MaxPulseLen):]
	}
	if e.Confidence == quality.ConfidenceInvalid {
		s.hr.reset()
		// The shoulders do not need the skin colour: a missing pulse or a dark room leaves breathing to its own test.
		if breathErr == nil && !quality.BreathingBlocked(in) {
			e.RRRpm, e.Phase = breath.RPM, breath.Phase
		}
		return e
	}
	// The quality keeps the SNR of this window; only the rate is tracked.
	e.HRBpm, e.SNRdB = s.hr.update(ref, hr.BPM, hr.SNRdB), hr.SNRdB
	if breathErr == nil {
		e.RRRpm, e.Phase = breath.RPM, breath.Phase
	}
	return e
}

// heartRate leaves moving frames out, so a burst of motion costs the burst and a fresh warm-up, not a whole window.
// priorBPM, the tracked rate or NaN, settles which harmonic of the pulse is the pulse.
func (s *Session) heartRate(from int, ref, priorBPM float64) (rppg.HeartRate, float64, error) {
	idx := s.run(from, ref, s.cfg.RPPG.MaxGapMs, func(x *Sample) bool { return x.Face && x.speed <= quality.MaxMotionPerS })
	s.ts, s.r, s.g, s.b = s.ts[:0], s.r[:0], s.g[:0], s.b[:0]
	for _, i := range idx {
		x := s.at(i)
		s.ts, s.r, s.g, s.b = append(s.ts, x.TimeMs), append(s.r, x.R), append(s.g, x.G), append(s.b, x.B)
	}
	if len(s.ts) < 2 {
		return rppg.HeartRate{}, 0, rppg.ErrTooShort
	}
	spanS := (s.ts[len(s.ts)-1] - s.ts[0]) / 1000
	hr, err := rppg.EstimateHeartRate(s.ts, s.r, s.g, s.b, priorBPM, s.cfg.RPPG)
	return hr, spanS, err
}

// breathing keeps the last known phase for hysteresis until the shoulder signal breaks.
func (s *Session) breathing(ref float64) (resp.Breath, error) {
	idx := s.run(s.since(ref-s.cfg.BufferS*1000), ref, s.cfg.Resp.MaxGapMs, func(x *Sample) bool { return x.ShoulderW > 0 })
	s.ts, s.y, s.w = s.ts[:0], s.y[:0], s.w[:0]
	for _, i := range idx {
		x := s.at(i)
		s.ts, s.y, s.w = append(s.ts, x.TimeMs), append(s.y, x.ShoulderY), append(s.w, x.ShoulderW)
	}
	if len(s.ts) < 2 || (s.ts[len(s.ts)-1]-s.ts[0])/1000 < s.cfg.Resp.MinDurationS {
		s.prevPhase = resp.PhaseUnknown
		return resp.Breath{}, resp.ErrTooShort
	}
	b, err := resp.Estimate(s.ts, s.y, s.w, s.prevPhase, s.cfg.Resp)
	if err == nil && b.Phase != resp.PhaseUnknown {
		s.prevPhase = b.Phase
	}
	return b, err
}

// qualityInputs measures the window [from, n). Every field stays finite, since quality.Assess does not handle NaN.
func (s *Session) qualityInputs(from int, ref float64) quality.Inputs {
	var in quality.Inputs
	var faces, lightSum float64
	for i := from; i < s.n; i++ {
		if x := s.at(i); x.Face {
			faces++
			lightSum += x.Light
		}
	}
	in.FaceFraction = faces / float64(s.n-from)
	if faces > 0 {
		in.LightMean = lightSum / faces
	}

	// Only recent motion counts: moving frames are already left out of the heart-rate window.
	motion := s.stat[:0]
	for i := s.since(ref - motionWindow); i < s.n; i++ {
		if x := s.at(i); x.Face {
			motion = append(motion, x.speed)
		}
	}
	if len(motion) > 0 {
		slices.Sort(motion)
		in.MotionP90 = motion[int(math.Ceil(0.9*float64(len(motion))))-1]
	}
	s.stat = motion

	mouth := s.stat[:0]
	for i := s.since(ref - mouthWindow); i < s.n; i++ {
		if x := s.at(i); x.Face {
			mouth = append(mouth, x.Mouth)
		}
	}
	in.MouthStd = stddev(mouth)

	// Intervals across a gap are not a timing fault of the window; the gap already restarts the warm-up.
	var count int
	var spanMs float64
	for i := from + 1; i < s.n; i++ {
		if dt := s.at(i).TimeMs - s.at(i-1).TimeMs; dt <= s.cfg.RPPG.MaxGapMs {
			count++
			spanMs += dt
			in.MaxGapMs = max(in.MaxGapMs, dt)
		}
	}
	if spanMs > 0 {
		in.SampleRateHz = float64(count) / (spanMs / 1000)
	}
	return in
}

func stddev(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	var mean float64
	for _, v := range x {
		mean += v
	}
	mean /= float64(len(x))
	var ss float64
	for _, v := range x {
		ss += (v - mean) * (v - mean)
	}
	return math.Sqrt(ss / float64(len(x)))
}
