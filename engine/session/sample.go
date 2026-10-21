package session

import (
	"errors"
	"math"
)

var (
	ErrSampleLen = errors.New("session: sample slice has the wrong length")
	ErrNotFinite = errors.New("session: sample contains NaN or infinity")
)

// Flat layout of one sample, shared with packages/vitals-web/src/engine/layout.ts.
const (
	FieldTimeMs = iota
	FieldR
	FieldG
	FieldB
	FieldFace
	FieldMotion
	FieldLight
	FieldMouth
	FieldShoulderY
	FieldShoulderW
	SampleLen
)

type Sample struct {
	TimeMs, R, G, B float64
	Face            bool
	Motion, Light   float64
	Mouth           float64
	ShoulderY       float64
	ShoulderW       float64

	speed float64
}

func SampleFromSlice(x []float64) (Sample, error) {
	if len(x) != SampleLen {
		return Sample{}, ErrSampleLen
	}
	for _, v := range x {
		if !isFinite(v) {
			return Sample{}, ErrNotFinite
		}
	}
	return Sample{
		TimeMs:    x[FieldTimeMs],
		R:         x[FieldR],
		G:         x[FieldG],
		B:         x[FieldB],
		Face:      x[FieldFace] != 0,
		Motion:    x[FieldMotion],
		Light:     x[FieldLight],
		Mouth:     x[FieldMouth],
		ShoulderY: x[FieldShoulderY],
		ShoulderW: x[FieldShoulderW],
	}, nil
}

// values is the flat layout of x; SampleFromSlice is its inverse.
func (x Sample) values() [SampleLen]float64 {
	var v [SampleLen]float64
	v[FieldTimeMs], v[FieldR], v[FieldG], v[FieldB] = x.TimeMs, x.R, x.G, x.B
	if x.Face {
		v[FieldFace] = 1
	}
	v[FieldMotion], v[FieldLight], v[FieldMouth] = x.Motion, x.Light, x.Mouth
	v[FieldShoulderY], v[FieldShoulderW] = x.ShoulderY, x.ShoulderW
	return v
}

func (x Sample) finite() bool {
	for _, v := range x.values() {
		if !isFinite(v) {
			return false
		}
	}
	return true
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
