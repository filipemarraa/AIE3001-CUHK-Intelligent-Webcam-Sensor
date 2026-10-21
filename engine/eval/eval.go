package eval

import (
	"math"
	"reflect"
	"slices"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/quality"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/resp"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/session"
	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/sim"
)

// FalseConfidentBPM is the error above which a high or medium reading counts as a confident mistake.
const FalseConfidentBPM = 5

type Point struct {
	TimeS                float64
	HR, RR               float64
	Phase                resp.Phase
	Confidence           quality.Confidence
	TrueHR, TrueRR       float64
	Inhaling, PhaseClear bool
}

type Metrics struct {
	Scenario         string
	HRCoverage       float64
	HRMAE            float64
	HRP95            float64
	HRFalseConfident float64
	FirstHRS         float64
	RRCoverage       float64
	RRMAE            float64
	PhaseCallRate    float64
	PhaseAgreement   float64
}

// RunScenario feeds the scenario to a fresh session and asks for one estimate per second of sample time.
func RunScenario(sc sim.Scenario, cfg session.Config) []Point {
	s := session.New(cfg)
	var pts []Point
	next := 1000.0
	for _, x := range sc.Generate() {
		for x.TimeMs >= next {
			pts = append(pts, point(sc, cfg, s.Estimate(next)))
			next += 1000
		}
		s.Push(x)
	}
	return pts
}

func point(sc sim.Scenario, cfg session.Config, e session.Estimate) Point {
	t := e.TimeMs / 1000
	in, clear := sc.Inhaling(t)
	return Point{
		TimeS: t, HR: e.HRBpm, RR: e.RRRpm, Phase: e.Phase, Confidence: e.Confidence,
		TrueHR: sc.MeanHR(t-cfg.HRWindowS, t), TrueRR: sc.RPM, Inhaling: in, PhaseClear: clear,
	}
}

// Summarize scores heart rate from hrFromS and breathing from rrFromS, the earliest times a value is expected.
func Summarize(pts []Point, hrFromS, rrFromS float64) Metrics {
	m := Metrics{FirstHRS: math.NaN()}
	var hrErr, rrErr []float64
	var hrN, rrN, confident, confidentWrong, clear, calls, agree int
	for _, p := range pts {
		if !math.IsNaN(p.HR) && math.IsNaN(m.FirstHRS) {
			m.FirstHRS = p.TimeS
		}
		if p.TimeS >= hrFromS {
			hrN++
			if !math.IsNaN(p.HR) {
				err := math.Abs(p.HR - p.TrueHR)
				hrErr = append(hrErr, err)
				if p.Confidence == quality.ConfidenceHigh || p.Confidence == quality.ConfidenceMedium {
					confident++
					if err > FalseConfidentBPM {
						confidentWrong++
					}
				}
			}
		}
		if p.TimeS < rrFromS {
			continue
		}
		rrN++
		if !math.IsNaN(p.RR) {
			rrErr = append(rrErr, math.Abs(p.RR-p.TrueRR))
		}
		if p.PhaseClear {
			clear++
			if p.Phase == resp.PhaseIn || p.Phase == resp.PhaseOut {
				calls++
				if (p.Phase == resp.PhaseIn) == p.Inhaling {
					agree++
				}
			}
		}
	}
	m.HRCoverage = ratio(len(hrErr), hrN)
	m.HRMAE = mean(hrErr)
	m.HRP95 = percentile(hrErr, 0.95)
	m.HRFalseConfident = ratio(confidentWrong, confident)
	m.RRCoverage = ratio(len(rrErr), rrN)
	m.RRMAE = mean(rrErr)
	m.PhaseCallRate = ratio(calls, clear)
	m.PhaseAgreement = ratio(agree, calls)
	return m
}

// Mean averages each metric over runs of the same scenario, skipping runs where it is undefined.
func Mean(ms []Metrics) Metrics {
	var out Metrics
	if len(ms) == 0 {
		return out
	}
	out.Scenario = ms[0].Scenario
	dst := reflect.ValueOf(&out).Elem()
	for f := 0; f < dst.NumField(); f++ {
		if dst.Field(f).Kind() != reflect.Float64 {
			continue
		}
		var vals []float64
		for _, m := range ms {
			if v := reflect.ValueOf(m).Field(f).Float(); !math.IsNaN(v) {
				vals = append(vals, v)
			}
		}
		dst.Field(f).SetFloat(mean(vals))
	}
	return out
}

func ratio(a, b int) float64 {
	if b == 0 {
		return math.NaN()
	}
	return float64(a) / float64(b)
}

func mean(x []float64) float64 {
	if len(x) == 0 {
		return math.NaN()
	}
	var s float64
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

func percentile(x []float64, p float64) float64 {
	if len(x) == 0 {
		return math.NaN()
	}
	s := slices.Clone(x)
	slices.Sort(s)
	return s[int(math.Ceil(p*float64(len(s))))-1]
}
