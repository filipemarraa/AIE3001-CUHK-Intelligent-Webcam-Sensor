package dsp

import (
	"errors"
	"math"
	"math/bits"
	"math/cmplx"
)

var (
	ErrFlat         = errors.New("dsp: no power in band")
	ErrNoPeakInBand = errors.New("dsp: spectral peak is not inside the band")
)

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

// HalfPowerRatio is how much of the strongest peak's power the peak at half its frequency needs to be taken for the
// fundamental: the second harmonic of a slow pulse can outgrow the pulse itself, but half the frequency of a fast pulse
// only holds noise.
const HalfPowerRatio = 0.5

// HarmonicSearchHz is how far from exactly half or twice the strongest peak a candidate peak may sit.
const HarmonicSearchHz = 0.05

func DominantFrequency(x []float64, fs, lowHz, highHz float64) (Peak, error) {
	sp, err := newSpectrum(x, fs, lowHz, highHz)
	if err != nil {
		return Peak{}, err
	}
	return sp.peak(sp.best)
}

// HeartFrequency is DominantFrequency for a pulse, whose strongest peak may be its second harmonic. Half the strongest
// frequency replaces it when it holds HalfPowerRatio of its power, and twice it when it does; with a prior (NaN when
// there is none) a candidate is taken only if it is closer to the prior, and twice the frequency needs a prior.
func HeartFrequency(x []float64, fs, lowHz, highHz, priorHz float64) (Peak, error) {
	sp, err := newSpectrum(x, fs, lowHz, highHz)
	if err != nil {
		return Peak{}, err
	}
	top, err := sp.peak(sp.best)
	if err != nil {
		return Peak{}, err
	}
	hasPrior := !math.IsNaN(priorHz)
	closer := func(f float64) bool { return !hasPrior || math.Abs(f-priorHz) < math.Abs(top.FreqHz-priorHz) }
	if k, ok := sp.candidate(top.FreqHz / 2); ok && closer(sp.freq(k)) {
		return sp.peak(k)
	}
	if k, ok := sp.candidate(2 * top.FreqHz); ok && hasPrior && closer(sp.freq(k)) {
		return sp.peak(k)
	}
	return top, nil
}

type spectrum struct {
	power                         []float64
	binHz, lowHz, highHz, windowS float64
	// best is the strongest bin in [kLo, kHi].
	kLo, kHi, best int
}

func newSpectrum(x []float64, fs, lowHz, highHz float64) (*spectrum, error) {
	if len(x) < int(math.Ceil(2*fs/lowHz)) {
		return nil, ErrTooShort
	}
	if !allFinite(x) {
		return nil, ErrNotFinite
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

	sp := &spectrum{binHz: fs / float64(nfft), lowHz: lowHz, highHz: highHz, windowS: float64(len(x)) / fs}
	sp.kLo = int(math.Ceil(lowHz / sp.binHz))
	sp.kHi = min(int(math.Floor(highHz/sp.binHz)), nfft/2)
	sp.power = make([]float64, nfft/2+1)
	for k := range sp.power {
		sp.power[k] = real(buf[k])*real(buf[k]) + imag(buf[k])*imag(buf[k])
	}

	sp.best = sp.kLo
	var total float64
	for k := sp.kLo; k <= sp.kHi; k++ {
		total += sp.power[k]
		if sp.power[k] > sp.power[sp.best] {
			sp.best = k
		}
	}
	if total <= 1e-20 {
		return nil, ErrFlat
	}
	return sp, nil
}

func (sp *spectrum) freq(k int) float64 { return float64(k) * sp.binHz }

func (sp *spectrum) isLocalMax(k int) bool {
	return k > 0 && k < len(sp.power)-1 && sp.power[k-1] < sp.power[k] && sp.power[k+1] < sp.power[k]
}

// candidate returns the strongest in-band local maximum within HarmonicSearchHz of hz, if it holds HalfPowerRatio of
// the strongest peak's power.
func (sp *spectrum) candidate(hz float64) (int, bool) {
	lo := max(sp.kLo, int(math.Ceil((hz-HarmonicSearchHz)/sp.binHz)))
	hi := min(sp.kHi, int(math.Floor((hz+HarmonicSearchHz)/sp.binHz)))
	best := -1
	for k := lo; k <= hi; k++ {
		if sp.isLocalMax(k) && (best < 0 || sp.power[k] > sp.power[best]) {
			best = k
		}
	}
	return best, best >= 0 && sp.power[best] >= HalfPowerRatio*sp.power[sp.best]
}

// peak refines bin k and measures its SNR.
func (sp *spectrum) peak(k int) (Peak, error) {
	// A maximum on the band edge that keeps rising outside belongs to a frequency outside the band.
	if !sp.isLocalMax(k) {
		return Peak{}, ErrNoPeakInBand
	}

	freq := sp.freq(k)
	if sp.power[k-1] > 0 && sp.power[k+1] > 0 {
		l, c, r := math.Log(sp.power[k-1]), math.Log(sp.power[k]), math.Log(sp.power[k+1])
		if den := l - 2*c + r; den != 0 {
			freq += 0.5 * (l - r) / den * sp.binHz
		}
	}
	if freq < sp.lowHz || freq > sp.highHz {
		return Peak{}, ErrNoPeakInBand
	}

	// The Hann main lobe is 2/T wide on each side; a narrower signal window would count the peak's own energy as noise.
	half := math.Max(0.1, 2/sp.windowS)
	var signal, noise float64
	for k := sp.kLo; k <= sp.kHi; k++ {
		f := sp.freq(k)
		if math.Abs(f-freq) <= half || math.Abs(f-2*freq) <= half {
			signal += sp.power[k]
		} else {
			noise += sp.power[k]
		}
	}
	noise = math.Max(noise, signal*1e-6)
	return Peak{FreqHz: freq, SNRdB: 10 * math.Log10(signal/noise)}, nil
}
