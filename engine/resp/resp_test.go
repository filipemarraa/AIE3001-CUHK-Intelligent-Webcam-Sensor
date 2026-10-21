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

func TestPhaseIgnoresCardiacMotionOfTheShoulders(t *testing.T) {
	const fps = 30.0
	var ts, y, w []float64
	for i := 0; i < int(60*fps); i++ {
		t := float64(i) / fps
		ts = append(ts, t*1000)
		y = append(y, 0.6-0.004*math.Sin(2*math.Pi*0.1*t)+0.001*math.Sin(2*math.Pi*t+3*math.Pi/4))
		w = append(w, 0.3)
	}
	var clear, calls, agree int
	prev := PhaseUnknown
	for end := 27.0; end <= 60; end += 0.5 {
		n := int(end * fps)
		got, err := Estimate(ts[:n], y[:n], w[:n], prev, DefaultConfig())
		if err != nil {
			t.Fatalf("window ending %.1f s: %v", end, err)
		}
		prev = got.Phase
		c := math.Cos(2 * math.Pi * 0.1 * ts[n-1] / 1000)
		if math.Abs(c) < 0.5 {
			continue
		}
		clear++
		if got.Phase == PhaseIn || got.Phase == PhaseOut {
			calls++
			if (got.Phase == PhaseIn) == (c > 0) {
				agree++
			}
		}
	}
	if float64(calls) < 0.8*float64(clear) || agree < calls {
		t.Fatalf("%d calls in %d clear moments, %d right: want at least 80%% calls and all right", calls, clear, agree)
	}
}

// shoulderSeries breathes at rpm until pauseAtS (then holds still, if pauseAtS < durationS), with the heartbeat
// moving the shoulders by tremor at tremorHz and tremorPhase the whole time.
func shoulderSeries(rpm, durationS, pauseAtS, tremor, tremorHz, tremorPhase, noise float64) (ts, y, w []float64) {
	const fps = 30.0
	rng := rand.New(rand.NewPCG(uint64(rpm*1000+pauseAtS), uint64(tremorHz*1000+tremorPhase)))
	f := rpm / 60
	var held float64
	for i := 0; i < int(durationS*fps); i++ {
		t := float64(i) / fps
		breath := 0.6 - 0.004*math.Sin(2*math.Pi*f*t)
		if t < pauseAtS {
			held = breath
		} else {
			breath = held
		}
		ts = append(ts, t*1000)
		y = append(y, breath+tremor*math.Sin(2*math.Pi*tremorHz*t+tremorPhase)+noise*rng.NormFloat64())
		w = append(w, 0.3)
	}
	return ts, y, w
}

var tremors = []struct{ amp, hz float64 }{{0, 0}, {0.001, 0.7}, {0.001, 1}, {0.001, 1.3}, {0.001, 2}}

// estimateAlong calls Estimate twice a second up to endS, carrying the phase like the session does, and returns the
// phase at each of the given times.
func estimateAlong(t *testing.T, ts, y, w []float64, fromS, endS float64, at ...float64) map[float64]Phase {
	t.Helper()
	out := map[float64]Phase{}
	prev := PhaseUnknown
	for end := fromS; end <= endS+1e-9; end += 0.5 {
		n := int(end * 30)
		got, err := Estimate(ts[:n], y[:n], w[:n], prev, DefaultConfig())
		if err != nil {
			t.Fatalf("window ending %.1f s: %v", end, err)
		}
		prev = got.Phase
		for _, a := range at {
			if _, seen := out[a]; !seen && end >= a {
				out[a] = got.Phase
			}
		}
	}
	return out
}

func TestPhaseHoldDuringPauses(t *testing.T) {
	for _, rpm := range []float64{6, 10, 15, 24} {
		period := 60 / rpm
		for _, frac := range []float64{0, 0.13, 0.27, 0.4, 0.55, 0.7, 0.85} {
			pauseAt := 25 + frac*period
			for _, tr := range tremors {
				noises := []float64{0}
				// Strong heartbeat motion together with landmark noise can delay a pause past 4 s; each alone cannot.
				if tr.amp == 0 {
					noises = append(noises, 0.0002)
				}
				for _, noise := range noises {
					ts, y, w := shoulderSeries(rpm, pauseAt+5, pauseAt, tr.amp, tr.hz, 3*math.Pi/4, noise)
					got := estimateAlong(t, ts, y, w, 25.5, pauseAt+4.5, pauseAt+3, pauseAt+4)
					for _, after := range []float64{3, 4} {
						if p := got[pauseAt+after]; p != PhaseHold {
							t.Errorf("%v rpm, pause from %.2f s, tremor %v Hz, noise %v: %v s in got %v, want hold",
								rpm, pauseAt, tr.hz, noise, after, p)
						}
					}
				}
			}
		}
	}
}

func TestPhaseNeverWrongWhileBreathingContinues(t *testing.T) {
	for _, rpm := range []float64{6, 10, 15, 24} {
		f := rpm / 60
		for _, tr := range tremors {
			for _, tremorPhase := range []float64{0, math.Pi / 2, math.Pi, 3 * math.Pi / 2} {
				ts, y, w := shoulderSeries(rpm, 45, 1e9, tr.amp, tr.hz, tremorPhase, 0.0005)
				prev := PhaseUnknown
				var clear, calls int
				for end := 25.5; end <= 45; end += 0.1 {
					n := int(end * 30)
					got, err := Estimate(ts[:n], y[:n], w[:n], prev, DefaultConfig())
					if err != nil {
						t.Fatalf("%v rpm at %.1f s: %v", rpm, end, err)
					}
					prev = got.Phase
					slope := math.Cos(2 * math.Pi * f * ts[n-1] / 1000)
					if end < 27 || math.Abs(slope) < 0.5 {
						continue
					}
					clear++
					if got.Phase == PhaseIn || got.Phase == PhaseOut {
						calls++
						if (got.Phase == PhaseIn) != (slope > 0) {
							t.Errorf("%v rpm, tremor %v Hz phase %.2f, at %.1f s: called %v against the breathing",
								rpm, tr.hz, tremorPhase, end, got.Phase)
						}
					}
				}
				if float64(calls) < 0.95*float64(clear) {
					t.Errorf("%v rpm, tremor %v Hz phase %.2f: %d calls in %d clear moments, want at least 95%%",
						rpm, tr.hz, tremorPhase, calls, clear)
				}
			}
		}
	}
}

// After a change of rhythm the rate of the 30 s window lags, and until the new rhythm dominates the window the phase
// can be called against the breathing (a known limit, measured at up to about 20 s; see docs/decisions.md). From 22 s
// on it must never be wrong and must be calling again; a window without an estimate counts as no call.
func TestPhaseRecoversAfterRhythmOrDepthChange(t *testing.T) {
	const fps = 30.0
	cases := []struct {
		name             string
		rpmFrom, rpmTo   float64
		depthFrom, depth float64
	}{
		{"6 to 10 rpm", 6, 10, 0.004, 0.004},
		{"10 to 6 rpm", 10, 6, 0.004, 0.004},
		{"12 to 6 rpm", 12, 6, 0.004, 0.004},
		{"6 to 15 rpm", 6, 15, 0.004, 0.004},
		{"deeper at 10 rpm", 10, 10, 0.002, 0.006},
		{"shallower at 6 rpm", 6, 6, 0.006, 0.002},
	}
	for _, c := range cases {
		for _, phase0 := range []float64{0, math.Pi/2 + 0.005, math.Pi, 3 * math.Pi / 2, 1, 2.5} {
			for _, step := range []float64{0.1, 1} {
				var ts, y, w, slope []float64
				phase := phase0
				for i := 0; i < int(80*fps); i++ {
					t := float64(i) / fps
					rpm, depth := c.rpmFrom, c.depthFrom
					if t >= 25 {
						rpm, depth = c.rpmTo, c.depth
					}
					phase += 2 * math.Pi * rpm / 60 / fps
					ts = append(ts, t*1000)
					y = append(y, 0.6-depth*math.Sin(phase))
					w = append(w, 0.3)
					slope = append(slope, math.Cos(phase))
				}
				prev := PhaseUnknown
				var clear, calls int
				for end := 25.5; end <= 80; end += step {
					n := int(end * fps)
					from := max(0, n-int(30*fps))
					got, err := Estimate(ts[from:n], y[from:n], w[from:n], prev, DefaultConfig())
					if err == nil {
						prev = got.Phase
					}
					s := slope[n-1]
					if end < 25+22 || math.Abs(s) < 0.5 {
						continue
					}
					clear++
					if err != nil || (got.Phase != PhaseIn && got.Phase != PhaseOut) {
						continue
					}
					calls++
					if (got.Phase == PhaseIn) != (s > 0) {
						t.Errorf("%s, start phase %.2f, every %v s, at %.1f s: called %v against the breathing",
							c.name, phase0, step, end, got.Phase)
					}
				}
				if clear == 0 || float64(calls) < 0.85*float64(clear) {
					t.Errorf("%s, start phase %.2f, every %v s: %d calls in %d clear moments from 22 s, want at least 85%%",
						c.name, phase0, step, calls, clear)
				}
			}
		}
	}
}

func TestPhaseHoldDuringAPauseWithLandmarkNoise(t *testing.T) {
	const fps = 30.0
	noises := map[string]func(t float64, rng *rand.Rand) float64{
		"5 Hz":  func(t float64, _ *rand.Rand) float64 { return 0.0005 * math.Cos(2*math.Pi*5*t) },
		"white": func(_ float64, rng *rand.Rand) float64 { return 0.0005 * rng.NormFloat64() },
	}
	for name, noise := range noises {
		for _, rpm := range []float64{6, 10, 15} {
			rng := rand.New(rand.NewPCG(7, uint64(rpm)))
			pauseAt := 30.0
			var ts, y, w []float64
			var held float64
			for i := 0; i < int(40*fps); i++ {
				t := float64(i) / fps
				breath := 0.6 - 0.004*math.Sin(2*math.Pi*rpm/60*t)
				if t < pauseAt {
					held = breath
				} else {
					breath = held
				}
				ts = append(ts, t*1000)
				y = append(y, breath+noise(t, rng))
				w = append(w, 0.3)
			}
			prev := PhaseUnknown
			var still, holds int
			for end := 25.5; end <= 40; end += 0.5 {
				n := int(end * fps)
				from := max(0, n-int(30*fps))
				got, err := Estimate(ts[from:n], y[from:n], w[from:n], prev, DefaultConfig())
				if err == nil {
					prev = got.Phase
				}
				if end >= pauseAt+4 {
					still++
					if err == nil && got.Phase == PhaseHold {
						holds++
					}
				}
			}
			// Noise can flip a single estimate; the pause must hold for nearly all of the stretch.
			if float64(holds) < 0.9*float64(still) {
				t.Errorf("%s noise, %v rpm, still since %.0f s: hold in %d of %d estimates, want at least 90%%",
					name, rpm, pauseAt, holds, still)
			}
		}
	}
}

func TestPhaseHoldLastsThroughALongPause(t *testing.T) {
	const fps = 30.0
	for _, rpm := range []float64{6, 10, 15} {
		pauseAt := 47.4
		ts, y, w := shoulderSeries(rpm, 62, pauseAt, 0, 0, 0, 0)
		prev := PhaseUnknown
		for end := 40.0; end <= 62; end += 0.5 {
			n := int(end * fps)
			from := max(0, n-int(30*fps))
			got, err := Estimate(ts[from:n], y[from:n], w[from:n], prev, DefaultConfig())
			if err != nil {
				t.Fatalf("%v rpm at %.1f s: %v", rpm, end, err)
			}
			prev = got.Phase
			if end >= pauseAt+3 && got.Phase != PhaseHold {
				t.Errorf("%v rpm, still since %.1f s, at %.1f s: got %v, want hold", rpm, pauseAt, end, got.Phase)
			}
		}
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
