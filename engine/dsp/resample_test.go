package dsp

import (
	"errors"
	"math"
	"testing"
)

func TestResampleLinearRampOntoUniformGrid(t *testing.T) {
	ts := []float64{0, 30, 70, 100, 140, 170, 200}
	ramp := make([]float64, len(ts))
	for i, v := range ts {
		ramp[i] = 2*v + 5
	}

	out, err := Resample(ts, [][]float64{ramp}, 50, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := len(out[0]), 11; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
	for k, v := range out[0] {
		want := 2*float64(k)*20 + 5
		if math.Abs(v-want) > 1e-9 {
			t.Errorf("out[%d] = %v, want %v", k, v, want)
		}
	}
}

func TestResampleKeepsChannelsAligned(t *testing.T) {
	ts := []float64{0, 40, 80}
	a := []float64{0, 4, 8}
	b := []float64{10, 6, 2}

	out, err := Resample(ts, [][]float64{a, b}, 50, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantA := []float64{0, 2, 4, 6, 8}
	wantB := []float64{10, 8, 6, 4, 2}
	for k := range wantA {
		if math.Abs(out[0][k]-wantA[k]) > 1e-9 || math.Abs(out[1][k]-wantB[k]) > 1e-9 {
			t.Errorf("k=%d got (%v, %v), want (%v, %v)", k, out[0][k], out[1][k], wantA[k], wantB[k])
		}
	}
}

func TestResampleErrors(t *testing.T) {
	tests := []struct {
		name string
		ts   []float64
		ch   [][]float64
		want error
	}{
		{"single sample", []float64{0}, [][]float64{{1}}, ErrTooShort},
		{"gap above limit", []float64{0, 33, 300, 333}, [][]float64{{1, 2, 3, 4}}, ErrGap},
		{"repeated timestamp", []float64{0, 33, 33, 66}, [][]float64{{1, 2, 3, 4}}, ErrNotIncreasing},
		{"backwards timestamp", []float64{0, 33, 20, 66}, [][]float64{{1, 2, 3, 4}}, ErrNotIncreasing},
		{"length mismatch", []float64{0, 33, 66}, [][]float64{{1, 2}}, ErrLengthMismatch},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Resample(tc.ts, tc.ch, 30, 250)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
