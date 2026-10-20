package dsp

import (
	"errors"
	"math"
	"math/bits"
	"math/cmplx"
)

var ErrFlat = errors.New("dsp: no power in band")

type Peak struct {
	FreqHz float64
	SNRdB  float64
}

func FFT(x []complex128) {
	n := len(x)
	if n == 0 || n&(n-1) != 0 {
		panic("dsp: FFT length must be a power of two")
	}
	shift := 64 - bits.TrailingZeros(uint(n))
	for i := range x {
		j := int(bits.Reverse64(uint64(i)) >> shift)
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		step := cmplx.Rect(1, -2*math.Pi/float64(size))
		for start := 0; start < n; start += size {
			w := complex(1, 0)
			for k := 0; k < size/2; k++ {
				a, b := x[start+k], w*x[start+k+size/2]
				x[start+k], x[start+k+size/2] = a+b, a-b
				w *= step
			}
		}
	}
}

func nextPow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

func DominantFrequency(x []float64, fs, lowHz, highHz float64) (Peak, error) {
	if len(x) < int(math.Ceil(2*fs/lowHz)) {
		return Peak{}, ErrTooShort
	}

	var mean float64
	for _, v := range x {
		mean += v
	}
	mean /= float64(len(x))

	// Zero padding to at least two minutes of samples keeps each bin under 0.5 bpm wide.
	nfft := nextPow2(max(len(x), int(math.Ceil(120*fs))))
	buf := make([]complex128, nfft)
	for i, v := range x {
		hann := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(len(x)-1))
		buf[i] = complex((v-mean)*hann, 0)
	}
	FFT(buf)

	binHz := fs / float64(nfft)
	kLo := int(math.Ceil(lowHz / binHz))
	kHi := min(int(math.Floor(highHz/binHz)), nfft/2)
	power := make([]float64, nfft/2+1)
	for k := range power {
		power[k] = real(buf[k])*real(buf[k]) + imag(buf[k])*imag(buf[k])
	}

	best, total := kLo, 0.0
	for k := kLo; k <= kHi; k++ {
		total += power[k]
		if power[k] > power[best] {
			best = k
		}
	}
	if total <= 1e-20 {
		return Peak{}, ErrFlat
	}

	freq := float64(best) * binHz
	if best > 0 && best < len(power)-1 && power[best-1] > 0 && power[best+1] > 0 {
		l, c, r := math.Log(power[best-1]), math.Log(power[best]), math.Log(power[best+1])
		if den := l - 2*c + r; den != 0 {
			freq += 0.5 * (l - r) / den * binHz
		}
	}

	// The Hann main lobe is 2/T wide on each side; a narrower signal window would count the peak's own energy as noise.
	half := math.Max(0.1, 2*fs/float64(len(x)))
	var signal, noise float64
	for k := kLo; k <= kHi; k++ {
		f := float64(k) * binHz
		if math.Abs(f-freq) <= half || math.Abs(f-2*freq) <= half {
			signal += power[k]
		} else {
			noise += power[k]
		}
	}
	noise = math.Max(noise, signal*1e-6)
	return Peak{FreqHz: freq, SNRdB: 10 * math.Log10(signal/noise)}, nil
}
