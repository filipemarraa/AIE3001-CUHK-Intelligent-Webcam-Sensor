package rppg

import (
	"math"
	"math/rand/v2"
)

type synthConfig struct {
	bpm        float64
	durationS  float64
	fps        float64
	jitterMs   float64
	dc         [3]float64
	pulseAmp   float64
	flickerHz  float64
	flickerAmp float64
	noise      float64
	seed       uint64
}

var bloodVolumePulse = [3]float64{0.33, 0.77, 0.53}

func synth(c synthConfig) (ts, r, g, b []float64) {
	rng := rand.New(rand.NewPCG(c.seed, c.seed+1))
	n := int(c.durationS * c.fps)
	f := c.bpm / 60
	out := [3][]float64{}
	for i := 0; i < n; i++ {
		t := float64(i)/c.fps + c.jitterMs/1000*(rng.Float64()-0.5)
		pulse := math.Sin(2*math.Pi*f*t) + 0.4*math.Sin(4*math.Pi*f*t+0.5)
		light := 1 + c.flickerAmp*math.Sin(2*math.Pi*c.flickerHz*t) + 0.02*math.Sin(2*math.Pi*0.05*t)
		ts = append(ts, t*1000)
		for k := range out {
			v := c.dc[k]*light*(1+c.pulseAmp*bloodVolumePulse[k]*pulse) + c.noise*rng.NormFloat64()
			out[k] = append(out[k], v)
		}
	}
	return ts, out[0], out[1], out[2]
}
