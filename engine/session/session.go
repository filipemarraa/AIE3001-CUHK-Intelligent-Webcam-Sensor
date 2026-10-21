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
	maxFps      = 60
	mouthWindow = 2000
)

type Estimate struct {
	TimeMs       float64
	HRBpm, RRRpm float64
	Phase        resp.Phase
	Quality      float64
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
	cfg       Config
	buf       []Sample
	head, n   int
	lastMs    float64
	prevPhase resp.Phase

	ts, r, g, b, y, w, stat []float64
	idx                     []int
}

func New(cfg Config) *Session {
	capacity := max(2, int(math.Ceil(cfg.BufferS*maxFps)))
	s := &Session{cfg: cfg, buf: make([]Sample, capacity), idx: make([]int, 0, capacity)}
	for _, p := range []*[]float64{&s.ts, &s.r, &s.g, &s.b, &s.y, &s.w, &s.stat} {
		*p = make([]float64, 0, capacity)
	}
	s.Reset()
	return s
}

func (s *Session) Reset() {
	s.head, s.n = 0, 0
	s.lastMs = math.Inf(-1)
	s.prevPhase = resp.PhaseUnknown
}

// Push drops samples that are out of order or not finite, so the resampler never sees them.
func (s *Session) Push(x Sample) {
	if !x.finite() || x.TimeMs <= s.lastMs {
		return
	}
	s.lastMs = x.TimeMs
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
	e := Estimate{TimeMs: ref, HRBpm: math.NaN(), RRRpm: math.NaN(), Confidence: quality.ConfidenceInvalid}
	hrFrom := s.since(ref - s.cfg.HRWindowS*1000)
	if hrFrom == s.n {
		s.prevPhase = resp.PhaseUnknown
		e.Reason = quality.ReasonWarmingUp
		return e
	}

	in := s.qualityInputs(hrFrom, ref)
	hr, spanS, hrErr := s.heartRate(hrFrom, ref)
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
		return e
	}
	e.HRBpm = hr.BPM
	if breathErr == nil {
		e.RRRpm, e.Phase = breath.RPM, breath.Phase
	}
	return e
}

func (s *Session) heartRate(from int, ref float64) (rppg.HeartRate, float64, error) {
	idx := s.run(from, ref, s.cfg.RPPG.MaxGapMs, func(x *Sample) bool { return x.Face })
	s.ts, s.r, s.g, s.b = s.ts[:0], s.r[:0], s.g[:0], s.b[:0]
	for _, i := range idx {
		x := s.at(i)
		s.ts, s.r, s.g, s.b = append(s.ts, x.TimeMs), append(s.r, x.R), append(s.g, x.G), append(s.b, x.B)
	}
	if len(s.ts) < 2 {
		return rppg.HeartRate{}, 0, rppg.ErrTooShort
	}
	spanS := (s.ts[len(s.ts)-1] - s.ts[0]) / 1000
	hr, err := rppg.EstimateHeartRate(s.ts, s.r, s.g, s.b, s.cfg.RPPG)
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
	motion := s.stat[:0]
	prevFaceMs := math.Inf(-1)
	for i := from; i < s.n; i++ {
		x := s.at(i)
		if !x.Face {
			continue
		}
		faces++
		lightSum += x.Light
		// Motion is the landmark displacement since the previous face frame; dividing by the time makes it a speed.
		if dt := (x.TimeMs - prevFaceMs) / 1000; dt > 0 && !math.IsInf(prevFaceMs, -1) {
			motion = append(motion, x.Motion/dt)
		}
		prevFaceMs = x.TimeMs
	}
	s.stat = motion
	in.FaceFraction = faces / float64(s.n-from)
	if faces > 0 {
		in.LightMean = lightSum / faces
	}
	if len(motion) > 0 {
		slices.Sort(motion)
		in.MotionP90 = motion[int(math.Ceil(0.9*float64(len(motion))))-1]
	}

	mouth := s.stat[:0]
	for i := s.since(ref - mouthWindow); i < s.n; i++ {
		if x := s.at(i); x.Face {
			mouth = append(mouth, x.Mouth)
		}
	}
	in.MouthStd = stddev(mouth)

	// Intervals across a gap are not jitter; the gap already restarts the warm-up.
	intervals := s.stat[:0]
	for i := from + 1; i < s.n; i++ {
		if dt := s.at(i).TimeMs - s.at(i-1).TimeMs; dt <= s.cfg.RPPG.MaxGapMs {
			intervals = append(intervals, dt)
		}
	}
	in.JitterMs = stddev(intervals)
	s.stat = intervals
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
