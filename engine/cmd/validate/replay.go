package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/eval"
)

// replayTable compares the skin regions of lab recordings against their reference heart rates: one row per recording
// and region, then the estimates of all recordings pooled by SNR band.
func replayTable(paths []string, cfg eval.ReplayConfig) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Reference: mean over the last %g s of each estimate, at least %d readings; estimates of the first %g s "+
		"(warm-up) are not scored.\n\n", cfg.ReferenceWindowS, cfg.MinReferences, cfg.Session.WarmupS)
	b.WriteString("| recording | subject | condition | fps | face | skin | references | evaluated | coverage | MAE | RMSE | ±5 bpm | bias | limits of agreement | harmonics | mean SNR dB | 1st HR s |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	pairs := map[string][]eval.Pair{}
	evaluated := map[string]int{}
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		rec, err := eval.ReadRecording(f)
		f.Close()
		if err != nil {
			return "", fmt.Errorf("%s: %w", path, err)
		}
		for _, r := range eval.CompareSkins(rec, cfg) {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %d | %d | %s | %s | %s | %s |\n", filepath.Base(path),
				rec.Subject, rec.Condition, fps(rec.EffectiveFPS()), pct(rec.FaceShare()), r.Skin, r.References, r.Evaluated, pct(r.Coverage()),
				errorCells(r.All), num(r.MeanSNRdB), num(r.FirstHRS))
			pairs[r.Skin] = append(pairs[r.Skin], r.Pairs...)
			evaluated[r.Skin] += r.Evaluated
		}
	}
	b.WriteString("\nHeart-rate error by SNR band, all recordings pooled (share: part of the evaluated estimates):\n\n")
	b.WriteString("| skin | SNR | estimates | share | MAE | RMSE | ±5 bpm | bias | limits of agreement | harmonics |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
	for _, name := range eval.SkinNames {
		all := eval.Stats(pairs[name])
		fmt.Fprintf(&b, "| %s | all | %d | %s | %s |\n", name, all.N, pct(ratio(all.N, evaluated[name])), errorCells(all))
		for _, band := range eval.Bands(pairs[name], evaluated[name]) {
			fmt.Fprintf(&b, "| %s | %s | %d | %s | %s |\n", name, snrRange(band.FromDB, band.ToDB), band.N,
				pct(band.Share), errorCells(band.ErrorStats))
		}
	}
	return b.String(), nil
}

// errorCells writes MAE, RMSE, share within ±5 bpm, bias, limits of agreement and harmonic errors.
func errorCells(s eval.ErrorStats) string {
	loa := "-"
	if !math.IsNaN(s.LoALow) {
		loa = fmt.Sprintf("%.2f to %.2f", s.LoALow, s.LoAHigh)
	}
	return fmt.Sprintf("%s | %s | %s | %s | %s | %d", num(s.MAE), num(s.RMSE), pct(s.Within5), num(s.Bias), loa, s.Harmonics)
}

func snrRange(from, to float64) string {
	switch {
	case math.IsInf(from, -1):
		return fmt.Sprintf("SNR < %g dB", to)
	case math.IsInf(to, 1):
		return fmt.Sprintf("SNR ≥ %g dB", from)
	}
	return fmt.Sprintf("%g to %g dB", from, to)
}

func ratio(a, b int) float64 {
	if b == 0 {
		return math.NaN()
	}
	return float64(a) / float64(b)
}

func fps(v float64) string {
	if math.IsNaN(v) {
		return "-"
	}
	return fmt.Sprintf("%.1f", v)
}
