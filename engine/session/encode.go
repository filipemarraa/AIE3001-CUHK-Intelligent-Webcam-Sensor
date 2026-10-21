package session

// Output layout, shared with packages/vitals-web/src/engine/layout.ts. OutPulse is the start of the pulse values.
const (
	OutTimeMs = iota
	OutHR
	OutRR
	OutPhase
	OutQuality
	OutConfidence
	OutReason
	OutPulseLen
	OutPulse
)

const OutMaxLen = OutPulse + MaxPulseLen

// Encode writes e into out and returns the number of values written. When out is too short for the whole pulse, the
// most recent values are kept; when it is too short for the header, nothing is written.
func Encode(e Estimate, out []float64) int {
	if len(out) < OutPulse {
		return 0
	}
	out[OutTimeMs] = e.TimeMs
	out[OutHR] = e.HRBpm
	out[OutRR] = e.RRRpm
	out[OutPhase] = float64(e.Phase)
	out[OutQuality] = e.Quality
	out[OutConfidence] = float64(e.Confidence)
	out[OutReason] = float64(e.Reason)
	pulse := e.Pulse[max(0, len(e.Pulse)-min(MaxPulseLen, len(out)-OutPulse)):]
	out[OutPulseLen] = float64(len(pulse))
	return OutPulse + copy(out[OutPulse:], pulse)
}
