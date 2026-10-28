package dsp

import (
	"math"
	"testing"
)

func sine(n int, fs, hz, amp float64) []float64 {
	x := make([]float64, n)
	for i := range x {
		x[i] = amp * math.Sin(2*math.Pi*hz*float64(i)/fs)
	}
	return x
}

func rms(x []float64) float64 {
	var s float64
	for _, v := range x {
		s += v * v
	}
	return math.Sqrt(s / float64(len(x)))
}

func TestDetrendRemovesLinearTrend(t *testing.T) {
	x := make([]float64, 100)
	for i := range x {
		x[i] = 3*float64(i) + 7
	}
	for i, v := range Detrend(x) {
		if math.Abs(v) > 1e-9 {
			t.Fatalf("Detrend(x)[%d] = %v, want 0", i, v)
		}
	}
}

func TestDetrendKeepsOscillation(t *testing.T) {
	osc := sine(900, 30, 1.2, 1)
	x := make([]float64, len(osc))
	for i := range x {
		x[i] = osc[i] + 0.5*float64(i) + 100
	}
	y := Detrend(x)
	var maxErr float64
	for i := range y {
		maxErr = math.Max(maxErr, math.Abs(y[i]-osc[i]))
	}
	if maxErr > 0.05 {
		t.Fatalf("max deviation from oscillation = %v, want <= 0.05", maxErr)
	}
}

func TestBandpassGain(t *testing.T) {
	const fs, n = 30.0, 900
	tests := []struct {
		name     string
		hz       float64
		min, max float64
	}{
		{"respiration drift is removed", 0.1, 0, 0.05},
		{"72 bpm passes", 1.2, 0.8, 1.05},
		{"120 bpm passes", 2.0, 0.8, 1.05},
		{"high frequency noise is removed", 8.0, 0, 0.05},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			y := Bandpass(sine(n, fs, tc.hz, 1), fs, 0.7, 3.0)
			gain := rms(y[n/4:3*n/4]) / (1 / math.Sqrt2)
			if gain < tc.min || gain > tc.max {
				t.Fatalf("gain = %.3f, want in [%v, %v]", gain, tc.min, tc.max)
			}
		})
	}
}

func TestBandpassIsZeroPhase(t *testing.T) {
	const fs, n = 30.0, 900
	x := sine(n, fs, 1.2, 1)
	y := Bandpass(x, fs, 0.7, 3.0)
	var dot, xx, yy float64
	for i := n / 4; i < 3*n/4; i++ {
		dot += x[i] * y[i]
		xx += x[i] * x[i]
		yy += y[i] * y[i]
	}
	if corr := dot / math.Sqrt(xx*yy); corr < 0.999 {
		t.Fatalf("correlation with input = %v, want >= 0.999", corr)
	}
}

func TestBandpassPowerGainMatchesTheFilter(t *testing.T) {
	const fs = 30.0
	for _, hz := range []float64{0.75, 0.92, 1.5, 2.5, 2.9} {
		x := sine(1800, fs, hz, 1)
		y := Bandpass(x, fs, 0.7, 3.0)
		measured := math.Pow(rms(y[300:1500])/rms(x[300:1500]), 2)
		if got := BandpassPowerGain(hz, fs, 0.7, 3.0); math.Abs(got-measured) > 0.02 {
			t.Errorf("%v Hz: gain %.3f, filter passes %.3f", hz, got, measured)
		}
	}
}
