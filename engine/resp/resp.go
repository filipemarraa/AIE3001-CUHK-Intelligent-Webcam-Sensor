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
	PhaseWindowS   float64
	EnterThreshold float64
	ExitThreshold  float64
	MinPeriodicity float64
	MinSNRdB       float64
}

func DefaultConfig() Config {
	return Config{
		SampleRateHz:   10,
		MaxGapMs:       250,
		LowHz:          0.08,
		HighHz:         0.5,
		MinDurationS:   25,
		PhaseWindowS:   0.5,
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

	// The band-passed tail rings at the window edge, so the slope comes from the lightly smoothed signal instead.
	d := tailSlope(detrended, cfg) / stddev(filtered)
	return Breath{RPM: peak.FreqHz * 60, SNRdB: peak.SNRdB, Periodicity: periodicity, Phase: nextPhase(d, prev, cfg)}, nil
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

// tailSlope is the least-squares slope, per second, over the last PhaseWindowS of x after a moving average of the
// same length. The average is centred, and shrinks towards the end of the window instead of reaching past it.
func tailSlope(x []float64, cfg Config) float64 {
	m := max(2, int(math.Round(cfg.PhaseWindowS*cfg.SampleRateHz)))
	m = min(m, len(x))
	half := m / 2
	smooth := make([]float64, m)
	for k := range smooth {
		i := len(x) - m + k
		lo, hi := max(0, i-half), min(len(x)-1, i+half)
		var sum float64
		for j := lo; j <= hi; j++ {
			sum += x[j]
		}
		smooth[k] = sum / float64(hi-lo+1)
	}
	var sumT, sumY, sumTT, sumTY float64
	for k, y := range smooth {
		t := float64(k) / cfg.SampleRateHz
		sumT += t
		sumY += y
		sumTT += t * t
		sumTY += t * y
	}
	n := float64(m)
	den := n*sumTT - sumT*sumT
	if den == 0 {
		return 0
	}
	return (n*sumTY - sumT*sumY) / den
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
