package session

import (
	"errors"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/quality"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/resp"
)

var (
	lightSkin        = [3]float64{180, 140, 120}
	bloodVolumePulse = [3]float64{0.33, 0.77, 0.53}
)

// synthSamples uses the colour model of engine/rppg/synth_test.go and the shoulder model of engine/resp/resp_test.go.
func synthSamples(bpm, rpm, durationS, fps float64) []Sample {
	rng := rand.New(rand.NewPCG(1, 2))
	n := int(durationS * fps)
	xs := make([]Sample, n)
	for i := range xs {
		t := float64(i)/fps + 0.008*(rng.Float64()-0.5)
		pulse := math.Sin(2*math.Pi*bpm/60*t) + 0.4*math.Sin(4*math.Pi*bpm/60*t+0.5)
		light := 1 + 0.02*math.Sin(2*math.Pi*0.05*t)
		var c [3]float64
		for k := range c {
			c[k] = lightSkin[k]*light*(1+0.008*bloodVolumePulse[k]*pulse) + 0.1*rng.NormFloat64()
		}
		xs[i] = Sample{
			TimeMs: t * 1000, R: c[0], G: c[1], B: c[2],
			Face: true, Motion: 0.01, Light: 130, Mouth: 0.02,
			ShoulderY: 0.6 - 0.004*math.Sin(2*math.Pi*rpm/60*t), ShoulderW: 0.3,
		}
	}
	return xs
}

func pushAll(s *Session, xs []Sample) float64 {
	for _, x := range xs {
		s.Push(x)
	}
	return xs[len(xs)-1].TimeMs
}

func until(xs []Sample, ms float64) []Sample {
	for i, x := range xs {
		if x.TimeMs >= ms {
			return xs[:i]
		}
	}
	return xs
}

func assertInvalid(t *testing.T, e Estimate, reason quality.Reason) {
	t.Helper()
	if e.Confidence != quality.ConfidenceInvalid || e.Reason != reason {
		t.Fatalf("confidence %v reason %v, want invalid %v", e.Confidence, e.Reason, reason)
	}
	if !math.IsNaN(e.HRBpm) || !math.IsNaN(e.RRRpm) || e.Phase != resp.PhaseUnknown {
		t.Fatalf("HR %v RR %v phase %v, want NaN, NaN, unknown", e.HRBpm, e.RRRpm, e.Phase)
	}
}

func assertHeartRate(t *testing.T, e Estimate, bpm float64) {
	t.Helper()
	if e.Confidence == quality.ConfidenceInvalid {
		t.Fatalf("confidence invalid, reason %v", e.Reason)
	}
	if math.Abs(e.HRBpm-bpm) > 2 {
		t.Fatalf("HR = %.2f, want %v ± 2", e.HRBpm, bpm)
	}
}

func TestWarmupInvalid(t *testing.T) {
	s := New(DefaultConfig())
	pushAll(s, synthSamples(72, 12, 5, 30))
	assertInvalid(t, s.Estimate(5000), quality.ReasonWarmingUp)
}

func TestHeartRateAfterWarmup(t *testing.T) {
	s := New(DefaultConfig())
	now := pushAll(s, synthSamples(72, 12, 15, 30))
	e := s.Estimate(now)
	assertHeartRate(t, e, 72)
	if len(e.Pulse) == 0 || len(e.Pulse) > MaxPulseLen {
		t.Fatalf("len(Pulse) = %d, want 1..%d", len(e.Pulse), MaxPulseLen)
	}
	if e.TimeMs != now {
		t.Fatalf("TimeMs = %v, want %v", e.TimeMs, now)
	}
}

func TestBreathingAt6RPMAcrossWindows(t *testing.T) {
	xs := synthSamples(72, 6, 60, 30)
	s := New(DefaultConfig())

	pushed := len(until(xs, 20000))
	e := s.Estimate(pushAll(s, xs[:pushed]))
	assertHeartRate(t, e, 72)
	if !math.IsNaN(e.RRRpm) || e.Phase != resp.PhaseUnknown {
		t.Fatalf("at 20 s: RR %v phase %v, want NaN, unknown", e.RRRpm, e.Phase)
	}

	for endMs := 26000.0; endMs <= 60000; endMs += 500 {
		next := len(until(xs, endMs))
		pushAll(s, xs[pushed:next])
		pushed = next
		e := s.Estimate(endMs)
		if math.Abs(e.RRRpm-6) > 1 || e.Phase == resp.PhaseUnknown {
			t.Errorf("at %.1f s: RR %.2f phase %v, want 6 ± 1 and a known phase", endMs/1000, e.RRRpm, e.Phase)
		}
	}
}

func TestShouldersWithoutBreathingLeaveRRUnknown(t *testing.T) {
	xs := synthSamples(72, 6, 40, 30)
	rng := rand.New(rand.NewPCG(3, 4))
	for i := range xs {
		xs[i].ShoulderY = 0.6 + 0.001*rng.NormFloat64()
	}
	s := New(DefaultConfig())
	e := s.Estimate(pushAll(s, xs))
	assertHeartRate(t, e, 72)
	if !math.IsNaN(e.RRRpm) || e.Phase != resp.PhaseUnknown {
		t.Fatalf("RR %v phase %v from shoulders without breathing, want NaN and unknown", e.RRRpm, e.Phase)
	}
}

func TestHeartRateFailureAfterWarmupGivesAReason(t *testing.T) {
	xs := synthSamples(72, 6, 20, 30)
	for i := range xs {
		xs[i].R, xs[i].G, xs[i].B = 150, 110, 90
	}
	s := New(DefaultConfig())
	assertInvalid(t, s.Estimate(pushAll(s, xs)), quality.ReasonWarmingUp)
}

func TestBreathingHasItsOwnConfidence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		spoil   func(x *Sample)
		reason  quality.Reason
		visible bool
	}{
		{"no pulse", func(x *Sample) { x.R, x.G, x.B = 150, 110, 90 }, quality.ReasonWarmingUp, true},
		{"dark room", func(x *Sample) { x.R, x.G, x.B, x.Light = 12, 8, 6, 15 }, quality.ReasonLowLight, true},
		{"no face", func(x *Sample) { x.Face, x.R, x.G, x.B, x.Light = false, 0, 0, 0, 0 }, quality.ReasonNoFace, false},
		{"moving", func(x *Sample) { x.Motion = 0.2 }, quality.ReasonMotion, false},
		{"talking", func(x *Sample) { x.Mouth = 0.02 + 0.1*math.Sin(x.TimeMs/150) }, quality.ReasonTalking, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			xs := synthSamples(72, 10, 40, 30)
			for i := range xs {
				tc.spoil(&xs[i])
			}
			s := New(DefaultConfig())
			e := s.Estimate(pushAll(s, xs))
			if e.Confidence != quality.ConfidenceInvalid || e.Reason != tc.reason || !math.IsNaN(e.HRBpm) {
				t.Fatalf("confidence %v reason %v HR %v, want invalid %v and no HR", e.Confidence, e.Reason, e.HRBpm, tc.reason)
			}
			if !tc.visible {
				if !math.IsNaN(e.RRRpm) || e.Phase != resp.PhaseUnknown {
					t.Fatalf("RR %v phase %v, want NaN and unknown", e.RRRpm, e.Phase)
				}
				return
			}
			if math.Abs(e.RRRpm-10) > 1 || e.Phase == resp.PhaseUnknown {
				t.Fatalf("RR %.2f phase %v, want 10 ± 1 and a known phase", e.RRRpm, e.Phase)
			}
		})
	}
}

func TestEstimateCarriesTheHeartRateSNR(t *testing.T) {
	s := New(DefaultConfig())
	e := s.Estimate(pushAll(s, synthSamples(72, 6, 20, 30)))
	assertHeartRate(t, e, 72)
	if !(e.SNRdB > 3) {
		t.Fatalf("SNR %v dB on a clean pulse, want above 3", e.SNRdB)
	}
	s = New(DefaultConfig())
	if e := s.Estimate(pushAll(s, synthSamples(72, 6, 5, 30))); !math.IsNaN(e.SNRdB) {
		t.Fatalf("SNR %v dB while warming up, want NaN", e.SNRdB)
	}
}

func TestMotionIsMeasuredPerSecond(t *testing.T) {
	for _, tc := range []struct {
		fps    float64
		reason quality.Reason
	}{
		{30, quality.ReasonMotion},
		{10, quality.ReasonNone},
	} {
		xs := synthSamples(72, 6, 20, tc.fps)
		for i := range xs {
			xs[i].Motion = 0.04
		}
		s := New(DefaultConfig())
		e := s.Estimate(pushAll(s, xs))
		if e.Reason != tc.reason {
			t.Errorf("%v fps, 0.04 face widths per frame: reason %v, want %v", tc.fps, e.Reason, tc.reason)
		}
	}
}

func TestMotionBurstOnlyCostsTheBurstAndTheWarmup(t *testing.T) {
	xs := synthSamples(72, 6, 50, 30)
	for i := range xs {
		if xs[i].TimeMs >= 30000 && xs[i].TimeMs < 33000 {
			xs[i].Motion = 0.2
		}
	}
	s := New(DefaultConfig())
	pushed := len(until(xs, 32000))
	pushAll(s, xs[:pushed])
	if e := s.Estimate(32000); e.Reason != quality.ReasonMotion || !math.IsNaN(e.RRRpm) {
		t.Fatalf("during the burst: reason %v RR %v, want too_much_motion and no RR", e.Reason, e.RRRpm)
	}
	next := len(until(xs, 36000))
	pushAll(s, xs[pushed:next])
	if e := s.Estimate(36000); e.Reason != quality.ReasonWarmingUp || !math.IsNaN(e.HRBpm) || math.Abs(e.RRRpm-6) > 1 {
		t.Fatalf("after the burst: reason %v HR %v RR %v, want warming_up, no HR and RR 6 ± 1", e.Reason, e.HRBpm, e.RRRpm)
	}
	pushed, next = next, len(until(xs, 44000))
	pushAll(s, xs[pushed:next])
	assertHeartRate(t, s.Estimate(44000), 72)
}

func TestNoFaceInvalidates(t *testing.T) {
	xs := synthSamples(72, 6, 25, 30)
	for i := range xs {
		if xs[i].TimeMs >= 15000 {
			xs[i].Face = false
			xs[i].R, xs[i].G, xs[i].B, xs[i].Light, xs[i].Mouth, xs[i].Motion = 0, 0, 0, 0, 0, 0
		}
	}
	s := New(DefaultConfig())
	now := pushAll(s, xs)
	assertInvalid(t, s.Estimate(now), quality.ReasonNoFace)
}

func TestDarkRoom(t *testing.T) {
	xs := synthSamples(72, 6, 15, 30)
	dark := [3]float64{12, 8, 6}
	for i := range xs {
		xs[i].R *= dark[0] / lightSkin[0]
		xs[i].G *= dark[1] / lightSkin[1]
		xs[i].B *= dark[2] / lightSkin[2]
		xs[i].Light = 15
	}
	s := New(DefaultConfig())
	now := pushAll(s, xs)
	assertInvalid(t, s.Estimate(now), quality.ReasonLowLight)
}

func TestGapInvalidatesThenRecovers(t *testing.T) {
	var xs []Sample
	for _, x := range synthSamples(72, 6, 33, 30) {
		if x.TimeMs < 15000 || x.TimeMs >= 17000 {
			xs = append(xs, x)
		}
	}
	s := New(DefaultConfig())

	now := pushAll(s, until(xs, 22000))
	assertInvalid(t, s.Estimate(now), quality.ReasonWarmingUp)

	now = pushAll(s, xs[len(until(xs, 22000)):])
	assertHeartRate(t, s.Estimate(now), 72)
}

func TestShortStallIsBridged(t *testing.T) {
	var xs []Sample
	for _, x := range synthSamples(72, 6, 40, 30) {
		if x.TimeMs < 30000 || x.TimeMs >= 30400 {
			xs = append(xs, x)
		}
	}
	s := New(DefaultConfig())
	e := s.Estimate(pushAll(s, until(xs, 32000)))
	assertHeartRate(t, e, 72)
	if math.Abs(e.RRRpm-6) > 1 {
		t.Fatalf("RR %.2f after a 400 ms stall, want 6 ± 1", e.RRRpm)
	}
}

func TestStaleSamplesInvalidate(t *testing.T) {
	s := New(DefaultConfig())
	now := pushAll(s, synthSamples(72, 6, 25, 30))
	assertHeartRate(t, s.Estimate(now), 72)
	assertInvalid(t, s.Estimate(now+1000), quality.ReasonWarmingUp)
	assertInvalid(t, s.Estimate(now+60000), quality.ReasonWarmingUp)
}

func TestPushDropsNonIncreasingAndNonFiniteSamples(t *testing.T) {
	xs := synthSamples(72, 6, 15, 30)
	s := New(DefaultConfig())
	for i, x := range xs {
		s.Push(x)
		if i%10 != 0 {
			continue
		}
		dup := x
		s.Push(dup)
		older := x
		older.TimeMs -= 5
		s.Push(older)
		for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			for _, field := range []*float64{
				&x.TimeMs, &x.R, &x.G, &x.B, &x.Motion, &x.Light, &x.Mouth, &x.ShoulderY, &x.ShoulderW,
			} {
				saved := *field
				*field = bad
				x.TimeMs++
				s.Push(x)
				x.TimeMs--
				*field = saved
			}
		}
	}
	if s.n != len(xs) {
		t.Fatalf("buffer holds %d samples, want %d", s.n, len(xs))
	}
	assertHeartRate(t, s.Estimate(xs[len(xs)-1].TimeMs), 72)
}

func toSlice(x Sample) []float64 {
	v := make([]float64, SampleLen)
	v[FieldTimeMs], v[FieldR], v[FieldG], v[FieldB] = x.TimeMs, x.R, x.G, x.B
	if x.Face {
		v[FieldFace] = 1
	}
	v[FieldMotion], v[FieldLight], v[FieldMouth] = x.Motion, x.Light, x.Mouth
	v[FieldShoulderY], v[FieldShoulderW] = x.ShoulderY, x.ShoulderW
	return v
}

func TestSampleFromSlice(t *testing.T) {
	x := Sample{TimeMs: 1, R: 2, G: 3, B: 4, Face: true, Motion: 5, Light: 6, Mouth: 7, ShoulderY: 8, ShoulderW: 9}
	got, err := SampleFromSlice(toSlice(x))
	if err != nil || got != x {
		t.Fatalf("got %+v, %v; want %+v", got, err, x)
	}
	if v := x.values(); !slices.Equal(v[:], toSlice(x)) {
		t.Fatalf("values() = %v, want %v", v, toSlice(x))
	}
	v := toSlice(x)
	v[FieldFace] = 0
	if got, _ := SampleFromSlice(v); got.Face {
		t.Fatal("Face = true for 0")
	}
	if _, err := SampleFromSlice(v[:SampleLen-1]); err == nil {
		t.Fatal("short slice accepted")
	}
	for f := 0; f < SampleLen; f++ {
		v := toSlice(x)
		v[f] = math.NaN()
		if _, err := SampleFromSlice(v); !errors.Is(err, ErrNotFinite) {
			t.Fatalf("NaN in field %d: err = %v, want ErrNotFinite", f, err)
		}
	}
}

func TestRingBufferBounded(t *testing.T) {
	s := New(DefaultConfig())
	xs := synthSamples(72, 6, 120, 30)
	now := pushAll(s, xs)
	capacity := len(s.buf)
	if s.n > capacity {
		t.Fatalf("n = %d above capacity %d", s.n, capacity)
	}
	assertHeartRate(t, s.Estimate(now), 72)

	x := xs[len(xs)-1]
	allocs := testing.AllocsPerRun(100, func() {
		x.TimeMs += 33
		s.Push(x)
	})
	if allocs != 0 {
		t.Fatalf("Push allocates %v times, want 0", allocs)
	}
	if len(s.buf) != capacity {
		t.Fatalf("buffer grew from %d to %d", capacity, len(s.buf))
	}
}

func TestResetClears(t *testing.T) {
	xs := synthSamples(72, 6, 15, 30)
	s := New(DefaultConfig())
	now := pushAll(s, xs)
	assertHeartRate(t, s.Estimate(now), 72)

	s.Reset()
	assertInvalid(t, s.Estimate(now), quality.ReasonWarmingUp)

	now = pushAll(s, xs)
	assertHeartRate(t, s.Estimate(now), 72)
}
