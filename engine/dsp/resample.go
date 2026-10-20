package dsp

import (
	"errors"
	"math"
)

var (
	ErrTooShort       = errors.New("dsp: not enough samples")
	ErrGap            = errors.New("dsp: gap between samples above limit")
	ErrNotIncreasing  = errors.New("dsp: timestamps must be strictly increasing")
	ErrLengthMismatch = errors.New("dsp: channel length differs from timestamps")
)

func Resample(tsMs []float64, channels [][]float64, fs, maxGapMs float64) ([][]float64, error) {
	if len(tsMs) < 2 {
		return nil, ErrTooShort
	}
	for _, ch := range channels {
		if len(ch) != len(tsMs) {
			return nil, ErrLengthMismatch
		}
	}
	for i := 1; i < len(tsMs); i++ {
		dt := tsMs[i] - tsMs[i-1]
		if dt <= 0 {
			return nil, ErrNotIncreasing
		}
		if dt > maxGapMs {
			return nil, ErrGap
		}
	}

	stepMs := 1000 / fs
	n := int(math.Floor((tsMs[len(tsMs)-1]-tsMs[0])/stepMs+1e-9)) + 1
	out := make([][]float64, len(channels))
	for c := range out {
		out[c] = make([]float64, n)
	}

	j := 0
	for k := 0; k < n; k++ {
		t := tsMs[0] + float64(k)*stepMs
		for j < len(tsMs)-2 && tsMs[j+1] < t {
			j++
		}
		w := (t - tsMs[j]) / (tsMs[j+1] - tsMs[j])
		for c, ch := range channels {
			out[c][k] = ch[j] + w*(ch[j+1]-ch[j])
		}
	}
	return out, nil
}
