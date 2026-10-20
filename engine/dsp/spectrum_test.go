package dsp

import (
	"errors"
	"math"
	"math/cmplx"
	"math/rand/v2"
	"testing"
)

func naiveDFT(x []complex128) []complex128 {
	n := len(x)
	out := make([]complex128, n)
	for k := range out {
		for j, v := range x {
			out[k] += v * cmplx.Rect(1, -2*math.Pi*float64(k*j)/float64(n))
		}
	}
	return out
}

func TestFFTMatchesNaiveDFT(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for _, n := range []int{1, 2, 8, 64} {
		x := make([]complex128, n)
		for i := range x {
			x[i] = complex(rng.NormFloat64(), rng.NormFloat64())
		}
		want := naiveDFT(x)
		got := append([]complex128(nil), x...)
		FFT(got)
		for k := range want {
			if cmplx.Abs(got[k]-want[k]) > 1e-9 {
				t.Fatalf("n=%d k=%d got %v, want %v", n, k, got[k], want[k])
			}
		}
	}
}

func TestFFTPanicsOnNonPowerOfTwo(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for length 6")
		}
	}()
	FFT(make([]complex128, 6))
}

func TestDominantFrequencyFindsTone(t *testing.T) {
	const fs = 30.0
	for _, hz := range []float64{0.83, 1.2, 1.87, 2.5} {
		x := sine(450, fs, hz, 1)
		p, err := DominantFrequency(x, fs, 0.7, 3.0)
		if err != nil {
			t.Fatalf("hz=%v: unexpected error: %v", hz, err)
		}
		if math.Abs(p.FreqHz-hz) > 0.01 {
			t.Errorf("hz=%v: got %v", hz, p.FreqHz)
		}
	}
}

func TestDominantFrequencyIgnoresOutOfBandPeak(t *testing.T) {
	const fs = 30.0
	strong := sine(450, fs, 0.25, 10)
	weak := sine(450, fs, 1.5, 1)
	for i := range strong {
		strong[i] += weak[i]
	}
	p, err := DominantFrequency(strong, fs, 0.7, 3.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(p.FreqHz-1.5) > 0.02 {
		t.Fatalf("got %v, want 1.5", p.FreqHz)
	}
}

func TestSNRDropsWithNoise(t *testing.T) {
	const fs = 30.0
	rng := rand.New(rand.NewPCG(3, 4))
	clean := sine(450, fs, 1.2, 1)
	noisy := make([]float64, len(clean))
	for i := range clean {
		noisy[i] = clean[i] + 2*rng.NormFloat64()
	}

	pc, err := DominantFrequency(clean, fs, 0.7, 3.0)
	if err != nil {
		t.Fatal(err)
	}
	pn, err := DominantFrequency(noisy, fs, 0.7, 3.0)
	if err != nil {
		t.Fatal(err)
	}
	if pc.SNRdB < 10 {
		t.Errorf("clean SNR = %.1f dB, want >= 10", pc.SNRdB)
	}
	if pn.SNRdB >= pc.SNRdB-5 {
		t.Errorf("noisy SNR = %.1f dB, want well below clean %.1f dB", pn.SNRdB, pc.SNRdB)
	}
}

func TestDominantFrequencyErrors(t *testing.T) {
	if _, err := DominantFrequency([]float64{1, 2}, 30, 0.7, 3.0); !errors.Is(err, ErrTooShort) {
		t.Errorf("short input: err = %v, want ErrTooShort", err)
	}
	if _, err := DominantFrequency(make([]float64, 450), 30, 0.7, 3.0); !errors.Is(err, ErrFlat) {
		t.Errorf("flat input: err = %v, want ErrFlat", err)
	}
}
