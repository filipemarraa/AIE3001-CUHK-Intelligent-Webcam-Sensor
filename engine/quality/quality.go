package quality

// MaxMotionPerS is the face landmark speed, in interocular distances per second, above which a frame is moving.
// It is per second so it does not depend on the frame rate.
const MaxMotionPerS = 0.75

const (
	minFaceFraction = 0.8
	minLight        = 40
	goodLight       = 120
	maxMouthStd     = 0.03
	goodSNRdB       = 10
	highQuality     = 0.75
	mediumQuality   = 0.6
)

// The resampler handles irregular frame times, so timing is judged by the samples per second and the longest gap.
// Below minSampleRateHz the pulse has too few samples per beat; maxGapMs is about half a beat at 120 bpm.
const (
	minSampleRateHz  = 8
	goodSampleRateHz = 20
	maxGapMs         = 250
)

// Below minSNRdB the spectral peak is as likely noise as pulse: with no pulse in the colour, the simulation reported a
// heart rate 30 bpm off. Provisional until recordings against a reference device set it.
const minSNRdB = 1.0

type Confidence int

const (
	ConfidenceHigh Confidence = iota
	ConfidenceMedium
	ConfidenceLow
	ConfidenceInvalid
)

func (c Confidence) String() string {
	switch c {
	case ConfidenceHigh:
		return "high"
	case ConfidenceMedium:
		return "medium"
	case ConfidenceLow:
		return "low"
	default:
		return "invalid"
	}
}

type Reason int

const (
	ReasonNone Reason = iota
	ReasonNoFace
	ReasonMotion
	ReasonLowLight
	ReasonTalking
	ReasonWarmingUp
	ReasonWeakPulse
)

func (r Reason) String() string {
	switch r {
	case ReasonNoFace:
		return "no_face"
	case ReasonMotion:
		return "too_much_motion"
	case ReasonLowLight:
		return "low_light"
	case ReasonTalking:
		return "talking"
	case ReasonWarmingUp:
		return "warming_up"
	case ReasonWeakPulse:
		return "weak_pulse"
	default:
		return ""
	}
}

type Inputs struct {
	FaceFraction float64
	MotionP90    float64
	LightMean    float64
	MouthStd     float64
	SampleRateHz float64
	MaxGapMs     float64
	SNRdB        float64
	HasHR        bool
	WarmingUp    bool
}

type Result struct {
	Quality    float64
	Confidence Confidence
	Reason     Reason
}

func Assess(in Inputs) Result {
	q := 0.4*unit(in.SNRdB/goodSNRdB) +
		0.2*unit(1-in.MotionP90/MaxMotionPerS) +
		0.2*unit((in.LightMean-minLight)/(goodLight-minLight)) +
		0.2*timing(in)

	r := reason(in)
	conf := ConfidenceInvalid
	if r == ReasonNone && in.HasHR {
		switch {
		case q >= highQuality:
			conf = ConfidenceHigh
		case q >= mediumQuality:
			conf = ConfidenceMedium
		default:
			conf = ConfidenceLow
		}
	}
	return Result{Quality: q, Confidence: conf, Reason: r}
}

// BreathingBlocked reports what also disturbs the shoulders, whatever reason comes first for the heart rate.
func BreathingBlocked(in Inputs) bool {
	return in.FaceFraction < minFaceFraction || in.MotionP90 > MaxMotionPerS || in.MouthStd > maxMouthStd
}

func reason(in Inputs) Reason {
	switch {
	case in.FaceFraction < minFaceFraction:
		return ReasonNoFace
	case in.MotionP90 > MaxMotionPerS:
		return ReasonMotion
	case in.LightMean < minLight:
		return ReasonLowLight
	case in.MouthStd > maxMouthStd:
		return ReasonTalking
	case in.WarmingUp:
		return ReasonWarmingUp
	case in.HasHR && in.SNRdB < minSNRdB:
		return ReasonWeakPulse
	}
	return ReasonNone
}

func timing(in Inputs) float64 {
	if in.MaxGapMs > maxGapMs {
		return 0
	}
	return unit((in.SampleRateHz - minSampleRateHz) / (goodSampleRateHz - minSampleRateHz))
}

func unit(x float64) float64 {
	return min(max(x, 0), 1)
}
