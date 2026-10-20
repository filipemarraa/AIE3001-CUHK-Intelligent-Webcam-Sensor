package rppg

import "math"

func POS(r, g, b []float64, windowLen int) []float64 {
	n := len(g)
	h := make([]float64, n)
	if windowLen < 2 || n < windowLen {
		return h
	}

	s1 := make([]float64, windowLen)
	s2 := make([]float64, windowLen)
	for m := 0; m+windowLen <= n; m++ {
		mr, mg, mb := mean(r[m:m+windowLen]), mean(g[m:m+windowLen]), mean(b[m:m+windowLen])
		if mr == 0 || mg == 0 || mb == 0 {
			continue
		}
		for i := 0; i < windowLen; i++ {
			rn, gn, bn := r[m+i]/mr, g[m+i]/mg, b[m+i]/mb
			s1[i] = gn - bn
			s2[i] = -2*rn + gn + bn
		}
		alpha := 0.0
		if sd2 := stddev(s2); sd2 > 0 {
			alpha = stddev(s1) / sd2
		}
		var hm float64
		for i := range s1 {
			hm += s1[i] + alpha*s2[i]
		}
		hm /= float64(windowLen)
		for i := range s1 {
			h[m+i] += s1[i] + alpha*s2[i] - hm
		}
	}
	return h
}

func mean(x []float64) float64 {
	var s float64
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

func stddev(x []float64) float64 {
	m := mean(x)
	var s float64
	for _, v := range x {
		s += (v - m) * (v - m)
	}
	return math.Sqrt(s / float64(len(x)))
}
