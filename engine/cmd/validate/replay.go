package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/eval"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/session"
)

// replayTable compares the skin regions of lab recordings against their reference heart rates.
func replayTable(paths []string) (string, error) {
	var b strings.Builder
	b.WriteString("| recording | subject | condition | skin | references | covered | HR MAE | HR P95 | MAE high/medium | mean SNR dB | 1st HR s |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|\n")
	type bin struct {
		n   int
		sum float64
	}
	bins := map[string][]bin{}
	var edges [][2]float64
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
		for _, r := range eval.CompareSkins(rec, session.DefaultConfig()) {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %d | %s | %s | %s | %s | %s | %s |\n", filepath.Base(path), rec.Subject,
				rec.Condition, r.Skin, r.References, pct(ratio(r.Covered, r.References)), num(r.MAE), num(r.P95),
				num(r.ConfidentMAE), num(r.MeanSNRdB), num(r.FirstHRS))
			if bins[r.Skin] == nil {
				bins[r.Skin] = make([]bin, len(r.BySNR))
			}
			edges = edges[:0]
			for i, s := range r.BySNR {
				edges = append(edges, [2]float64{s.FromDB, s.ToDB})
				if s.Readings > 0 {
					bins[r.Skin][i].n += s.Readings
					bins[r.Skin][i].sum += s.MAE * float64(s.Readings)
				}
			}
		}
	}
	b.WriteString("\nHeart-rate error by SNR (readings · MAE), all recordings:\n\n| skin |")
	for _, e := range edges {
		fmt.Fprintf(&b, " %s |", snrRange(e[0], e[1]))
	}
	b.WriteString("\n|---|" + strings.Repeat("---|", len(edges)) + "\n")
	for _, name := range eval.SkinNames {
		fmt.Fprintf(&b, "| %s |", name)
		for _, s := range bins[name] {
			mae := math.NaN()
			if s.n > 0 {
				mae = s.sum / float64(s.n)
			}
			fmt.Fprintf(&b, " %d · %s |", s.n, num(mae))
		}
		b.WriteString("\n")
	}
	return b.String(), nil
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
