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

// recording writes a scenario as the lab page would: the regions carry the pulse, the boxes only noise. With
// estimates, an estimate line follows the first sample of every second, as the worker estimates after pushing it.
// A reference follows every refEveryMs: 1000 is a Bluetooth chest strap (the mean heart rate of the last second),
// 15000 a watch read by hand (the mean of the last 10 s).
func recording(t *testing.T, sc sim.Scenario, estimates bool, refEveryMs float64) string {
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
	nextRef := refEveryMs
	nextEstimate := 1000.0
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
		if estimates && x.TimeMs >= nextEstimate {
			write(map[string]any{"kind": "estimate", "e": map[string]any{"tMs": x.TimeMs, "hrBpm": nil}})
			nextEstimate = (math.Floor(x.TimeMs/1000) + 1) * 1000
		}
		if x.TimeMs >= nextRef {
			if refEveryMs < 15000 {
				write(map[string]any{"kind": "reference", "tMs": x.TimeMs, "hrBpm": sc.MeanHR(x.TimeMs/1000-1, x.TimeMs/1000),
					"source": "ble", "device": "Polar H10", "rrMs": []float64{850.5}})
			} else {
				write(map[string]any{"kind": "reference", "tMs": x.TimeMs, "hrBpm": sc.MeanHR(x.TimeMs/1000-10, x.TimeMs/1000),
					"source": "watch"})
			}
			nextRef += refEveryMs
		}
	}
	return b.String()
}

func TestReadRecording(t *testing.T) {
	sc := sim.Base("replay")
	sc.DurationS = 40
	rec, err := ReadRecording(strings.NewReader(recording(t, sc, true, 15000)))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Subject != "P1" || rec.Condition != "sim" {
		t.Fatalf("subject %q condition %q, want P1 and sim", rec.Subject, rec.Condition)
	}
	if share := rec.FaceShare(); share != 1 {
		t.Fatalf("face share %v, want 1", share)
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
	rec, err := ReadRecording(strings.NewReader(recording(t, sc, true, 1000)))
	if err != nil {
		t.Fatal(err)
	}
	results := CompareSkins(rec, DefaultReplayConfig())
	if len(results) != 2 || results[0].Skin != "boxes" || results[1].Skin != "anatomical" {
		t.Fatalf("results %+v, want boxes then anatomical", results)
	}
	boxes, regions := results[0], results[1]
	// An estimate a second from the end of the 10 s warm-up to 90 s.
	if regions.References != 89 || regions.Evaluated < 78 || regions.Evaluated > 82 {
		t.Fatalf("%d references and %d estimates evaluated, want 89 (one a second from 1 s) and about 80",
			regions.References, regions.Evaluated)
	}
	if c := regions.Coverage(); c < 0.85 || regions.All.MAE > 2 || regions.All.Within5 < 0.95 || !(regions.MeanSNRdB > 3) {
		t.Fatalf("regions coverage %v, %+v, want most estimates within 2 bpm and SNR above 3 dB", c, regions.All)
	}
	if regions.All.Harmonics != 0 || math.Abs(regions.All.Bias) > 1 || regions.All.LoAHigh-regions.All.LoALow > 8 {
		t.Fatalf("regions %+v, want no harmonic, a small bias and narrow limits of agreement", regions.All)
	}
	if !(boxes.All.MAE > 5 || boxes.Covered < boxes.Evaluated/2) {
		t.Fatalf("boxes %+v: noise without a pulse should not match the reference", boxes)
	}
	if math.IsNaN(regions.FirstHRS) || regions.FirstHRS > 12 {
		t.Fatalf("first heart rate at %v s, want by 12 s", regions.FirstHRS)
	}
	var binned int
	for _, band := range regions.BySNR {
		binned += band.N
	}
	if binned != regions.Covered || len(regions.Pairs) != regions.Covered {
		t.Fatalf("SNR bands hold %d readings and %d pairs, want %d", binned, len(regions.Pairs), regions.Covered)
	}
}

func TestCompareSkinsNeedsEnoughReferencesInTheWindow(t *testing.T) {
	sc := sim.Base("replay")
	sc.DurationS = 60
	rec, err := ReadRecording(strings.NewReader(recording(t, sc, true, 15000)))
	if err != nil {
		t.Fatal(err)
	}
	if r := CompareSkins(rec, DefaultReplayConfig())[1]; r.Evaluated != 0 || r.Covered != 0 {
		t.Fatalf("a reading every 15 s evaluated %d estimates with the default minimum, want none", r.Evaluated)
	}
	cfg := DefaultReplayConfig()
	cfg.MinReferences = 1
	if r := CompareSkins(rec, cfg)[1]; r.Evaluated == 0 || r.All.MAE > 3 {
		t.Fatalf("with one reference enough: %+v, want estimates evaluated within 3 bpm", r)
	}
}

func TestReferenceIsTheMeanOverTheAnalysisWindow(t *testing.T) {
	var refs []Reference
	for s := 0; s <= 30; s++ {
		refs = append(refs, Reference{TimeMs: float64(s) * 1000, HRBpm: 60 + float64(s)})
	}
	for _, tc := range []struct {
		tMs, windowS float64
		min          int
		want         float64
		ok           bool
	}{
		{30000, 15, 5, 82.5, true}, // 15 to 30 s: 75 to 90 bpm
		{30000, 10, 5, 85, true},
		{3000, 15, 5, 0, false}, // only 4 readings so far
		{4000, 15, 5, 62, true},
		{40000, 10, 5, 0, false}, // the strap stopped at 30 s: one reading left in 30 to 40 s
		{40000, 10, 1, 90, true},
		{45000, 10, 1, 0, false},
	} {
		got, ok := referenceAt(refs, tc.tMs, tc.windowS*1000, tc.min)
		if ok != tc.ok || (ok && math.Abs(got-tc.want) > 1e-9) {
			t.Errorf("at %v ms over %v s with %d: %v %v, want %v %v", tc.tMs, tc.windowS, tc.min, got, ok, tc.want, tc.ok)
		}
	}
}

func TestErrorStats(t *testing.T) {
	pairs := []Pair{{EstBpm: 72, RefBpm: 70}, {EstBpm: 80, RefBpm: 80}, {EstBpm: 140, RefBpm: 70}, {EstBpm: 35, RefBpm: 70}}
	s := Stats(pairs)
	sd := math.Sqrt(5786.75 / 3)
	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"MAE", s.MAE, 26.75},
		{"RMSE", s.RMSE, math.Sqrt(1532.25)},
		{"within 5", s.Within5, 0.5},
		{"bias", s.Bias, 9.25},
		{"LoA low", s.LoALow, 9.25 - 1.96*sd},
		{"LoA high", s.LoAHigh, 9.25 + 1.96*sd},
	} {
		if math.Abs(c.got-c.want) > 1e-9 {
			t.Errorf("%s %v, want %v", c.name, c.got, c.want)
		}
	}
	if s.N != 4 || s.Harmonics != 2 {
		t.Errorf("N %d harmonics %d, want 4 and 2 (double and half)", s.N, s.Harmonics)
	}
	if e := Stats(nil); e.N != 0 || !math.IsNaN(e.MAE) || !math.IsNaN(e.LoALow) {
		t.Errorf("no pairs: %+v, want NaN", e)
	}
}

func TestBandsSplitThePairsBySNR(t *testing.T) {
	pairs := []Pair{
		{EstBpm: 70, RefBpm: 70, SNRdB: -2}, {EstBpm: 80, RefBpm: 70, SNRdB: 1}, {EstBpm: 71, RefBpm: 70, SNRdB: 4},
		{EstBpm: 72, RefBpm: 70, SNRdB: 10}, {EstBpm: 70, RefBpm: 70, SNRdB: math.NaN()},
	}
	bands := Bands(pairs, 10)
	if len(bands) != 4 {
		t.Fatalf("%d bands, want 4", len(bands))
	}
	for i, want := range []struct {
		n     int
		mae   float64
		share float64
	}{{1, 0, 0.1}, {1, 10, 0.1}, {1, 1, 0.1}, {1, 2, 0.1}} {
		if b := bands[i]; b.N != want.n || b.MAE != want.mae || b.Share != want.share {
			t.Errorf("band %d (%v to %v dB): %+v, want %+v", i, b.FromDB, b.ToDB, b, want)
		}
	}
}

func TestReadRecordingKeepsTheBluetoothReferences(t *testing.T) {
	text := `{"kind":"header","version":1}
{"kind":"reference","tMs":1000.5,"hrBpm":64,"source":"ble","device":"Polar H10 1A2B","rrMs":[937.5,940]}
{"kind":"reference","tMs":2000,"hrBpm":66,"source":"watch"}`
	rec, err := ReadRecording(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	want := []Reference{{TimeMs: 1000.5, HRBpm: 64, Source: "ble"}, {TimeMs: 2000, HRBpm: 66, Source: "watch"}}
	if len(rec.References) != 2 || rec.References[0] != want[0] || rec.References[1] != want[1] {
		t.Fatalf("references %+v, want %+v", rec.References, want)
	}
}

func TestCompareSkinsTreatsAMissingRegionAsNoFace(t *testing.T) {
	sc := sim.Base("replay")
	sc.DurationS = 40
	rec, err := ReadRecording(strings.NewReader(recording(t, sc, true, 1000)))
	if err != nil {
		t.Fatal(err)
	}
	for i := range rec.Samples {
		rec.Samples[i].Skins[3], rec.Samples[i].Skins[4], rec.Samples[i].Skins[5] = 0, 0, 0
	}
	if r := CompareSkins(rec, DefaultReplayConfig())[1]; r.Evaluated == 0 || r.Covered != 0 {
		t.Fatalf("regions without pixels covered %d of %d estimates, want none", r.Covered, r.Evaluated)
	}
}

func TestReplayEstimatesWhenTheWorkerDid(t *testing.T) {
	sc := sim.Base("replay")
	sc.DurationS = 20
	for _, estimates := range []bool{true, false} {
		rec, err := ReadRecording(strings.NewReader(recording(t, sc, estimates, 1000)))
		if err != nil {
			t.Fatal(err)
		}
		if got := len(rec.EstimateTimes) > 0; got != estimates {
			t.Fatalf("estimate times read: %v, want %v", got, estimates)
		}
		if first := CompareSkins(rec, DefaultReplayConfig())[1].FirstHRS; !(first <= 10.1) {
			t.Errorf("recorded estimates %v: first heart rate at %v s, want by 10.1 s as in the worker", estimates, first)
		}
	}
}

func TestReplayKeepsTheRegionThatFoundSkin(t *testing.T) {
	sc := sim.Base("replay")
	sc.DurationS = 50
	rec, err := ReadRecording(strings.NewReader(recording(t, sc, true, 1000)))
	if err != nil {
		t.Fatal(err)
	}
	for i := range rec.Samples {
		v := &rec.Samples[i].Values
		v[session.FieldFace], v[session.FieldR], v[session.FieldG], v[session.FieldB], v[session.FieldLight] = 0, 0, 0, 0, 0
		rec.Samples[i].Skins[0], rec.Samples[i].Skins[1], rec.Samples[i].Skins[2] = 0, 0, 0
	}
	results := CompareSkins(rec, DefaultReplayConfig())
	if results[0].Covered != 0 || results[1].Coverage() < 0.85 {
		t.Fatalf("boxes covered %d, regions %d of %d: the regions found skin while the boxes did not",
			results[0].Covered, results[1].Covered, results[1].Evaluated)
	}
}
