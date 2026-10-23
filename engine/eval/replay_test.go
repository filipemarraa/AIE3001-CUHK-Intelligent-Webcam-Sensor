package eval

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/session"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/sim"
)

// recording writes a scenario as the lab page would: the regions carry the pulse, the boxes only noise.
func recording(t *testing.T, sc sim.Scenario) string {
	t.Helper()
	rng := rand.New(rand.NewPCG(7, 8))
	var b strings.Builder
	write := func(v any) {
		line, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	write(map[string]any{"kind": "header", "version": 1, "subject": "P1", "condition": "sim",
		"regions": []string{"boxes", "anatomical"}})
	nextRef := 15000.0
	for _, x := range sc.Generate() {
		face := 0.0
		if x.Face {
			face = 1
		}
		v := []float64{x.TimeMs, x.R, x.G, x.B, face, x.Motion, x.Light, x.Mouth, x.ShoulderY, x.ShoulderW}
		skins := []float64{0, 0, 0, 0, 0, 0}
		if x.Face {
			skins = []float64{
				sc.Skin[0] + 0.1*rng.NormFloat64(), sc.Skin[1] + 0.1*rng.NormFloat64(), sc.Skin[2] + 0.1*rng.NormFloat64(),
				x.R, x.G, x.B,
			}
		}
		write(map[string]any{"kind": "sample", "v": v, "skins": skins})
		if x.TimeMs >= nextRef {
			write(map[string]any{"kind": "reference", "tMs": x.TimeMs, "hrBpm": sc.MeanHR(x.TimeMs/1000-10, x.TimeMs/1000),
				"source": "watch"})
			nextRef += 15000
		}
	}
	write(map[string]any{"kind": "estimate", "e": map[string]any{"tMs": 1000, "hrBpm": nil}})
	return b.String()
}

func TestReadRecording(t *testing.T) {
	sc := sim.Base("replay")
	sc.DurationS = 40
	rec, err := ReadRecording(strings.NewReader(recording(t, sc)))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Subject != "P1" || rec.Condition != "sim" {
		t.Fatalf("subject %q condition %q, want P1 and sim", rec.Subject, rec.Condition)
	}
	if len(rec.Samples) != len(sc.Generate()) || len(rec.References) != 2 {
		t.Fatalf("%d samples and %d references, want %d and 2", len(rec.Samples), len(rec.References), len(sc.Generate()))
	}
}

func TestReadRecordingNamesTheBadLine(t *testing.T) {
	for _, tc := range []struct{ name, text, want string }{
		{"no header", `{"kind":"sample","v":[1,2,3,4,5,6,7,8,9,10],"skins":[1,2,3,4,5,6]}`, "line 1"},
		{"version", `{"kind":"header","version":2}`, "version 2"},
		{"regions", `{"kind":"header","version":1,"regions":["anatomical","boxes"]}`, "regions"},
		{"short sample", "{\"kind\":\"header\",\"version\":1}\n\n{\"kind\":\"sample\",\"v\":[1,2],\"skins\":[1,2,3,4,5,6]}", "line 3"},
		{"bad json", "{\"kind\":\"header\",\"version\":1}\n{\"kind\":", "line 2"},
	} {
		_, err := ReadRecording(strings.NewReader(tc.text))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want one mentioning %q", tc.name, err, tc.want)
		}
	}
}

func TestCompareSkinsAgainstTheReference(t *testing.T) {
	sc := sim.Base("replay")
	sc.DurationS = 90
	rec, err := ReadRecording(strings.NewReader(recording(t, sc)))
	if err != nil {
		t.Fatal(err)
	}
	results := CompareSkins(rec, session.DefaultConfig())
	if len(results) != 2 || results[0].Skin != "boxes" || results[1].Skin != "anatomical" {
		t.Fatalf("results %+v, want boxes then anatomical", results)
	}
	boxes, regions := results[0], results[1]
	if regions.References != 5 || regions.Covered != 5 || regions.MAE > 2 || !(regions.MeanSNRdB > 3) {
		t.Fatalf("regions %+v, want the 5 references (every 15 s) covered within 2 bpm and SNR above 3 dB", regions)
	}
	if !(boxes.MAE > 5 || boxes.Covered < boxes.References) {
		t.Fatalf("boxes %+v: noise without a pulse should not match the reference", boxes)
	}
	if math.IsNaN(regions.FirstHRS) || regions.FirstHRS > 12 {
		t.Fatalf("first heart rate at %v s, want by 12 s", regions.FirstHRS)
	}
	var binned int
	for _, bin := range regions.BySNR {
		binned += bin.Readings
	}
	if binned != regions.Covered {
		t.Fatalf("SNR bins hold %d readings, want %d", binned, regions.Covered)
	}
}

func TestCompareSkinsTreatsAMissingRegionAsNoFace(t *testing.T) {
	sc := sim.Base("replay")
	sc.DurationS = 40
	rec, err := ReadRecording(strings.NewReader(recording(t, sc)))
	if err != nil {
		t.Fatal(err)
	}
	for i := range rec.Samples {
		rec.Samples[i].Skins[3], rec.Samples[i].Skins[4], rec.Samples[i].Skins[5] = 0, 0, 0
	}
	if r := CompareSkins(rec, session.DefaultConfig())[1]; r.Covered != 0 {
		t.Fatalf("regions without pixels covered %d references, want none", r.Covered)
	}
}
