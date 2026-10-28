package session

import (
	"cmp"
	"math"
	"slices"
)

const (
	// TrackWindowS is how far back the tracker looks: each estimate already covers a 15 s window, so a few seconds of
	// them are enough to outvote a single bad one.
	TrackWindowS = 5
	// MaxSlewBPMPerS is the fastest change the tracker follows; a heart does not jump faster than this at rest.
	MaxSlewBPMPerS = 10
	// TrackGapS is how long without a valid estimate the tracker keeps its value before it starts over.
	TrackGapS = 3
)

type reading struct {
	ms, bpm, weight float64
}

// tracker smooths the heart rate: an SNR-weighted median of the recent estimates, limited to MaxSlewBPMPerS.
type tracker struct {
	recent []reading
	sorted []reading
	bpm    float64
	lastMs float64
}

func newTracker() *tracker {
	tr := &tracker{}
	tr.reset()
	return tr
}

func (tr *tracker) reset() {
	tr.recent = tr.recent[:0]
	tr.bpm = math.NaN()
	tr.lastMs = math.Inf(-1)
}

// prior is the tracked rate at nowMs, or NaN when there is none or it is stale.
func (tr *tracker) prior(nowMs float64) float64 {
	if nowMs-tr.lastMs > TrackGapS*1000 {
		return math.NaN()
	}
	return tr.bpm
}

func (tr *tracker) update(nowMs, bpm, snrDB float64) float64 {
	if math.IsNaN(tr.prior(nowMs)) {
		tr.reset()
	}
	tr.recent = append(tr.recent, reading{ms: nowMs, bpm: bpm, weight: math.Pow(10, snrDB/10)})
	old := 0
	for old < len(tr.recent) && tr.recent[old].ms < nowMs-TrackWindowS*1000 {
		old++
	}
	tr.recent = slices.Delete(tr.recent, 0, old)

	target := tr.median()
	if math.IsNaN(tr.bpm) {
		tr.bpm = target
	} else {
		step := MaxSlewBPMPerS * max(0, nowMs-tr.lastMs) / 1000
		tr.bpm += max(-step, min(step, target-tr.bpm))
	}
	tr.lastMs = nowMs
	return tr.bpm
}

func (tr *tracker) median() float64 {
	tr.sorted = append(tr.sorted[:0], tr.recent...)
	slices.SortFunc(tr.sorted, func(a, b reading) int { return cmp.Compare(a.bpm, b.bpm) })
	var total float64
	for _, r := range tr.sorted {
		total += r.weight
	}
	var acc float64
	for _, r := range tr.sorted {
		if acc += r.weight; acc >= total/2 {
			return r.bpm
		}
	}
	return tr.sorted[len(tr.sorted)-1].bpm
}
