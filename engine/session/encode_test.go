package session

import (
	"math"
	"testing"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/quality"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/resp"
)

func TestEncodeLayout(t *testing.T) {
	e := Estimate{
		TimeMs: 1234, HRBpm: 72.5, RRRpm: math.NaN(), Phase: resp.PhaseOut,
		Quality: 0.8, Confidence: quality.ConfidenceMedium, Reason: quality.ReasonNone,
		Pulse: []float64{0.1, -0.2, 0.3},
	}
	out := make([]float64, OutMaxLen)
	n := Encode(e, out)
	if n != OutPulse+3 {
		t.Fatalf("n = %d, want %d", n, OutPulse+3)
	}
	want := map[int]float64{
		OutTimeMs: 1234, OutHR: 72.5, OutPhase: 2, OutQuality: 0.8, OutConfidence: 1, OutReason: 0,
		OutPulseLen: 3, OutPulse: 0.1, OutPulse + 1: -0.2, OutPulse + 2: 0.3,
	}
	for i, v := range want {
		if out[i] != v {
			t.Errorf("out[%d] = %v, want %v", i, out[i], v)
		}
	}
	if !math.IsNaN(out[OutRR]) {
		t.Errorf("out[OutRR] = %v, want NaN", out[OutRR])
	}
	if OutMaxLen != OutPulse+MaxPulseLen || MaxPulseLen != 150 {
		t.Errorf("OutMaxLen = %d, MaxPulseLen = %d", OutMaxLen, MaxPulseLen)
	}
}

func TestEncodeCodes(t *testing.T) {
	out := make([]float64, OutMaxLen)
	Encode(Estimate{Phase: resp.PhaseHold, Confidence: quality.ConfidenceInvalid, Reason: quality.ReasonWarmingUp}, out)
	if out[OutPhase] != 3 || out[OutConfidence] != 3 || out[OutReason] != 5 || out[OutPulseLen] != 0 {
		t.Fatalf("codes = %v", out[:OutPulse])
	}
}

func TestEncodeClampsPulse(t *testing.T) {
	pulse := make([]float64, MaxPulseLen+20)
	for i := range pulse {
		pulse[i] = float64(i)
	}
	out := make([]float64, OutMaxLen)
	if n := Encode(Estimate{Pulse: pulse}, out); n != OutMaxLen || out[OutPulseLen] != MaxPulseLen {
		t.Fatalf("n = %d, len = %v, want %d, %d", n, out[OutPulseLen], OutMaxLen, MaxPulseLen)
	}
	if out[OutPulse] != 20 || out[OutMaxLen-1] != float64(len(pulse)-1) {
		t.Fatalf("pulse kept %v..%v, want the last %d values", out[OutPulse], out[OutMaxLen-1], MaxPulseLen)
	}

	short := make([]float64, OutPulse+2)
	if n := Encode(Estimate{Pulse: pulse}, short); n != OutPulse+2 || short[OutPulseLen] != 2 {
		t.Fatalf("short buffer: n = %d, len = %v", n, short[OutPulseLen])
	}
	if n := Encode(Estimate{}, make([]float64, OutPulse-1)); n != 0 {
		t.Fatalf("buffer below header: n = %d, want 0", n)
	}
}
