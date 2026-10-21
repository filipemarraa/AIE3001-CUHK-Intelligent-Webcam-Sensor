package resp

import (
	"errors"
	"math"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/dsp"
)

var (
	ErrTooShort    = errors.New("resp: window shorter than the minimum duration")
	ErrNoShoulders = errors.New("resp: shoulder width must be positive")
	ErrUnreliable  = errors.New("resp: shoulder signal is not periodic enough to be breathing")
)

type Phase int

const (
	PhaseUnknown Phase = iota
	PhaseIn
	PhaseOut
	PhaseHold
)

func (p Phase) String() string {
	switch p {
	case PhaseIn:
		return "in"
	case PhaseOut:
		return "out"
	case PhaseHold:
		return "hold"
	}
	return ""
}

type Config struct {
	SampleRateHz   float64
	MaxGapMs       float64
	LowHz          float64
	HighHz         float64
	MinDurationS   float64
	HoldWindowS    float64
	HoldRatio      float64
	EnterThreshold float64
	ExitThreshold  float64
	MinPeriodicity float64
	MinSNRdB       float64
}

func DefaultConfig() Config {
	return Config{
		SampleRateHz:   10,
		MaxGapMs:       500,
		LowHz:          0.08,
		HighHz:         0.5,
		MinDurationS:   25,
		HoldWindowS:    2,
		HoldRatio:      0.15,
		EnterThreshold: 0.2,
		ExitThreshold:  0.1,
		MinPeriodicity: 0.5,
		MinSNRdB:       3,
	}
}

type Breath struct {
	RPM         float64
	SNRdB       float64
	Periodicity float64
	Phase       Phase
}

func Estimate(tsMs, shoulderY, shoulderW []float64, prev Phase, cfg Config) (Breath, error) {
	if len(shoulderY) != len(tsMs) || len(shoulderW) != len(tsMs) {
		return Breath{}, dsp.ErrLengthMismatch
	}
	for _, w := range shoulderW {
		if !(w > 0) {
			return Breath{}, ErrNoShoulders
		}
	}
	if len(tsMs) < 2 || (tsMs[len(tsMs)-1]-tsMs[0])/1000 < cfg.MinDurationS {
		return Breath{}, ErrTooShort
	}

	// Image y grows downward, so inhaling lifts the shoulders and raises v.
	v := make([]float64, len(tsMs))
	for i := range v {
		v[i] = -shoulderY[i] / shoulderW[i]
	}
	grid, err := dsp.Resample(tsMs, [][]float64{v}, cfg.SampleRateHz, cfg.MaxGapMs)
	if err != nil {
		return Breath{}, err
	}
	detrended := dsp.Detrend(grid[0])
	filtered := dsp.Bandpass(detrended, cfg.SampleRateHz, cfg.LowHz, cfg.HighHz)

	peak, err := dsp.DominantFrequency(filtered, cfg.SampleRateHz, cfg.LowHz, cfg.HighHz)
	if err != nil {
		return Breath{}, err
	}
	// The SNR band is wide for slow breathing, so noise can still score well; repeating at its own period cannot.
	periodicity := autocorrelation(filtered, int(math.Round(cfg.SampleRateHz/peak.FreqHz)))
	if periodicity < cfg.MinPeriodicity || peak.SNRdB < cfg.MinSNRdB {
		return Breath{}, ErrUnreliable
	}

	return Breath{
		RPM: peak.FreqHz * 60, SNRdB: peak.SNRdB, Periodicity: periodicity,
		Phase: phase(detrended, peak.FreqHz, prev, cfg),
	}, nil
}

// phase reads the direction from a sinusoid fitted at the breathing frequency over the last cycle: a matched filter
// that rejects faster motion such as the heartbeat and does not ring at the window edge like a band-pass filter.
// Turning points come out as hold through the hysteresis on that direction. A real pause is a recent window that a
// flat line explains much better than the sinusoid does; a turning point while breathing goes on is still explained by
// the sinusoid, whatever the mean slope of the window.
func phase(x []float64, freqHz float64, prev Phase, cfg Config) Phase {
	fit, ok := fitSinusoid(x, freqHz, cfg.SampleRateHz)
	if !ok {
		return PhaseHold
	}
	// A 1 s moving average nulls 1 Hz and damps the rest of the heartbeat band, which keeps moving the shoulders
	// during a pause; the same average is applied to the sinusoid so the comparison stays fair.
	smooth := max(1, int(math.Round(cfg.SampleRateHz)))
	m := min(len(x)-smooth+1, max(3, int(math.Round(min(cfg.HoldWindowS, 0.25/freqHz)*cfg.SampleRateHz))))
	if m >= 3 && paused(movingAverage(x[len(x)-m-smooth+1:], smooth), movingAverage(fit.tail(m+smooth-1), smooth), fit.amp, cfg) {
		return PhaseHold
	}
	return nextPhase(fit.direction(), prev, cfg)
}

// movingAverage returns the trailing means of n consecutive values: len(x)-n+1 of them.
func movingAverage(x []float64, n int) []float64 {
	out := make([]float64, 0, len(x)-n+1)
	var sum float64
	for i, v := range x {
		sum += v
		if i >= n {
			sum -= x[i-n]
		}
		if i >= n-1 {
			out = append(out, sum/float64(n))
		}
	}
	return out
}

func nextPhase(d float64, prev Phase, cfg Config) Phase {
	switch {
	case math.Abs(d) < cfg.ExitThreshold:
		return PhaseHold
	case d > cfg.EnterThreshold:
		return PhaseIn
	case d < -cfg.EnterThreshold:
		return PhaseOut
	case prev == PhaseUnknown:
		return PhaseHold
	}
	return prev
}

// paused compares how well a flat line and the fitted sinusoid (with its own offset) explain the recent samples.
func paused(observed, model []float64, amp float64, cfg Config) bool {
	if stddev(model) < cfg.HoldRatio*0.1*amp {
		return false
	}
	var offset float64
	for i := range observed {
		offset += observed[i] - model[i]
	}
	offset /= float64(len(observed))
	mean := 0.0
	for _, v := range observed {
		mean += v
	}
	mean /= float64(len(observed))
	var errFlat, errModel float64
	for i, v := range observed {
		errFlat += (v - mean) * (v - mean)
		r := v - model[i] - offset
		errModel += r * r
	}
	return errModel > 2*errFlat
}

// sinusoid is a·cos(ωk) + b·sin(ωk) over sample index k, fitted to the last n samples of a signal.
type sinusoid struct {
	a, b, omega, amp float64
	n                int
}

func (s sinusoid) at(k int) float64 {
	return s.a*math.Cos(s.omega*float64(k)) + s.b*math.Sin(s.omega*float64(k))
}

// direction is the slope at the last sample over the peak slope: in [-1, 1], positive while rising.
func (s sinusoid) direction() float64 {
	end := s.omega * float64(s.n-1)
	return (-s.a*math.Sin(end) + s.b*math.Cos(end)) / s.amp
}

// tail returns the last m values of the fitted sinusoid.
func (s sinusoid) tail(m int) []float64 {
	out := make([]float64, m)
	for i := range out {
		out[i] = s.at(s.n - m + i)
	}
	return out
}

// fitSinusoid fits a·cos(ωt) + b·sin(ωt) + c over the last period of x by least squares.
func fitSinusoid(x []float64, freqHz, fs float64) (sinusoid, bool) {
	n := min(len(x), max(4, int(math.Round(fs/freqHz))))
	seg := x[len(x)-n:]
	omega := 2 * math.Pi * freqHz / fs
	var m [3][4]float64
	for k, v := range seg {
		basis := [3]float64{math.Cos(omega * float64(k)), math.Sin(omega * float64(k)), 1}
		for i := range 3 {
			for j := range 3 {
				m[i][j] += basis[i] * basis[j]
			}
			m[i][3] += basis[i] * v
		}
	}
	coef, ok := solve3(m)
	if !ok {
		return sinusoid{}, false
	}
	s := sinusoid{a: coef[0], b: coef[1], omega: omega, amp: math.Hypot(coef[0], coef[1]), n: n}
	return s, s.amp > 0
}

// solve3 solves a 3x3 linear system given as an augmented matrix, by Gaussian elimination with partial pivoting.
func solve3(m [3][4]float64) ([3]float64, bool) {
	for col := range 3 {
		pivot := col
		for r := col + 1; r < 3; r++ {
			if math.Abs(m[r][col]) > math.Abs(m[pivot][col]) {
				pivot = r
			}
		}
		if math.Abs(m[pivot][col]) < 1e-12 {
			return [3]float64{}, false
		}
		m[col], m[pivot] = m[pivot], m[col]
		for r := col + 1; r < 3; r++ {
			f := m[r][col] / m[col][col]
			for c := col; c < 4; c++ {
				m[r][c] -= f * m[col][c]
			}
		}
	}
	var out [3]float64
	for r := 2; r >= 0; r-- {
		v := m[r][3]
		for c := r + 1; c < 3; c++ {
			v -= m[r][c] * out[c]
		}
		out[r] = v / m[r][r]
	}
	return out, true
}

// autocorrelation is the normalised correlation of x with itself shifted by lag samples, in [-1, 1].
func autocorrelation(x []float64, lag int) float64 {
	if lag <= 0 || lag >= len(x) {
		return 0
	}
	var xy, xx, yy float64
	for i := 0; i+lag < len(x); i++ {
		xy += x[i] * x[i+lag]
		xx += x[i] * x[i]
		yy += x[i+lag] * x[i+lag]
	}
	if xx == 0 || yy == 0 {
		return 0
	}
	return xy / math.Sqrt(xx*yy)
}

func stddev(x []float64) float64 {
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
