package dsp

import (
	"math"
	"math/cmplx"
)

func Detrend(x []float64) []float64 {
	n := float64(len(x))
	var sumI, sumX, sumII, sumIX float64
	for i, v := range x {
		fi := float64(i)
		sumI += fi
		sumX += v
		sumII += fi * fi
		sumIX += fi * v
	}
	den := n*sumII - sumI*sumI
	slope := 0.0
	if den != 0 {
		slope = (n*sumIX - sumI*sumX) / den
	}
	intercept := (sumX - slope*sumI) / n

	y := make([]float64, len(x))
	for i, v := range x {
		y[i] = v - (intercept + slope*float64(i))
	}
	return y
}

type biquad struct {
	b0, b1, b2, a1, a2 float64
}

func butterworthBiquad(fs, fc float64, highpass bool) biquad {
	w0 := 2 * math.Pi * fc / fs
	cosW, alpha := math.Cos(w0), math.Sin(w0)/math.Sqrt2
	a0 := 1 + alpha
	b0, b1 := (1-cosW)/2, 1-cosW
	if highpass {
		b0, b1 = (1+cosW)/2, -(1 + cosW)
	}
	return biquad{b0 / a0, b1 / a0, b0 / a0, -2 * cosW / a0, (1 - alpha) / a0}
}

func (q biquad) apply(x []float64) []float64 {
	y := make([]float64, len(x))
	var x1, x2, y1, y2 float64
	for i, v := range x {
		out := q.b0*v + q.b1*x1 + q.b2*x2 - q.a1*y1 - q.a2*y2
		x2, x1 = x1, v
		y2, y1 = y1, out
		y[i] = out
	}
	return y
}

// powerGain is the power gain of the biquad for a tone at hz.
func (q biquad) powerGain(hz, fs float64) float64 {
	z1, z2 := cmplx.Rect(1, -2*math.Pi*hz/fs), cmplx.Rect(1, -4*math.Pi*hz/fs)
	h := (complex(q.b0, 0) + complex(q.b1, 0)*z1 + complex(q.b2, 0)*z2) / (1 + complex(q.a1, 0)*z1 + complex(q.a2, 0)*z2)
	return real(h)*real(h) + imag(h)*imag(h)
}

func bandpassStages(fs, lowHz, highHz float64) []biquad {
	return []biquad{butterworthBiquad(fs, lowHz, true), butterworthBiquad(fs, highHz, false)}
}

// BandpassPowerGain is the power that Bandpass passes of a tone at hz: each stage runs forward and backward.
func BandpassPowerGain(hz, fs, lowHz, highHz float64) float64 {
	g := 1.0
	for _, s := range bandpassStages(fs, lowHz, highHz) {
		g *= s.powerGain(hz, fs) * s.powerGain(hz, fs)
	}
	return g
}

func reverse(x []float64) []float64 {
	y := make([]float64, len(x))
	for i, v := range x {
		y[len(x)-1-i] = v
	}
	return y
}

func Bandpass(x []float64, fs, lowHz, highHz float64) []float64 {
	if len(x) < 2 {
		return append([]float64(nil), x...)
	}
	// Odd extension of three periods of the low cutoff lets the filter transient die out before the real samples.
	pad := min(len(x)-1, int(math.Ceil(3*fs/lowHz)))
	ext := make([]float64, 0, len(x)+2*pad)
	for i := pad; i >= 1; i-- {
		ext = append(ext, 2*x[0]-x[i])
	}
	ext = append(ext, x...)
	for i := 1; i <= pad; i++ {
		ext = append(ext, 2*x[len(x)-1]-x[len(x)-1-i])
	}

	stages := bandpassStages(fs, lowHz, highHz)
	for pass := 0; pass < 2; pass++ {
		for _, s := range stages {
			ext = s.apply(ext)
		}
		ext = reverse(ext)
	}
	return ext[pad : pad+len(x)]
}
