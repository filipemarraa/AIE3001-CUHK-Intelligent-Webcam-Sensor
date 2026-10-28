package session

import "testing"

// BenchmarkEstimate is the cost of one estimate on a full 30 s buffer; the worker asks for one per frame it accepts.
func BenchmarkEstimate(b *testing.B) {
	s := New(DefaultConfig())
	now := pushAll(s, synthSamples(72, 6, 30, 30))
	for b.Loop() {
		s.Estimate(now)
	}
}
