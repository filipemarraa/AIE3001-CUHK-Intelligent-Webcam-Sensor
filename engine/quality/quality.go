package quality

const (
	minFaceFraction = 0.8
	// Face landmark speed in interocular distances per second; per second so it does not depend on the frame rate.
	maxMotionP90  = 0.75
	minLight      = 40
	goodLight     = 120
	maxMouthStd   = 0.03
	maxJitterMs   = 20
	goodSNRdB     = 10
	highQuality   = 0.75
	mediumQuality = 0.6
)

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
	default:
		return ""
	}
}

type Inputs struct {
	FaceFraction float64
	MotionP90    float64
	LightMean    float64
	MouthStd     float64
	JitterMs     float64
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
		0.2*unit(1-in.MotionP90/maxMotionP90) +
		0.2*unit((in.LightMean-minLight)/(goodLight-minLight)) +
		0.2*unit(1-in.JitterMs/maxJitterMs)

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

func reason(in Inputs) Reason {
	switch {
	case in.FaceFraction < minFaceFraction:
		return ReasonNoFace
	case in.MotionP90 > maxMotionP90:
		return ReasonMotion
	case in.LightMean < minLight:
		return ReasonLowLight
	case in.MouthStd > maxMouthStd:
		return ReasonTalking
	case in.WarmingUp:
		return ReasonWarmingUp
	}
	return ReasonNone
}

func unit(x float64) float64 {
	return min(max(x, 0), 1)
}
