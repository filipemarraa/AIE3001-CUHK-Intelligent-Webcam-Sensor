package eval

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/quality"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/session"
)

// SkinNames is the order of the two skin colours in each recorded sample, three numbers (R, G, B) each.
var SkinNames = [2]string{"boxes", "anatomical"}

const (
	// An estimate within this many bpm of the reference counts as right.
	withinBpm = 5
	// An estimate within this many bpm of double or half the reference counts as a harmonic error.
	harmonicBpm = 5
)

// Recording is a session saved by the lab page as JSON lines, version 1: a header, then samples (the engine layout
// plus the colour of both skin regions), estimates, diagnostics and reference heart rates, read from a Bluetooth
// sensor (source "ble", about one a second) or typed from a watch (source "watch"). Estimates and diagnostics are kept in the file for reading by hand; the replay recomputes the estimates.
type Recording struct {
	Subject, Condition string
	Samples            []RecordedSample
	References         []Reference
	// EstimateTimes are the moments the worker asked for an estimate; the replay asks at the same ones.
	EstimateTimes []float64
}

type RecordedSample struct {
	Values [session.SampleLen]float64
	Skins  [3 * len(SkinNames)]float64
}

type Reference struct {
	TimeMs, HRBpm float64
	Source        string
}

type line struct {
	Kind      string    `json:"kind"`
	Version   int       `json:"version"`
	Subject   string    `json:"subject"`
	Condition string    `json:"condition"`
	Regions   []string  `json:"regions"`
	V         []float64 `json:"v"`
	Skins     []float64 `json:"skins"`
	TimeMs    *float64  `json:"tMs"`
	HRBpm     *float64  `json:"hrBpm"`
	Source    string    `json:"source"`
	E         *struct {
		TimeMs *float64 `json:"tMs"`
	} `json:"e"`
}

func ReadRecording(r io.Reader) (Recording, error) {
	var rec Recording
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	header := false
	n := 0
	for sc.Scan() {
		n++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var l line
		if err := json.Unmarshal([]byte(text), &l); err != nil {
			return rec, fmt.Errorf("line %d: %w", n, err)
		}
		if !header {
			if l.Kind != "header" {
				return rec, fmt.Errorf("line %d: the first line must be the header", n)
			}
			if l.Version != 1 {
				return rec, fmt.Errorf("line %d: version %d, want 1", n, l.Version)
			}
			if len(l.Regions) > 0 && !slices.Equal(l.Regions, SkinNames[:]) {
				return rec, fmt.Errorf("line %d: regions %v, want %v", n, l.Regions, SkinNames)
			}
			header = true
			rec.Subject, rec.Condition = l.Subject, l.Condition
			continue
		}
		switch l.Kind {
		case "sample":
			if len(l.V) != session.SampleLen || len(l.Skins) != 3*len(SkinNames) {
				return rec, fmt.Errorf("line %d: sample with %d values and %d skin colours, want %d and %d",
					n, len(l.V), len(l.Skins), session.SampleLen, 3*len(SkinNames))
			}
			var s RecordedSample
			copy(s.Values[:], l.V)
			copy(s.Skins[:], l.Skins)
			rec.Samples = append(rec.Samples, s)
		case "reference":
			if l.TimeMs == nil || l.HRBpm == nil {
				return rec, fmt.Errorf("line %d: reference without tMs or hrBpm", n)
			}
			rec.References = append(rec.References, Reference{TimeMs: *l.TimeMs, HRBpm: *l.HRBpm, Source: l.Source})
		case "estimate":
			if l.E == nil || l.E.TimeMs == nil {
				return rec, fmt.Errorf("line %d: estimate without e.tMs", n)
			}
			rec.EstimateTimes = append(rec.EstimateTimes, *l.E.TimeMs)
		case "diagnostics":
		default:
			return rec, fmt.Errorf("line %d: unknown kind %q", n, l.Kind)
		}
	}
	if err := sc.Err(); err != nil {
		return rec, fmt.Errorf("line %d: %w", n+1, err)
	}
	if !header {
		return rec, errors.New("line 1: empty recording, no header")
	}
	return rec, nil
}

// ReplayConfig sets how the estimates of a recording are scored against its reference.
type ReplayConfig struct {
	Session session.Config
	// ReferenceWindowS is the span of reference readings averaged for each estimate, ending at the estimate. The
	// engine estimates the heart rate over its last HRWindowS seconds, so by default the reference covers the same
	// ones: a single reading taken next to the estimate lags it by half a window when the heart rate changes.
	ReferenceWindowS float64
	// MinReferences is the fewest readings in that span for an estimate to be scored. A Bluetooth sensor sends one a
	// second; for a watch typed by hand every 10 to 15 s, use 1.
	MinReferences int
}

func DefaultReplayConfig() ReplayConfig {
	cfg := session.DefaultConfig()
	return ReplayConfig{Session: cfg, ReferenceWindowS: cfg.HRWindowS, MinReferences: 5}
}

// Pair is an estimate shown to the user and the reference over its window.
type Pair struct {
	EstBpm, RefBpm, SNRdB float64
	Confidence            quality.Confidence
}

// ErrorStats scores estimates against the reference, in bpm.
type ErrorStats struct {
	N              int
	MAE, RMSE, P95 float64
	// Within5 is the share of estimates within 5 bpm of the reference.
	Within5 float64
	// Bias is the mean of estimate minus reference; LoALow and LoAHigh are its Bland-Altman limits of agreement,
	// the bias plus or minus 1.96 standard deviations.
	Bias, LoALow, LoAHigh float64
	// Harmonics counts estimates near double or half the reference, the typical error of a spectral peak picker.
	Harmonics int
}

func Stats(pairs []Pair) ErrorStats {
	s := ErrorStats{N: len(pairs)}
	abs := make([]float64, 0, len(pairs))
	diffs := make([]float64, 0, len(pairs))
	var squares float64
	within := 0
	for _, p := range pairs {
		d := p.EstBpm - p.RefBpm
		diffs = append(diffs, d)
		abs = append(abs, math.Abs(d))
		squares += d * d
		if math.Abs(d) <= withinBpm {
			within++
		}
		if math.Abs(p.EstBpm-2*p.RefBpm) < harmonicBpm || math.Abs(p.EstBpm-p.RefBpm/2) < harmonicBpm {
			s.Harmonics++
		}
	}
	s.MAE, s.P95, s.Bias = mean(abs), percentile(abs, 0.95), mean(diffs)
	s.RMSE, s.Within5 = math.Sqrt(squares/float64(len(pairs))), ratio(within, len(pairs))
	sd := math.NaN()
	if len(diffs) > 1 {
		var v float64
		for _, d := range diffs {
			v += (d - s.Bias) * (d - s.Bias)
		}
		sd = math.Sqrt(v / float64(len(diffs)-1))
	}
	s.LoALow, s.LoAHigh = s.Bias-1.96*sd, s.Bias+1.96*sd
	return s
}

// SNRBand scores the estimates whose SNR falls in [FromDB, ToDB), to choose the minimum SNR for showing a heart
// rate. Share is their part of all the estimates scored.
type SNRBand struct {
	FromDB, ToDB float64
	Share        float64
	ErrorStats
}

var snrEdges = []float64{math.Inf(-1), 0, 3, 6, math.Inf(1)}

func Bands(pairs []Pair, evaluated int) []SNRBand {
	out := make([]SNRBand, 0, len(snrEdges)-1)
	for b := range len(snrEdges) - 1 {
		var in []Pair
		for _, p := range pairs {
			if p.SNRdB >= snrEdges[b] && p.SNRdB < snrEdges[b+1] {
				in = append(in, p)
			}
		}
		out = append(out, SNRBand{FromDB: snrEdges[b], ToDB: snrEdges[b+1], Share: ratio(len(in), evaluated), ErrorStats: Stats(in)})
	}
	return out
}

// SkinResult scores the heart rate of one skin region against the reference of a recording.
type SkinResult struct {
	Skin       string
	References int
	// Evaluated counts the estimates after the warm-up with enough reference readings in their window; Covered,
	// those of them that showed a heart rate.
	Evaluated, Covered int
	All                ErrorStats
	// Confident counts only estimates of high or medium confidence.
	Confident ErrorStats
	MeanSNRdB float64
	// FirstHRS is the time from the first sample to the first heart rate.
	FirstHRS float64
	BySNR    []SNRBand
	Pairs    []Pair
}

func (r SkinResult) Coverage() float64 {
	return ratio(r.Covered, r.Evaluated)
}

// CompareSkins replays the recording once per skin region, through the same session as the browser.
func CompareSkins(rec Recording, cfg ReplayConfig) []SkinResult {
	out := make([]SkinResult, 0, len(SkinNames))
	for k, name := range SkinNames {
		out = append(out, compareSkin(rec, k, name, cfg))
	}
	return out
}

type reading struct {
	timeMs, hr, snr float64
	confidence      quality.Confidence
}

func compareSkin(rec Recording, k int, name string, cfg ReplayConfig) SkinResult {
	res := SkinResult{Skin: name, References: len(rec.References), FirstHRS: math.NaN()}
	if len(rec.Samples) == 0 {
		res.All, res.Confident, res.MeanSNRdB, res.BySNR = Stats(nil), Stats(nil), math.NaN(), Bands(nil, 0)
		return res
	}
	start := rec.Samples[0].Values[session.FieldTimeMs]
	var confident []Pair
	var snrs []float64
	for _, r := range replay(rec, k, cfg.Session) {
		shown := !math.IsNaN(r.hr) && r.confidence != quality.ConfidenceInvalid
		if shown && math.IsNaN(res.FirstHRS) {
			res.FirstHRS = (r.timeMs - start) / 1000
		}
		if r.timeMs-start < cfg.Session.WarmupS*1000 {
			continue
		}
		ref, ok := referenceAt(rec.References, r.timeMs, cfg.ReferenceWindowS*1000, cfg.MinReferences)
		if !ok {
			continue
		}
		res.Evaluated++
		if !shown {
			continue
		}
		p := Pair{EstBpm: r.hr, RefBpm: ref, SNRdB: r.snr, Confidence: r.confidence}
		res.Pairs = append(res.Pairs, p)
		if p.Confidence == quality.ConfidenceHigh || p.Confidence == quality.ConfidenceMedium {
			confident = append(confident, p)
		}
		if !math.IsNaN(p.SNRdB) {
			snrs = append(snrs, p.SNRdB)
		}
	}
	res.Covered = len(res.Pairs)
	res.All, res.Confident, res.MeanSNRdB = Stats(res.Pairs), Stats(confident), mean(snrs)
	res.BySNR = Bands(res.Pairs, res.Evaluated)
	return res
}

// referenceAt is the mean of the reference readings in [tMs-windowMs, tMs], when there are at least min of them.
func referenceAt(refs []Reference, tMs, windowMs float64, min int) (float64, bool) {
	var sum float64
	n := 0
	for _, r := range refs {
		if r.TimeMs >= tMs-windowMs && r.TimeMs <= tMs {
			sum += r.HRBpm
			n++
		}
	}
	if n == 0 || n < min {
		return 0, false
	}
	return sum / float64(n), true
}

// replay pushes each sample before asking for an estimate, as the worker does: at the recorded estimate times, or,
// without them, after the first sample of every new second.
func replay(rec Recording, k int, cfg session.Config) []reading {
	s := session.New(cfg)
	var out []reading
	estimate := func(t float64) {
		e := s.Estimate(t)
		out = append(out, reading{timeMs: t, hr: e.HRBpm, snr: e.SNRdB, confidence: e.Confidence})
	}
	push := func(x RecordedSample) {
		if sample, err := session.SampleFromSlice(withSkin(x, k)); err == nil {
			s.Push(sample)
		}
	}
	if len(rec.EstimateTimes) > 0 {
		i := 0
		for _, t := range rec.EstimateTimes {
			for ; i < len(rec.Samples) && rec.Samples[i].Values[session.FieldTimeMs] <= t; i++ {
				push(rec.Samples[i])
			}
			estimate(t)
		}
		return out
	}
	next := math.Inf(-1)
	for _, x := range rec.Samples {
		push(x)
		t := x.Values[session.FieldTimeMs]
		if math.IsInf(next, -1) {
			next = (math.Floor(t/1000) + 1) * 1000
		}
		if t >= next {
			estimate(t)
			next = (math.Floor(t/1000) + 1) * 1000
		}
	}
	return out
}

// withSkin puts the colour of skin region k in the sample. Each region decides on its own whether the frame has a
// face: the one in use during the recording may have found no skin while the other did.
func withSkin(x RecordedSample, k int) []float64 {
	v := x.Values
	r, g, b := x.Skins[3*k], x.Skins[3*k+1], x.Skins[3*k+2]
	if r == 0 && g == 0 && b == 0 {
		v[session.FieldFace], v[session.FieldR], v[session.FieldG], v[session.FieldB], v[session.FieldLight] = 0, 0, 0, 0, 0
		return v[:]
	}
	v[session.FieldFace] = 1
	v[session.FieldR], v[session.FieldG], v[session.FieldB] = r, g, b
	v[session.FieldLight] = 0.299*r + 0.587*g + 0.114*b
	return v[:]
}

// FaceShare is the share of samples with a face, which must stay near zero on a recording of an empty scene.
func (rec Recording) FaceShare() float64 {
	if len(rec.Samples) == 0 {
		return math.NaN()
	}
	faces := 0
	for _, x := range rec.Samples {
		if x.Values[session.FieldFace] != 0 {
			faces++
		}
	}
	return float64(faces) / float64(len(rec.Samples))
}
