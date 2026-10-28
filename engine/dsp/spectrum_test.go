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
	for _, hz := range []float64{0.75, 0.83, 1.2, 1.87, 2.5, 2.95} {
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

func TestDominantFrequencyRejectsPeakBeyondBandEdge(t *testing.T) {
	const fs = 30.0
	for _, hz := range []float64{0.6, 3.05} {
		_, err := DominantFrequency(sine(450, fs, hz, 1), fs, 0.7, 3.0)
		if !errors.Is(err, ErrNoPeakInBand) {
			t.Errorf("hz=%v: err = %v, want ErrNoPeakInBand", hz, err)
		}
	}
}

func TestDominantFrequencyRejectsNonFiniteInput(t *testing.T) {
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		x := sine(450, 30, 1.2, 1)
		x[200] = bad
		if _, err := DominantFrequency(x, 30, 0.7, 3.0); !errors.Is(err, ErrNotFinite) {
			t.Errorf("value %v: err = %v, want ErrNotFinite", bad, err)
		}
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

func tones(n int, fs float64, hz, amp []float64) []float64 {
	x := make([]float64, n)
	for i, f := range hz {
		s := sine(n, fs, f, amp[i])
		for j := range x {
			x[j] += s[j]
		}
	}
	return x
}

func TestHeartFrequencyPrefersTheFundamentalOverAStrongerHarmonic(t *testing.T) {
	x := Bandpass(tones(450, 30, []float64{0.75, 1.5}, []float64{0.8, 1}), 30, 0.7, 3.0)
	if p, _ := DominantFrequency(x, 30, 0.7, 3.0); math.Abs(p.FreqHz-1.5) > 0.01 {
		t.Fatalf("raw peak %v, want the harmonic at 1.5 for this test to mean anything", p.FreqHz)
	}
	p, err := HeartFrequency(x, 30, 0.7, 3.0, math.NaN())
	if err != nil || math.Abs(p.FreqHz-0.75) > 0.01 {
		t.Fatalf("got %v %v, want 0.75", p.FreqHz, err)
	}
}

// Below HalfOverrideRatio the prior decides: here the pulse has 0.56 of the harmonic's power before the band-pass.
func TestHeartFrequencyKeepsTheHarmonicNearThePrior(t *testing.T) {
	x := Bandpass(tones(450, 30, []float64{0.75, 1.5}, []float64{0.75, 1}), 30, 0.7, 3.0)
	if p, _ := HeartFrequency(x, 30, 0.7, 3.0, math.NaN()); math.Abs(p.FreqHz-0.75) > 0.01 {
		t.Fatalf("no prior: got %v, want 0.75", p.FreqHz)
	}
	p, err := HeartFrequency(x, 30, 0.7, 3.0, 1.45)
	if err != nil || math.Abs(p.FreqHz-1.5) > 0.01 {
		t.Fatalf("got %v %v, want 1.5 next to the previous 1.45", p.FreqHz, err)
	}
}

func TestHeartFrequencyIgnoresAWeakHalfFrequency(t *testing.T) {
	x := tones(450, 30, []float64{1.25, 2.5}, []float64{0.6, 1})
	p, err := HeartFrequency(x, 30, 0.7, 3.0, math.NaN())
	if err != nil || math.Abs(p.FreqHz-2.5) > 0.01 {
		t.Fatalf("got %v %v, want 2.5: the half frequency has %.0f%% of its power", p.FreqHz, err, 100*0.36)
	}
}

func TestHeartFrequencyNeverTakesTheDouble(t *testing.T) {
	x := tones(450, 30, []float64{0.8, 1.6}, []float64{1, 0.9})
	for _, prior := range []float64{math.NaN(), 1.55} {
		if p, _ := HeartFrequency(x, 30, 0.7, 3.0, prior); math.Abs(p.FreqHz-0.8) > 0.01 {
			t.Errorf("prior %v: got %v, want the strongest peak 0.8", prior, p.FreqHz)
		}
	}
}

func TestHeartFrequencyStrongHalfOverridesThePrior(t *testing.T) {
	x := Bandpass(tones(450, 30, []float64{0.9, 1.8}, []float64{0.95, 1}), 30, 0.7, 3.0)
	if p, _ := HeartFrequency(x, 30, 0.7, 3.0, 1.8); math.Abs(p.FreqHz-0.9) > 0.01 {
		t.Errorf("half with 90%% of the power, prior on the double: got %v, want 0.9", p.FreqHz)
	}
}

// A 55 bpm pulse with a strong harmonic: the band-pass leaves the harmonic with more than twice its power, and only
// undoing that gain shows the pulse.
func TestHeartFrequencyUndoesTheBandpassGain(t *testing.T) {
	x := Bandpass(tones(450, 30, []float64{55.0 / 60, 110.0 / 60}, []float64{1, 1.2}), 30, 0.7, 3.0)
	if p, _ := DominantFrequency(x, 30, 0.7, 3.0); math.Abs(p.FreqHz*60-110) > 1 {
		t.Fatalf("raw peak %.1f bpm, want the harmonic at 110 for this test to mean anything", p.FreqHz*60)
	}
	p, err := HeartFrequency(x, 30, 0.7, 3.0, math.NaN())
	if err != nil || math.Abs(p.FreqHz*60-55) > 1 {
		t.Fatalf("got %.1f bpm %v, want 55", p.FreqHz*60, err)
	}
}

func TestHeartFrequencyHalfMustBeInsideTheBand(t *testing.T) {
	x := tones(450, 30, []float64{0.6, 1.2}, []float64{1, 1})
	p, err := HeartFrequency(x, 30, 0.7, 3.0, math.NaN())
	if err != nil || math.Abs(p.FreqHz-1.2) > 0.01 {
		t.Fatalf("got %v %v, want 1.2: 0.6 Hz is below the band", p.FreqHz, err)
	}
}

func TestHeartFrequencyRejectsPeakBeyondBandEdge(t *testing.T) {
	for _, hz := range []float64{0.6, 3.05} {
		_, err := HeartFrequency(sine(450, 30, hz, 1), 30, 0.7, 3.0, math.NaN())
		if !errors.Is(err, ErrNoPeakInBand) {
			t.Errorf("hz=%v: err = %v, want ErrNoPeakInBand", hz, err)
		}
	}
}
