package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/eval"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/sim"
)

func TestReplayTableNamesTheFileThatFails(t *testing.T) {
	_, err := replayTable([]string{filepath.Join(t.TempDir(), "missing.jsonl")}, eval.DefaultReplayConfig())
	if err == nil || !strings.Contains(err.Error(), "missing.jsonl") {
		t.Fatalf("error %v, want one naming the file", err)
	}
	bad := filepath.Join(t.TempDir(), "bad.jsonl")
	if err := os.WriteFile(bad, []byte(`{"kind":"sample"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := replayTable([]string{bad}, eval.DefaultReplayConfig()); err == nil || !strings.Contains(err.Error(), "bad.jsonl: line 1") {
		t.Fatalf("error %v, want one naming the file and the line", err)
	}
}

func TestReplayTableHasARowPerSkinRegion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p1.jsonl")
	text := `{"kind":"header","version":1,"subject":"P1","condition":"good light"}
{"kind":"sample","v":[0,180,140,120,1,0.01,150,0.02,0.6,0.3],"skins":[180,140,120,181,141,121]}
{"kind":"reference","tMs":0,"hrBpm":70,"source":"watch"}
{"kind":"diagnostics","tMs":1000,"frameCostMs":20,"droppedFrames":0}
`
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	table, err := replayTable([]string{path}, eval.DefaultReplayConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"| p1.jsonl | P1 | good light | - | 100% | boxes | 1 | 0 | - |", "| p1.jsonl | P1 | good light | - | 100% | anatomical | 1 | 0 | - |", "SNR < 0 dB"} {
		if !strings.Contains(table, want) {
			t.Errorf("table lacks %q:\n%s", want, table)
		}
	}
}

// writeSynthetic writes a lab recording of a simulated person with a chest strap: the anatomical regions carry the
// pulse, the boxes only the skin colour, and a reference arrives every second.
func writeSynthetic(t *testing.T, path string, durationS float64) {
	t.Helper()
	sc := sim.Base("synthetic")
	sc.DurationS = durationS
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	write := func(v any) {
		if err := enc.Encode(v); err != nil {
			t.Fatal(err)
		}
	}
	write(map[string]any{"kind": "header", "version": 1, "subject": "S1", "condition": "synthetic",
		"regions": []string{"boxes", "anatomical"}})
	nextRef := 1000.0
	for _, x := range sc.Generate() {
		face, skins := 0.0, []float64{0, 0, 0, 0, 0, 0}
		if x.Face {
			face, skins = 1, []float64{sc.Skin[0], sc.Skin[1], sc.Skin[2], x.R, x.G, x.B}
		}
		write(map[string]any{"kind": "sample", "v": []float64{x.TimeMs, x.R, x.G, x.B, face, x.Motion, x.Light, x.Mouth,
			x.ShoulderY, x.ShoulderW}, "skins": skins})
		if x.TimeMs >= nextRef {
			write(map[string]any{"kind": "reference", "tMs": x.TimeMs, "hrBpm": sc.MeanHR(x.TimeMs/1000-1, x.TimeMs/1000),
				"source": "ble", "device": "Polar H10"})
			nextRef += 1000
		}
	}
}

func TestReplayTableScoresBySNRBand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s1.jsonl")
	writeSynthetic(t, path, 60)
	table, err := replayTable([]string{path}, eval.DefaultReplayConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Reference: mean over the last 15 s of each estimate, at least 5 readings",
		"| s1.jsonl | S1 | synthetic | 30.0 | 100% | anatomical | 59 | 50 | 100% |",
		"| anatomical | all | 50 | 100% |",
		"| anatomical | SNR ≥ 6 dB | 50 | 100% |",
		"| anatomical | SNR < 0 dB | 0 | 0% | - |",
	} {
		if !strings.Contains(table, want) {
			t.Errorf("table lacks %q:\n%s", want, table)
		}
	}
	cfg := eval.DefaultReplayConfig()
	cfg.ReferenceWindowS, cfg.MinReferences = 5, 20
	table, err = replayTable([]string{path}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if want := "| s1.jsonl | S1 | synthetic | 30.0 | 100% | anatomical | 59 | 0 | - |"; !strings.Contains(table, want) {
		t.Errorf("20 readings cannot fit in 5 s: table lacks %q:\n%s", want, table)
	}
}
