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
