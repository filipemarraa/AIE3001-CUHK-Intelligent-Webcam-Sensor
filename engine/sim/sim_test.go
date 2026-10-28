package sim

import (
	"math"
	"slices"
	"testing"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/session"
)

func TestGenerateIsDeterministicPerSeed(t *testing.T) {
	sc := Base("x")
	sc.CostMs, sc.CostSDMs = 45, 12
	sc.Seed = 7
	a, b := sc.Generate(), sc.Generate()
	if !slices.Equal(a, b) {
		t.Fatal("same seed produced different samples")
	}
	sc.Seed = 8
	if slices.Equal(a, sc.Generate()) {
		t.Fatal("different seeds produced the same samples")
	}
}

func TestBusyWorkerDropsFramesWithoutLongGaps(t *testing.T) {
	sc := Base("x")
	sc.CostMs, sc.CostSDMs = 45, 12
	sc.JitterMs = 10
	xs := sc.Generate()
	for i := 1; i < len(xs); i++ {
		if xs[i].TimeMs <= xs[i-1].TimeMs {
			t.Fatalf("timestamp %d not increasing: %v then %v", i, xs[i-1].TimeMs, xs[i].TimeMs)
		}
	}
	fps := float64(len(xs)) / sc.DurationS
	if fps < 13 || fps > 19 {
		t.Fatalf("effective rate %.1f fps, want 13 to 19 like the browser", fps)
	}
	if gap := maxGapMs(xs); gap > 150 {
		t.Fatalf("largest gap %.0f ms without stalls, want under 150", gap)
	}
}

func TestStallRateDoesNotDependOnWorkerLoad(t *testing.T) {
	count := func(costMs float64) int {
		sc := Base("x")
		sc.DurationS = 600
		sc.CostMs, sc.CostSDMs = costMs, 0
		sc.StallEveryS, sc.StallMs = 20, 400
		n := 0
		xs := sc.Generate()
		for i := 1; i < len(xs); i++ {
			if xs[i].TimeMs-xs[i-1].TimeMs > 300 {
				n++
			}
		}
		return n
	}
	for _, cost := range []float64{10, 45, 70} {
		if n := count(cost); n < 20 || n > 42 {
			t.Errorf("cost %v ms: %d stalls in 600 s, want about 30 (one every 20 s)", cost, n)
		}
	}
}

func TestStallsCreateLongGaps(t *testing.T) {
	sc := Base("x")
	sc.CostMs, sc.CostSDMs = 45, 12
	sc.StallEveryS, sc.StallMs = 20, 400
	if gap := maxGapMs(sc.Generate()); gap < 300 {
		t.Fatalf("largest gap %.0f ms with 400 ms stalls, want above 300", gap)
	}
}

func maxGapMs(xs []session.Sample) float64 {
	var g float64
	for i := 1; i < len(xs); i++ {
		g = max(g, xs[i].TimeMs-xs[i-1].TimeMs)
	}
	return g
}

func TestHeartRateTruth(t *testing.T) {
	sc := Base("x")
	sc.RSABPM = 0
	if got := sc.HR(0); got != 74 {
		t.Fatalf("HR(0) = %v, want 74", got)
	}
	if got := sc.HR(sc.DurationS); math.Abs(got-68) > 1e-9 {
		t.Fatalf("HR(end) = %v, want 68", got)
	}
	if got := sc.MeanHR(0, sc.DurationS); math.Abs(got-71) > 0.01 {
		t.Fatalf("mean HR = %v, want 71", got)
	}
}

func TestInhalingFollowsRisingShoulders(t *testing.T) {
	sc := Base("x")
	sc.ShoulderNoise, sc.BallisticAmp = 0, 0
	xs := sc.Generate()
	var agree, total int
	for i := 2; i < len(xs); i++ {
		in, clear := sc.Inhaling(xs[i].TimeMs / 1000)
		if !clear || xs[i].ShoulderY == xs[i-2].ShoulderY {
			continue
		}
		rising := xs[i].ShoulderY < xs[i-2].ShoulderY
		total++
		if rising == in {
			agree++
		}
	}
	if total == 0 || float64(agree)/float64(total) < 0.95 {
		t.Fatalf("inhaling agrees with rising shoulders in %d of %d samples", agree, total)
	}
}

func TestFaceLostAndMotionAndTalking(t *testing.T) {
	sc := Base("x")
	sc.FaceLost = []Burst{{StartS: 10, DurS: 2}}
	sc.Motion = []Burst{{StartS: 20, DurS: 5}}
	sc.MotionSpeed = 3
	sc.Talking = []Burst{{StartS: 30, DurS: 5}}
	var lost, still, moving []float64
	var mouthQuiet, mouthTalking []float64
	for _, x := range sc.Generate() {
		ts := x.TimeMs / 1000
		switch {
		case ts >= 10 && ts < 12:
			if x.Face || x.R != 0 {
				t.Fatalf("face lost at %.2f s but sample has face data", ts)
			}
			lost = append(lost, ts)
		case ts >= 20.5 && ts < 25:
			moving = append(moving, x.Motion)
		case ts >= 30 && ts < 35:
			mouthTalking = append(mouthTalking, x.Mouth)
		case ts > 1 && ts < 10:
			still = append(still, x.Motion)
			mouthQuiet = append(mouthQuiet, x.Mouth)
		}
	}
	if len(lost) == 0 {
		t.Fatal("no face-lost samples")
	}
	if mean(moving) < 10*mean(still) {
		t.Fatalf("motion during burst %.4f, still %.4f: want at least 10x", mean(moving), mean(still))
	}
	if std(mouthTalking) < 0.03 || std(mouthQuiet) > 0.01 {
		t.Fatalf("mouth std talking %.3f quiet %.3f", std(mouthTalking), std(mouthQuiet))
	}
}

func TestSuiteNamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, sc := range Suite() {
		if seen[sc.Name] {
			t.Fatalf("duplicate scenario %q", sc.Name)
		}
		seen[sc.Name] = true
	}
}

func mean(x []float64) float64 {
	var s float64
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

func std(x []float64) float64 {
	m := mean(x)
	var s float64
	for _, v := range x {
		s += (v - m) * (v - m)
	}
	return math.Sqrt(s / float64(len(x)))
}

// amplitude projects x onto a tone at hz, so it measures one harmonic of a signal sampled at ts seconds.
func amplitude(ts, x []float64, hz float64) float64 {
	var re, im float64
	for i, t := range ts {
		re += x[i] * math.Cos(2*math.Pi*hz*t)
		im += x[i] * math.Sin(2*math.Pi*hz*t)
	}
	return 2 * math.Hypot(re, im) / float64(len(ts))
}

func TestHarmonicSetsTheSecondHarmonicOfThePulse(t *testing.T) {
	for _, h := range []float64{0, 0.4, 1} {
		sc := Base("x")
		sc.BaseBPM, sc.DriftBPM, sc.RSABPM = 60, 0, 0
		sc.Noise, sc.JitterMs = 0, 0
		sc.Harmonic = h
		var ts, g []float64
		for _, x := range sc.Generate() {
			ts, g = append(ts, x.TimeMs/1000), append(g, x.G)
		}
		ratio := amplitude(ts, g, 2) / amplitude(ts, g, 1)
		if math.Abs(ratio-h) > 0.05 {
			t.Errorf("harmonic %v: second over first harmonic = %.3f", h, ratio)
		}
	}
}

func TestHarmonicChangesAtHarmonicAtS(t *testing.T) {
	sc := Base("x")
	sc.BaseBPM, sc.DriftBPM, sc.RSABPM = 60, 0, 0
	sc.Noise, sc.JitterMs = 0, 0
	sc.Harmonic, sc.LaterHarmonic, sc.HarmonicAtS = 1.5, 0.6, 60
	var ts, g [2][]float64
	for _, x := range sc.Generate() {
		half := 0
		if x.TimeMs >= 60000 {
			half = 1
		}
		ts[half], g[half] = append(ts[half], x.TimeMs/1000), append(g[half], x.G)
	}
	for half, want := range []float64{1.5, 0.6} {
		if ratio := amplitude(ts[half], g[half], 2) / amplitude(ts[half], g[half], 1); math.Abs(ratio-want) > 0.05 {
			t.Errorf("half %d: second over first harmonic = %.3f, want %v", half, ratio, want)
		}
	}
}

func TestColourDriftIsSlowAndIndependentPerChannel(t *testing.T) {
	sc := Base("x")
	sc.DurationS = 600
	sc.PulseAmp, sc.Noise, sc.JitterMs = 0, 0, 0
	sc.CostMs, sc.CostSDMs = 45, 12
	sc.DriftNoise, sc.DriftTauS = 0.01, 1
	var r, g []float64
	for _, x := range sc.Generate() {
		// The slow 0.05 Hz light swing is common to all channels; the ratio leaves only the drift.
		r, g = append(r, x.R/LightSkin[0]), append(g, x.G/LightSkin[1])
	}
	rel := make([]float64, len(r))
	for i := range r {
		rel[i] = r[i] / g[i]
	}
	if sd := std(rel); sd < 0.01 || sd > 0.02 {
		t.Errorf("std of R/G = %.4f, want about 0.014 (two independent channels of 0.01)", sd)
	}
	if ac := lag1(rel); ac < 0.9 {
		t.Errorf("lag-1 autocorrelation %.2f, want above 0.9 for a 1 s time constant at ~16 fps", ac)
	}
}

func TestNoDriftLeavesSamplesUnchanged(t *testing.T) {
	sc := Base("x")
	sc.CostMs, sc.CostSDMs = 45, 12
	a := sc.Generate()
	sc.DriftTauS = 1
	if !slices.Equal(a, sc.Generate()) {
		t.Fatal("a time constant without drift noise changed the samples")
	}
}

func lag1(x []float64) float64 {
	m := mean(x)
	var num, den float64
	for i := range x {
		den += (x[i] - m) * (x[i] - m)
		if i > 0 {
			num += (x[i] - m) * (x[i-1] - m)
		}
	}
	return num / den
}

// The suite must not be trivially easy: it spans the heart-rate band and includes the conditions that fool peak picking.
func TestSuiteCoversHardHeartRates(t *testing.T) {
	var minHR, maxHR, maxHarmonic, maxDrift float64 = math.Inf(1), 0, 0, 0
	for _, sc := range Suite() {
		if sc.PulseAmp == 0 {
			continue
		}
		minHR, maxHR = min(minHR, sc.MeanHR(0, sc.DurationS)), max(maxHR, sc.MeanHR(0, sc.DurationS))
		maxHarmonic, maxDrift = max(maxHarmonic, sc.Harmonic), max(maxDrift, sc.DriftNoise)
		if sc.CostMs == 0 && sc.Name != "ideal" {
			t.Errorf("%s: no worker cost, so no frames are dropped", sc.Name)
		}
	}
	if minHR > 46 || maxHR < 170 {
		t.Errorf("mean heart rates span %.0f to %.0f bpm, want 45 to 175", minHR, maxHR)
	}
	if maxHarmonic < 0.8 {
		t.Errorf("strongest second harmonic %.2f, want a scenario at 0.8 or more", maxHarmonic)
	}
	if maxDrift == 0 {
		t.Error("no scenario has coloured drift noise")
	}
}
