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
// A flat recent slope, relative to that sinusoid, means a pause or a turning point.
func phase(x []float64, freqHz float64, prev Phase, cfg Config) Phase {
	omega := 2 * math.Pi * freqHz
	d, amp := sinusoidAtEnd(x, freqHz, cfg.SampleRateHz)
	// Past a quarter period the slope window averages rising and falling parts and fakes a pause.
	window := min(cfg.HoldWindowS, 0.25/freqHz)
	if amp == 0 || math.Abs(linearSlope(x, int(math.Round(window*cfg.SampleRateHz)), cfg.SampleRateHz)) < cfg.HoldRatio*amp*omega {
		return PhaseHold
	}
	return nextPhase(d, prev, cfg)
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

// sinusoidAtEnd fits a·cos(ωt) + b·sin(ωt) + c over the last period of x and returns the slope of the sinusoid at
// the last sample divided by its peak slope (so in [-1, 1], positive while rising) and its amplitude.
func sinusoidAtEnd(x []float64, freqHz, fs float64) (d, amp float64) {
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
		return 0, 0
	}
	a, b := coef[0], coef[1]
	amp = math.Hypot(a, b)
	if amp == 0 {
		return 0, 0
	}
	end := omega * float64(n-1)
	return (-a*math.Sin(end) + b*math.Cos(end)) / amp, amp
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

// linearSlope is the least-squares slope, per second, over the last n samples of x.
func linearSlope(x []float64, n int, fs float64) float64 {
	n = min(max(n, 2), len(x))
	seg := x[len(x)-n:]
	var sumT, sumY, sumTT, sumTY float64
	for k, y := range seg {
		t := float64(k) / fs
		sumT += t
		sumY += y
		sumTT += t * t
		sumTY += t * y
	}
	den := float64(n)*sumTT - sumT*sumT
	if den == 0 {
		return 0
	}
	return (float64(n)*sumTY - sumT*sumY) / den
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
