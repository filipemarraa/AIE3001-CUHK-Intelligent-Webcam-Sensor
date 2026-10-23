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

// A reference reading is matched with the first valid estimate at most this long after it.
const referenceLagMs = 2000

// Recording is a session saved by the lab page as JSON lines, version 1: a header, then samples (the engine layout
// plus the colour of both skin regions), estimates, diagnostics and reference heart rates read from a device such as
// a watch. Estimates and diagnostics are kept in the file for reading by hand; the replay recomputes the estimates.
type Recording struct {
	Subject, Condition string
	Samples            []RecordedSample
	References         []Reference
}

type RecordedSample struct {
	Values [session.SampleLen]float64
	Skins  [3 * len(SkinNames)]float64
}

type Reference struct {
	TimeMs, HRBpm float64
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
			rec.References = append(rec.References, Reference{TimeMs: *l.TimeMs, HRBpm: *l.HRBpm})
		case "estimate", "diagnostics":
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

type SNRBin struct {
	FromDB, ToDB float64
	Readings     int
	MAE          float64
}

// SkinResult scores the heart rate of one skin region against the references of a recording.
type SkinResult struct {
	Skin                string
	References, Covered int
	MAE, P95            float64
	// ConfidentMAE counts only readings of high or medium confidence.
	ConfidentMAE float64
	MeanSNRdB    float64
	// FirstHRS is the time from the first sample to the first heart rate.
	FirstHRS float64
	// BySNR splits the matched readings by SNR, to choose a minimum SNR for reporting a heart rate.
	BySNR []SNRBin
}

var snrEdges = []float64{math.Inf(-1), 0, 3, 6, math.Inf(1)}

// CompareSkins replays the recording once per skin region, through the same session as the browser.
func CompareSkins(rec Recording, cfg session.Config) []SkinResult {
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

func compareSkin(rec Recording, k int, name string, cfg session.Config) SkinResult {
	res := SkinResult{Skin: name, References: len(rec.References), FirstHRS: math.NaN()}
	readings := replay(rec, k, cfg)
	for _, r := range readings {
		if !math.IsNaN(r.hr) {
			res.FirstHRS = (r.timeMs - rec.Samples[0].Values[session.FieldTimeMs]) / 1000
			break
		}
	}
	var errs, confidentErrs, snrs []float64
	bins := make([][]float64, len(snrEdges)-1)
	for _, ref := range rec.References {
		r, ok := matching(readings, ref.TimeMs)
		if !ok {
			continue
		}
		e := math.Abs(r.hr - ref.HRBpm)
		errs = append(errs, e)
		if r.confidence == quality.ConfidenceHigh || r.confidence == quality.ConfidenceMedium {
			confidentErrs = append(confidentErrs, e)
		}
		if !math.IsNaN(r.snr) {
			snrs = append(snrs, r.snr)
			for b := range bins {
				if r.snr >= snrEdges[b] && r.snr < snrEdges[b+1] {
					bins[b] = append(bins[b], e)
				}
			}
		}
	}
	res.Covered = len(errs)
	res.MAE, res.P95, res.ConfidentMAE, res.MeanSNRdB = mean(errs), percentile(errs, 0.95), mean(confidentErrs), mean(snrs)
	for b, e := range bins {
		res.BySNR = append(res.BySNR, SNRBin{FromDB: snrEdges[b], ToDB: snrEdges[b+1], Readings: len(e), MAE: mean(e)})
	}
	return res
}

// replay asks for an estimate at every whole second after the first sample, as the worker does.
func replay(rec Recording, k int, cfg session.Config) []reading {
	if len(rec.Samples) == 0 {
		return nil
	}
	s := session.New(cfg)
	next := rec.Samples[0].Values[session.FieldTimeMs] + 1000
	var out []reading
	for _, x := range rec.Samples {
		sample, err := session.SampleFromSlice(withSkin(x, k))
		if err != nil {
			continue
		}
		for sample.TimeMs >= next {
			e := s.Estimate(next)
			out = append(out, reading{timeMs: next, hr: e.HRBpm, snr: e.SNRdB, confidence: e.Confidence})
			next += 1000
		}
		s.Push(sample)
	}
	return out
}

// withSkin puts the colour of skin region k in the sample. A region without pixels makes the sample faceless, as the
// worker does when the skin region it reads is empty.
func withSkin(x RecordedSample, k int) []float64 {
	v := x.Values
	r, g, b := x.Skins[3*k], x.Skins[3*k+1], x.Skins[3*k+2]
	if r == 0 && g == 0 && b == 0 {
		v[session.FieldFace], v[session.FieldR], v[session.FieldG], v[session.FieldB] = 0, 0, 0, 0
		v[session.FieldLight], v[session.FieldMouth], v[session.FieldMotion] = 0, 0, 0
		return v[:]
	}
	v[session.FieldR], v[session.FieldG], v[session.FieldB] = r, g, b
	v[session.FieldLight] = 0.299*r + 0.587*g + 0.114*b
	return v[:]
}

func matching(readings []reading, refMs float64) (reading, bool) {
	for _, r := range readings {
		if r.timeMs >= refMs && r.timeMs <= refMs+referenceLagMs && r.confidence != quality.ConfidenceInvalid {
			return r, true
		}
	}
	return reading{}, false
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
