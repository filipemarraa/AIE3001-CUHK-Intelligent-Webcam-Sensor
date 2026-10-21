package sim

import (
	"math"
	"math/rand/v2"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/session"
)

type Burst struct {
	StartS, DurS float64
}

func (b Burst) covers(t float64) bool {
	return t >= b.StartS && t < b.StartS+b.DurS
}

type Step struct {
	AtS  float64
	Gain [3]float64
}

type Scenario struct {
	Name      string
	DurationS float64
	FPS       float64
	JitterMs  float64

	// The worker keeps one frame in flight: frames that arrive while it is busy are dropped.
	CostMs, CostSDMs     float64
	StallEveryS, StallMs float64

	BaseBPM, DriftBPM, RSABPM float64
	RPM                       float64

	Skin       [3]float64
	PulseAmp   float64
	Noise      float64
	Light      float64
	FlickerHz  float64
	FlickerAmp float64
	Steps      []Step

	Motion      []Burst
	MotionAmp   float64
	MotionSpeed float64
	Talking     []Burst
	FaceLost    []Burst

	ShoulderAmp   float64
	ShoulderNoise float64
	BallisticAmp  float64
	SwayHz        float64
	SwayAmp       float64

	Seed uint64
}

var bloodVolumePulse = [3]float64{0.33, 0.77, 0.53}

// motionTint is not proportional to the skin colour, so POS cannot cancel it the way it cancels intensity changes.
var motionTint = [3]float64{1, 0.85, 0.7}

const (
	stillSpeedIODPerS = 0.05
	poseEveryNFrames  = 2
)

func (sc Scenario) HR(t float64) float64 {
	return sc.BaseBPM + sc.DriftBPM*t/sc.DurationS + sc.RSABPM*math.Sin(sc.breathPhase(t))
}

func (sc Scenario) breathPhase(t float64) float64 {
	return 2 * math.Pi * sc.RPM / 60 * t
}

// Inhaling is true while the chest rises; Clear is false near the turning points, where in and out are ambiguous.
func (sc Scenario) Inhaling(t float64) (inhaling, clear bool) {
	c := math.Cos(sc.breathPhase(t))
	return c > 0, math.Abs(c) >= 0.5
}

// MeanHR averages the true heart rate over [fromS, toS], the way a windowed estimator sees it.
func (sc Scenario) MeanHR(fromS, toS float64) float64 {
	fromS = max(0, fromS)
	if toS <= fromS {
		return sc.HR(toS)
	}
	const step = 0.01
	var sum float64
	n := 0
	for t := fromS; t <= toS; t += step {
		sum += sc.HR(t)
		n++
	}
	return sum / float64(n)
}

func (sc Scenario) Generate() []session.Sample {
	rng := rand.New(rand.NewPCG(sc.Seed, sc.Seed^0x9e3779b97f4a7c15))
	frames := int(sc.DurationS * sc.FPS)
	out := make([]session.Sample, 0, frames)
	cardiac := 0.0
	prevT := 0.0
	lastEmitted := math.Inf(-1)
	busyUntil := math.Inf(-1)
	nextStall := math.Inf(1)
	if sc.StallEveryS > 0 {
		nextStall = rng.ExpFloat64() * sc.StallEveryS
	}
	emitted := 0
	var shoulderY, shoulderW float64
	motionHz := 0.8 + 1.2*rng.Float64()

	for i := 0; i < frames; i++ {
		t := float64(i)/sc.FPS + sc.JitterMs/1000*(rng.Float64()-0.5)
		cardiac += 2 * math.Pi * sc.HR(t) / 60 * max(0, t-prevT)
		prevT = t
		if t < busyUntil {
			continue
		}
		cost := max(0, sc.CostMs+sc.CostSDMs*rng.NormFloat64())
		// Stalls follow the clock, not the frames the worker accepts, so a busier worker does not stall less.
		if t >= nextStall {
			cost += sc.StallMs
			nextStall = t + rng.ExpFloat64()*sc.StallEveryS
		}
		busyUntil = t + cost/1000

		x := session.Sample{TimeMs: t * 1000}
		if emitted%poseEveryNFrames == 0 {
			shoulderY, shoulderW = sc.shoulders(t, cardiac, rng)
		}
		x.ShoulderY, x.ShoulderW = shoulderY, shoulderW
		emitted++

		if covered(sc.FaceLost, t) {
			lastEmitted = math.Inf(-1)
			out = append(out, x)
			continue
		}
		x.Face = true
		x.R, x.G, x.B = sc.colour(t, cardiac, motionHz, rng)
		x.Light = sc.Light
		x.Mouth = 0.02 + 0.002*rng.NormFloat64()
		if covered(sc.Talking, t) {
			x.Mouth += 0.15 * math.Abs(math.Sin(2*math.Pi*4*t))
		}
		speed := stillSpeedIODPerS
		if covered(sc.Motion, t) {
			speed = sc.MotionSpeed
		}
		if !math.IsInf(lastEmitted, -1) {
			x.Motion = speed * (t - lastEmitted) * (0.8 + 0.4*rng.Float64())
		}
		lastEmitted = t
		out = append(out, x)
	}
	return out
}

func (sc Scenario) colour(t, cardiac, motionHz float64, rng *rand.Rand) (r, g, b float64) {
	pulse := math.Sin(cardiac) + 0.4*math.Sin(2*cardiac+0.5)
	light := 1 + sc.FlickerAmp*math.Sin(2*math.Pi*sc.FlickerHz*t) + 0.02*math.Sin(2*math.Pi*0.05*t)
	var c [3]float64
	for k := range c {
		gain := 1.0
		for _, s := range sc.Steps {
			if t >= s.AtS {
				gain *= s.Gain[k]
			}
		}
		if covered(sc.Motion, t) {
			gain *= 1 + sc.MotionAmp*motionTint[k]*math.Sin(2*math.Pi*motionHz*t)
		}
		c[k] = sc.Skin[k]*light*gain*(1+sc.PulseAmp*bloodVolumePulse[k]*pulse) + sc.Noise*rng.NormFloat64()
	}
	return c[0], c[1], c[2]
}

// Image y grows downward: inhaling lifts the shoulders, so y falls. Sway also changes the apparent width.
func (sc Scenario) shoulders(t, cardiac float64, rng *rand.Rand) (y, w float64) {
	sway := sc.SwayAmp * math.Sin(2*math.Pi*sc.SwayHz*t)
	y = 0.6 - sc.ShoulderAmp*math.Sin(sc.breathPhase(t)) + sc.BallisticAmp*math.Sin(cardiac) + sway +
		sc.ShoulderNoise*rng.NormFloat64()
	w = 0.3 * (1 + 2*sc.SwayAmp*math.Sin(2*math.Pi*sc.SwayHz*t+1))
	return y, w
}

func covered(bs []Burst, t float64) bool {
	for _, b := range bs {
		if b.covers(t) {
			return true
		}
	}
	return false
}
