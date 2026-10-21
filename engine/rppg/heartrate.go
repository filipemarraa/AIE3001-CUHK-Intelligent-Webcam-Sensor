package rppg

import (
	"errors"
	"math"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/dsp"
)

var ErrTooShort = errors.New("rppg: window shorter than the minimum duration")

type Config struct {
	SampleRateHz float64
	MaxGapMs     float64
	LowHz        float64
	HighHz       float64
	POSWindowS   float64
	MinDurationS float64
}

func DefaultConfig() Config {
	return Config{
		SampleRateHz: 30,
		MaxGapMs:     250,
		LowHz:        0.7,
		HighHz:       3.0,
		POSWindowS:   1.6,
		MinDurationS: 10,
	}
}

type HeartRate struct {
	BPM   float64
	SNRdB float64
	Pulse []float64
}

func EstimateHeartRate(tsMs, r, g, b []float64, cfg Config) (HeartRate, error) {
	if len(tsMs) < 2 || (tsMs[len(tsMs)-1]-tsMs[0])/1000 < cfg.MinDurationS {
		return HeartRate{}, ErrTooShort
	}
	grid, err := dsp.Resample(tsMs, [][]float64{r, g, b}, cfg.SampleRateHz, cfg.MaxGapMs)
	if err != nil {
		return HeartRate{}, err
	}

	windowLen := int(math.Round(cfg.POSWindowS * cfg.SampleRateHz))
	pulse := POS(grid[0], grid[1], grid[2], windowLen)
	pulse = dsp.Bandpass(dsp.Detrend(pulse), cfg.SampleRateHz, cfg.LowHz, cfg.HighHz)

	peak, err := dsp.DominantFrequency(pulse, cfg.SampleRateHz, cfg.LowHz, cfg.HighHz)
	if err != nil {
		return HeartRate{}, err
	}
	return HeartRate{BPM: peak.FreqHz * 60, SNRdB: peak.SNRdB, Pulse: pulse}, nil
}
