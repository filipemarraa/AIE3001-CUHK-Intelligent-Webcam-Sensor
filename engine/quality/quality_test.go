package quality

import (
	"os"
	"strings"
	"testing"
)

func good() Inputs {
	return Inputs{FaceFraction: 1, MotionP90: 0.1, LightMean: 130, MouthStd: 0.005, JitterMs: 3, SNRdB: 12, HasHR: true}
}

func TestAssessReasonPriority(t *testing.T) {
	tests := []struct {
		name   string
		mod    func(*Inputs)
		reason Reason
	}{
		{"no face beats motion", func(i *Inputs) { i.FaceFraction = 0.5; i.MotionP90 = 3 }, ReasonNoFace},
		{"motion beats low light", func(i *Inputs) { i.MotionP90 = 3; i.LightMean = 10 }, ReasonMotion},
		{"low light beats talking", func(i *Inputs) { i.LightMean = 10; i.MouthStd = 0.1 }, ReasonLowLight},
		{"talking beats warming up", func(i *Inputs) { i.MouthStd = 0.1; i.WarmingUp = true }, ReasonTalking},
		{"warming up", func(i *Inputs) { i.WarmingUp = true }, ReasonWarmingUp},
		{"no HR, nothing else", func(i *Inputs) { i.HasHR = false }, ReasonNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := good()
			tt.mod(&in)
			got := Assess(in)
			if got.Reason != tt.reason {
				t.Errorf("reason = %v, want %v", got.Reason, tt.reason)
			}
			if got.Confidence != ConfidenceInvalid {
				t.Errorf("confidence = %v, want invalid", got.Confidence)
			}
		})
	}
}

func TestBreathingBlocked(t *testing.T) {
	tests := []struct {
		name    string
		mod     func(*Inputs)
		blocked bool
	}{
		{"good", func(*Inputs) {}, false},
		{"warming up", func(i *Inputs) { i.WarmingUp = true; i.HasHR = false }, false},
		{"dark", func(i *Inputs) { i.LightMean = 10 }, false},
		{"no face", func(i *Inputs) { i.FaceFraction = 0.5 }, true},
		{"moving", func(i *Inputs) { i.MotionP90 = 3 }, true},
		{"talking in the dark", func(i *Inputs) { i.LightMean = 10; i.MouthStd = 0.1 }, true},
	}
	for _, tt := range tests {
		in := good()
		tt.mod(&in)
		if got := BreathingBlocked(in); got != tt.blocked {
			t.Errorf("%s: blocked = %v, want %v", tt.name, got, tt.blocked)
		}
	}
}

func TestAssessGoodSignalIsHigh(t *testing.T) {
	got := Assess(good())
	if got.Confidence != ConfidenceHigh || got.Reason != ReasonNone || got.Quality < 0.75 {
		t.Errorf("got %+v", got)
	}
}

func TestAssessLowSNRLowersConfidence(t *testing.T) {
	in := good()
	in.SNRdB = 2
	got := Assess(in)
	if got.Confidence != ConfidenceLow && got.Confidence != ConfidenceMedium {
		t.Errorf("confidence = %v, want low or medium", got.Confidence)
	}
}

func TestAssessQualityInUnitRange(t *testing.T) {
	for _, snr := range []float64{-20, 0, 5, 50} {
		for _, light := range []float64{0, 40, 255} {
			for _, motion := range []float64{0, 0.05, 1} {
				for _, jitter := range []float64{0, 20, 500} {
					in := Inputs{FaceFraction: 1, MotionP90: motion, LightMean: light, JitterMs: jitter, SNRdB: snr, HasHR: true}
					if q := Assess(in).Quality; q < 0 || q > 1 {
						t.Errorf("quality %v out of range for %+v", q, in)
					}
				}
			}
		}
	}
}

func TestStringsMatchTypeScript(t *testing.T) {
	src, err := os.ReadFile("../../packages/vitals-web/src/types.ts")
	if err != nil {
		t.Fatal(err)
	}
	lines := map[string]string{}
	for _, l := range strings.Split(string(src), "\n") {
		switch {
		case strings.HasPrefix(l, "export type Confidence"):
			lines["Confidence"] = l
		case strings.HasPrefix(l, "export type Reason"):
			lines["Reason"] = l
		}
	}
	for _, c := range []Confidence{ConfidenceHigh, ConfidenceMedium, ConfidenceLow, ConfidenceInvalid} {
		if !strings.Contains(lines["Confidence"], "'"+c.String()+"'") {
			t.Errorf("confidence %q missing in types.ts", c.String())
		}
	}
	for _, r := range []Reason{ReasonNoFace, ReasonMotion, ReasonLowLight, ReasonTalking, ReasonWarmingUp} {
		if !strings.Contains(lines["Reason"], "'"+r.String()+"'") {
			t.Errorf("reason %q missing in types.ts", r.String())
		}
	}
	if ReasonNone.String() != "" {
		t.Errorf("ReasonNone = %q, want empty", ReasonNone.String())
	}
}
