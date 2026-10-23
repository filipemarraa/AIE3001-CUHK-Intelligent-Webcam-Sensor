package sim

var (
	LightSkin = [3]float64{180, 140, 120}
	DarkSkin  = [3]float64{90, 60, 45}
)

// Base is a seated person at rest, paced at 6 breaths per minute, filmed at 30 fps with no dropped frames.
func Base(name string) Scenario {
	return Scenario{
		Name:          name,
		DurationS:     120,
		FPS:           30,
		JitterMs:      4,
		BaseBPM:       74,
		DriftBPM:      -6,
		RSABPM:        3,
		RPM:           6,
		Skin:          LightSkin,
		PulseAmp:      0.008,
		Noise:         0.1,
		Light:         130,
		ShoulderAmp:   0.004,
		ShoulderNoise: 0.0005,
		BallisticAmp:  0.0005,
	}
}

// Suite is the fixed benchmark: every engine change is compared on it before and after.
func Suite() []Scenario {
	ideal := Base("ideal")

	realFPS := Base("real_fps")
	realFPS.CostMs, realFPS.CostSDMs = 45, 12
	realFPS.JitterMs = 10

	stalls := realFPS
	stalls.Name = "worker_stalls"
	stalls.StallEveryS, stalls.StallMs = 20, 400

	dark := realFPS
	dark.Name = "dark_skin"
	dark.Skin = DarkSkin
	dark.PulseAmp = 0.003
	dark.Light = 70

	flicker := realFPS
	flicker.Name = "flicker_exposure"
	flicker.FlickerHz, flicker.FlickerAmp = 1.8, 0.01
	flicker.Steps = []Step{{AtS: 40, Gain: [3]float64{1.06, 1.04, 1.02}}, {AtS: 80, Gain: [3]float64{0.95, 0.97, 0.98}}}

	motion := realFPS
	motion.Name = "motion_bursts"
	motion.Motion = []Burst{{StartS: 30, DurS: 6}, {StartS: 70, DurS: 5}}
	motion.MotionAmp = 0.02
	motion.MotionSpeed = 3

	subtle := realFPS
	subtle.Name = "subtle_motion"
	subtle.Motion = []Burst{{StartS: 20, DurS: 80}}
	subtle.MotionAmp = 0.012
	subtle.MotionSpeed = 0.4

	talking := realFPS
	talking.Name = "talking"
	talking.Talking = []Burst{{StartS: 50, DurS: 10}}

	faceLost := realFPS
	faceLost.Name = "face_lost"
	faceLost.FaceLost = []Burst{{StartS: 45, DurS: 2}}

	breath10 := realFPS
	breath10.Name = "breathing_10rpm_sway"
	breath10.RPM = 10
	breath10.BallisticAmp = 0.001
	breath10.SwayHz, breath10.SwayAmp = 0.05, 0.002

	breath15 := realFPS
	breath15.Name = "breathing_15rpm_noisy"
	breath15.RPM = 15
	breath15.ShoulderAmp = 0.003
	breath15.ShoulderNoise = 0.002

	still := realFPS
	still.Name = "no_breathing_signal"
	still.ShoulderAmp = 0
	still.ShoulderNoise = 0.001

	noPulse := realFPS
	noPulse.Name = "rppg_lost_breathing_ok"
	noPulse.RPM = 10
	noPulse.PulseAmp = 0

	return []Scenario{ideal, realFPS, stalls, dark, flicker, motion, subtle, talking, faceLost, breath10, breath15, still, noPulse}
}
