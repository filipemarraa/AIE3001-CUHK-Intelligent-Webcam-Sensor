package session

import (
	"math"
	"testing"
)

func feed(tr *tracker, startMs float64, bpms, snrs []float64) float64 {
	var got float64
	for i, bpm := range bpms {
		got = tr.update(startMs+1000*float64(i), bpm, snrs[i])
	}
	return got
}

func TestTrackerStartsAtTheFirstReading(t *testing.T) {
	tr := newTracker()
	if !math.IsNaN(tr.prior(0)) {
		t.Fatalf("prior %v before any reading, want NaN", tr.prior(0))
	}
	if got := tr.update(0, 64, 3); got != 64 {
		t.Fatalf("first tracked value %v, want 64", got)
	}
	if tr.prior(1000) != 64 {
		t.Fatalf("prior %v, want 64", tr.prior(1000))
	}
}

func TestTrackerOutvotesAWeakOutlier(t *testing.T) {
	tr := newTracker()
	got := feed(tr, 0, []float64{70, 71, 70, 140, 71}, []float64{6, 6, 6, 0, 6})
	if math.Abs(got-70.5) > 1 {
		t.Fatalf("tracked %v after a weak 140 bpm reading, want about 70.5", got)
	}
}

func TestTrackerOutvotesAStrongSingleOutlier(t *testing.T) {
	tr := newTracker()
	got := feed(tr, 0, []float64{70, 71, 70, 71, 140}, []float64{3, 3, 3, 3, 9})
	if got > 72 {
		t.Fatalf("tracked %v after one 140 bpm reading, want the earlier 70 to 71", got)
	}
}

func TestTrackerLimitsTheRateOfChange(t *testing.T) {
	tr := newTracker()
	feed(tr, 0, []float64{60, 60, 60, 60, 60}, []float64{6, 6, 6, 6, 6})
	prev := 60.0
	for i := 5; i < 20; i++ {
		got := tr.update(1000*float64(i), 120, 6)
		if got-prev > MaxSlewBPMPerS+1e-9 {
			t.Fatalf("t=%d s: tracked moved %v bpm in 1 s, want at most %v", i, got-prev, MaxSlewBPMPerS)
		}
		prev = got
	}
	if prev != 120 {
		t.Fatalf("tracked %v after 15 s of 120 bpm, want 120", prev)
	}
}

func TestTrackerForgetsAfterAGap(t *testing.T) {
	tr := newTracker()
	feed(tr, 0, []float64{60, 60, 60}, []float64{6, 6, 6})
	gapMs := 2000 + 1000*float64(TrackGapS) + 1
	if !math.IsNaN(tr.prior(gapMs)) {
		t.Fatalf("prior %v after a gap, want NaN", tr.prior(gapMs))
	}
	if got := tr.update(gapMs, 110, 6); got != 110 {
		t.Fatalf("tracked %v after a gap, want the new 110 at once", got)
	}
}

func TestTrackerReset(t *testing.T) {
	tr := newTracker()
	feed(tr, 0, []float64{60, 60}, []float64{6, 6})
	tr.reset()
	if !math.IsNaN(tr.prior(2000)) {
		t.Fatal("prior survives reset")
	}
}
