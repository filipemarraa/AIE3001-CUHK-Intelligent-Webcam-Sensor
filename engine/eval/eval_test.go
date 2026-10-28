package eval

import (
	"math"
	"testing"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/quality"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/resp"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/session"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/sim"
)

var nan = math.NaN()

func TestSummarizeHeartRate(t *testing.T) {
	pts := []Point{
		{TimeS: 5, HR: nan, RR: nan, Confidence: quality.ConfidenceInvalid, TrueHR: 70},
		{TimeS: 12, HR: 72, RR: nan, Confidence: quality.ConfidenceHigh, TrueHR: 70},
		{TimeS: 13, HR: 64, RR: nan, Confidence: quality.ConfidenceMedium, TrueHR: 70},
		{TimeS: 14, HR: 71, RR: nan, Confidence: quality.ConfidenceLow, TrueHR: 70},
		{TimeS: 15, HR: nan, RR: nan, Confidence: quality.ConfidenceInvalid, TrueHR: 70},
	}
	m := Summarize(pts, 12, 30)
	if m.HRCoverage != 0.75 {
		t.Errorf("coverage = %v, want 0.75", m.HRCoverage)
	}
	if math.Abs(m.HRMAE-3) > 1e-9 {
		t.Errorf("MAE = %v, want 3", m.HRMAE)
	}
	if m.FirstHRS != 12 {
		t.Errorf("first HR at %v, want 12", m.FirstHRS)
	}
	if math.Abs(m.HRFalseConfident-0.5) > 1e-9 {
		t.Errorf("false confident = %v, want 0.5 (1 of 2 high/medium off by more than 5)", m.HRFalseConfident)
	}
	if m.HRP95 != 6 {
		t.Errorf("P95 = %v, want 6", m.HRP95)
	}
}

func TestSummarizeCountsHarmonicErrors(t *testing.T) {
	pts := []Point{
		{TimeS: 12, HR: 120, TrueHR: 60},
		{TimeS: 13, HR: 75, TrueHR: 150},
		{TimeS: 14, HR: 62, TrueHR: 60},
		{TimeS: 15, HR: 90, TrueHR: 60},
		{TimeS: 16, HR: nan, TrueHR: 60},
	}
	m := Summarize(pts, 12, 30)
	if m.HRHarmonic != 0.5 {
		t.Fatalf("harmonic error rate = %v, want 0.5 (double and half of 4 readings)", m.HRHarmonic)
	}
}

func TestSummarizeBreathingAndPhase(t *testing.T) {
	pts := []Point{
		{TimeS: 30, HR: 70, RR: 6.5, Phase: resp.PhaseIn, TrueHR: 70, TrueRR: 6, Inhaling: true, PhaseClear: true},
		{TimeS: 31, HR: 70, RR: 5, Phase: resp.PhaseIn, TrueHR: 70, TrueRR: 6, Inhaling: false, PhaseClear: true},
		{TimeS: 32, HR: 70, RR: nan, Phase: resp.PhaseHold, TrueHR: 70, TrueRR: 6, Inhaling: false, PhaseClear: true},
		{TimeS: 33, HR: 70, RR: 6, Phase: resp.PhaseOut, TrueHR: 70, TrueRR: 6, Inhaling: false, PhaseClear: false},
	}
	m := Summarize(pts, 0, 30)
	if m.RRCoverage != 0.75 || math.Abs(m.RRMAE-0.5) > 1e-9 {
		t.Errorf("RR coverage %v MAE %v, want 0.75 and 0.5", m.RRCoverage, m.RRMAE)
	}
	if math.Abs(m.PhaseCallRate-2.0/3) > 1e-9 || m.PhaseAgreement != 0.5 {
		t.Errorf("phase calls %v agreement %v, want 2/3 and 0.5", m.PhaseCallRate, m.PhaseAgreement)
	}
}

func TestSummarizeWithoutValuesIsNaN(t *testing.T) {
	m := Summarize([]Point{{TimeS: 20, HR: nan, RR: nan, TrueHR: 70, TrueRR: 6}}, 0, 0)
	if m.HRCoverage != 0 || !math.IsNaN(m.HRMAE) || !math.IsNaN(m.FirstHRS) || !math.IsNaN(m.RRMAE) {
		t.Fatalf("got %+v, want zero coverage and NaN errors", m)
	}
}

func TestMeanIgnoresNaN(t *testing.T) {
	got := Mean([]Metrics{{Scenario: "a", HRMAE: 2, RRMAE: nan}, {Scenario: "a", HRMAE: 4, RRMAE: 1}})
	if got.HRMAE != 3 || got.RRMAE != 1 || got.Scenario != "a" {
		t.Fatalf("got %+v", got)
	}
}

func TestRunScenarioEmitsOnePointPerSecond(t *testing.T) {
	sc := sim.Base("ideal")
	sc.DurationS = 40
	pts := RunScenario(sc, session.DefaultConfig())
	if len(pts) < 38 || len(pts) > 40 {
		t.Fatalf("got %d points for 40 s, want about 39", len(pts))
	}
	m := Summarize(pts, 12, 27)
	if m.HRCoverage < 0.9 || m.HRMAE > 2 {
		t.Fatalf("ideal scenario: HR coverage %.2f MAE %.2f", m.HRCoverage, m.HRMAE)
	}
}

func scenario(t *testing.T, name string) sim.Scenario {
	t.Helper()
	for _, sc := range sim.Suite() {
		if sc.Name == name {
			return sc
		}
	}
	t.Fatalf("no scenario %q", name)
	return sim.Scenario{}
}

// worstAfter is the largest heart-rate error from fromS on, with every second expected to have a reading.
func worstAfter(t *testing.T, pts []Point, fromS float64) float64 {
	t.Helper()
	var worst float64
	for _, p := range pts {
		if p.TimeS < fromS {
			continue
		}
		if math.IsNaN(p.HR) {
			t.Fatalf("no heart rate at %.0f s", p.TimeS)
		}
		worst = max(worst, math.Abs(p.HR-p.TrueHR))
	}
	return worst
}

// longestWrong is the longest run of seconds, from 12 s on, with the heart rate more than 5 bpm off.
func longestWrong(pts []Point) int {
	run, longest := 0, 0
	for _, p := range pts {
		if p.TimeS < 12 {
			continue
		}
		if math.IsNaN(p.HR) || math.Abs(p.HR-p.TrueHR) > 5 {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return longest
}

// The pulse has 0.69 of its harmonic's power, so either can hold the tracker; neither may hold it for long.
func TestStrongHarmonicDoesNotCaptureTheTracker(t *testing.T) {
	sc := scenario(t, "harmonic_1.2_55bpm")
	for seed := uint64(1); seed <= 3; seed++ {
		sc.Seed = seed
		if run := longestWrong(RunScenario(sc, session.DefaultConfig())); run > 15 {
			t.Errorf("seed %d: wrong for %d s in a row, want at most 15", seed, run)
		}
	}
}

func TestTrackerLetsGoOfAHarmonicThatFades(t *testing.T) {
	for _, name := range []string{"harmonic_fades_50bpm", "harmonic_fades_60bpm"} {
		sc := scenario(t, name)
		for seed := uint64(1); seed <= 3; seed++ {
			sc.Seed = seed
			// The harmonic fades at 30 s; the reading must follow within 15 s.
			if worst := worstAfter(t, RunScenario(sc, session.DefaultConfig()), 45); worst > 5 {
				t.Errorf("%s seed %d: worst error %.1f bpm from 45 s on", name, seed, worst)
			}
		}
	}
}
