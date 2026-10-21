package rppg

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/dsp"
)

var (
	lightSkin = [3]float64{180, 140, 120}
	darkSkin  = [3]float64{90, 60, 45}
)

func TestEstimateHeartRateOnSyntheticFaces(t *testing.T) {
	tones := []struct {
		name     string
		dc       [3]float64
		pulseAmp float64
	}{
		{"light", lightSkin, 0.008},
		{"dark", darkSkin, 0.003},
	}
	for _, tone := range tones {
		for _, bpm := range []float64{45, 50, 72, 110, 175} {
			t.Run(fmt.Sprintf("%s/%v bpm", tone.name, bpm), func(t *testing.T) {
				ts, r, g, b := synth(synthConfig{
					bpm: bpm, durationS: 15, fps: 30, jitterMs: 8,
					dc: tone.dc, pulseAmp: tone.pulseAmp, noise: 0.1, seed: uint64(bpm),
				})
				hr, err := EstimateHeartRate(ts, r, g, b, DefaultConfig())
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if math.Abs(hr.BPM-bpm) > 1.5 {
					t.Fatalf("BPM = %.2f, want %v ± 1.5", hr.BPM, bpm)
				}
				if len(hr.Pulse) == 0 {
					t.Fatal("Pulse is empty, want the band-passed pulse")
				}
			})
		}
	}
}

func TestEstimateHeartRateDoesNotReportRateJustAboveBand(t *testing.T) {
	ts, r, g, b := synth(synthConfig{
		bpm: 183, durationS: 15, fps: 30, dc: lightSkin, pulseAmp: 0.008, noise: 0.1, seed: 3,
	})
	hr, err := EstimateHeartRate(ts, r, g, b, DefaultConfig())
	if !errors.Is(err, dsp.ErrNoPeakInBand) {
		t.Fatalf("got %.1f bpm, err = %v, want dsp.ErrNoPeakInBand", hr.BPM, err)
	}
}

func TestEstimateHeartRateRejectsNonFiniteSample(t *testing.T) {
	ts, r, g, b := synth(synthConfig{bpm: 72, durationS: 15, fps: 30, dc: lightSkin, pulseAmp: 0.008, seed: 1})
	g[150] = math.NaN()
	if _, err := EstimateHeartRate(ts, r, g, b, DefaultConfig()); !errors.Is(err, dsp.ErrNotFinite) {
		t.Fatalf("err = %v, want dsp.ErrNotFinite", err)
	}
}

func TestPOSIgnoresLightFlickerThatFoolsGreenChannel(t *testing.T) {
	ts, r, g, b := synth(synthConfig{
		bpm: 72, durationS: 15, fps: 30, dc: lightSkin, pulseAmp: 0.006,
		flickerHz: 1.8, flickerAmp: 0.02, noise: 0.1, seed: 7,
	})
	cfg := DefaultConfig()

	grid, err := dsp.Resample(ts, [][]float64{g}, cfg.SampleRateHz, cfg.MaxGapMs)
	if err != nil {
		t.Fatal(err)
	}
	green := dsp.Bandpass(dsp.Detrend(grid[0]), cfg.SampleRateHz, cfg.LowHz, cfg.HighHz)
	naive, err := dsp.DominantFrequency(green, cfg.SampleRateHz, cfg.LowHz, cfg.HighHz)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(naive.FreqHz*60-108) > 2 {
		t.Fatalf("green channel found %.1f bpm; the test expects it to lock on the 108 bpm flicker", naive.FreqHz*60)
	}

	hr, err := EstimateHeartRate(ts, r, g, b, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(hr.BPM-72) > 1.5 {
		t.Fatalf("POS BPM = %.2f, want 72 ± 1.5", hr.BPM)
	}
}

func TestEstimateHeartRateErrors(t *testing.T) {
	cfg := DefaultConfig()

	ts, r, g, b := synth(synthConfig{bpm: 72, durationS: 5, fps: 30, dc: lightSkin, pulseAmp: 0.008, seed: 1})
	if _, err := EstimateHeartRate(ts, r, g, b, cfg); !errors.Is(err, ErrTooShort) {
		t.Errorf("5 s window: err = %v, want ErrTooShort", err)
	}

	ts, r, g, b = synth(synthConfig{bpm: 72, durationS: 15, fps: 30, dc: lightSkin, pulseAmp: 0.008, seed: 1})
	for i := 200; i < len(ts); i++ {
		ts[i] += 400
	}
	if _, err := EstimateHeartRate(ts, r, g, b, cfg); !errors.Is(err, dsp.ErrGap) {
		t.Errorf("400 ms gap: err = %v, want dsp.ErrGap", err)
	}

	if _, err := EstimateHeartRate(nil, nil, nil, nil, cfg); !errors.Is(err, ErrTooShort) {
		t.Errorf("empty input: err = %v, want ErrTooShort", err)
	}
}

func TestPOSOfConstantColorIsZero(t *testing.T) {
	c := make([]float64, 90)
	for i := range c {
		c[i] = 100
	}
	for i, v := range POS(c, c, c, 48) {
		if math.Abs(v) > 1e-12 {
			t.Fatalf("POS[%d] = %v, want 0", i, v)
		}
	}
}
