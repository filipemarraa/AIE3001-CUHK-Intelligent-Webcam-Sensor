package resp

import (
	"errors"
	"math"
	"math/rand/v2"
	"testing"
)

func synthShoulders(rpm, durationS, fps, jitterMs, noise float64, seed uint64) (ts, y, w []float64) {
	rng := rand.New(rand.NewPCG(seed, seed+1))
	n := int(durationS * fps)
	f := rpm / 60
	for i := 0; i < n; i++ {
		t := float64(i)/fps + jitterMs/1000*(rng.Float64()-0.5)
		ts = append(ts, t*1000)
		y = append(y, 0.6-0.004*math.Sin(2*math.Pi*f*t)+noise*rng.NormFloat64())
		w = append(w, 0.3)
	}
	return ts, y, w
}

func TestEstimateRate(t *testing.T) {
	for _, rpm := range []float64{6, 10, 15, 24} {
		ts, y, w := synthShoulders(rpm, 26, 30, 8, 0, 1)
		got, err := Estimate(ts, y, w, PhaseUnknown, DefaultConfig())
		if err != nil {
			t.Fatalf("rpm %v: %v", rpm, err)
		}
		if math.Abs(got.RPM-rpm) > 1 {
			t.Errorf("rpm %v: got %.2f", rpm, got.RPM)
		}
	}
}

func TestDefaultConfigCoversSixRPM(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.LowHz != 0.08 || cfg.HighHz != 0.5 || cfg.MinDurationS != 25 || cfg.SampleRateHz != 10 {
		t.Fatalf("got %+v, want LowHz 0.08, HighHz 0.5, MinDurationS 25, SampleRateHz 10", cfg)
	}
	// DominantFrequency needs 2*fs/LowHz samples.
	if need := 2 / cfg.LowHz; cfg.MinDurationS < need {
		t.Fatalf("MinDurationS %v below the %v s the spectrum needs", cfg.MinDurationS, need)
	}
}

func TestEstimateAt6RPMAcrossWindows(t *testing.T) {
	ts, y, w := synthShoulders(6, 60, 30, 8, 0.0005, 1)
	for endMs := 26000.0; endMs <= 60000; endMs += 500 {
		n := 0
		for n < len(ts) && ts[n] < endMs {
			n++
		}
		got, err := Estimate(ts[:n], y[:n], w[:n], PhaseUnknown, DefaultConfig())
		if err != nil {
			t.Errorf("window ending %.1f s: %v", endMs/1000, err)
			continue
		}
		if math.Abs(got.RPM-6) > 1 || got.Phase == PhaseUnknown {
			t.Errorf("window ending %.1f s: %.2f rpm, phase %v", endMs/1000, got.RPM, got.Phase)
		}
	}
}

func TestEstimateRejectsShouldersWithoutBreathing(t *testing.T) {
	for seed := uint64(1); seed <= 10; seed++ {
		ts, y, w := synthShoulders(0, 30, 15, 8, 0.001, seed)
		got, err := Estimate(ts, y, w, PhaseUnknown, DefaultConfig())
		if !errors.Is(err, ErrUnreliable) {
			t.Errorf("seed %d: got %.1f rpm, err %v, want ErrUnreliable", seed, got.RPM, err)
		}
	}
}

func TestEstimateReportsPeriodicity(t *testing.T) {
	ts, y, w := synthShoulders(10, 30, 15, 8, 0.0005, 1)
	got, err := Estimate(ts, y, w, PhaseUnknown, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if got.Periodicity < 0.8 {
		t.Fatalf("periodicity %.2f for clean breathing, want at least 0.8", got.Periodicity)
	}
}

func TestEstimateTooShort(t *testing.T) {
	ts, y, w := synthShoulders(10, 15, 30, 8, 0, 1)
	if _, err := Estimate(ts, y, w, PhaseUnknown, DefaultConfig()); !errors.Is(err, ErrTooShort) {
		t.Fatalf("got %v, want ErrTooShort", err)
	}
}

func TestEstimateNoShoulders(t *testing.T) {
	ts, y, w := synthShoulders(10, 26, 30, 0, 0, 1)
	w[100] = 0
	if _, err := Estimate(ts, y, w, PhaseUnknown, DefaultConfig()); !errors.Is(err, ErrNoShoulders) {
		t.Fatalf("got %v, want ErrNoShoulders", err)
	}
}

func TestPhaseFollowsSignal(t *testing.T) {
	cases := []struct {
		name      string
		durationS float64
		want      Phase
	}{
		{"sin rising at 30 s", 30, PhaseIn},
		{"sin falling at 35 s", 35, PhaseOut},
	}
	for _, c := range cases {
		ts, y, w := synthShoulders(6, c.durationS, 30, 0, 0, 1)
		got, err := Estimate(ts, y, w, PhaseUnknown, DefaultConfig())
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got.Phase != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got.Phase, c.want)
		}
	}
}

func TestPhaseHoldWhenFlat(t *testing.T) {
	ts, y, w := synthShoulders(6, 25, 30, 0, 0, 1)
	last := y[len(y)-1]
	for i := 1; i <= 90; i++ {
		ts = append(ts, ts[len(ts)-1]+1000.0/30)
		y = append(y, last)
		w = append(w, 0.3)
	}
	got, err := Estimate(ts, y, w, PhaseIn, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != PhaseHold {
		t.Errorf("got %v, want hold", got.Phase)
	}
}

func TestPhaseHysteresis(t *testing.T) {
	cfg := DefaultConfig()
	cases := []struct {
		d    float64
		prev Phase
		want Phase
	}{
		{0.15, PhaseIn, PhaseIn},
		{-0.15, PhaseOut, PhaseOut},
		{0.15, PhaseHold, PhaseHold},
		{0.15, PhaseUnknown, PhaseHold},
		{0.25, PhaseHold, PhaseIn},
		{-0.25, PhaseIn, PhaseOut},
		{0.05, PhaseIn, PhaseHold},
		{-0.05, PhaseOut, PhaseHold},
	}
	for _, c := range cases {
		if got := nextPhase(c.d, c.prev, cfg); got != c.want {
			t.Errorf("d=%v prev=%v: got %v, want %v", c.d, c.prev, got, c.want)
		}
	}
}

func TestPhaseString(t *testing.T) {
	want := map[Phase]string{PhaseUnknown: "", PhaseIn: "in", PhaseOut: "out", PhaseHold: "hold"}
	for p, s := range want {
		if p.String() != s {
			t.Errorf("%d: got %q, want %q", int(p), p.String(), s)
		}
	}
}
