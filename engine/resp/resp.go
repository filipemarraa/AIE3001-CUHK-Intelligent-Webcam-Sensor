package resp

import (
	"errors"
	"math"
	"slices"

	"github.com/filipemarraa/AIE3001-CUHK-Intelligent-Webcam-Sensor/engine/dsp"
)

var (
	ErrTooShort    = errors.New("resp: window shorter than the minimum duration")
	ErrNoShoulders = errors.New("resp: shoulder width must be positive")
	ErrUnreliable  = errors.New("resp: shoulder signal is not periodic enough to be breathing")
)

type Phase int

const (
	PhaseUnknown Phase = iota
	PhaseIn
	PhaseOut
	PhaseHold
)

func (p Phase) String() string {
	switch p {
	case PhaseIn:
		return "in"
	case PhaseOut:
		return "out"
	case PhaseHold:
		return "hold"
	}
	return ""
}

type Config struct {
	SampleRateHz   float64
	MaxGapMs       float64
	LowHz          float64
	HighHz         float64
	MinDurationS   float64
	HoldWindowS    float64
	HoldRatio      float64
	EnterThreshold float64
	ExitThreshold  float64
	MinPeriodicity float64
	MinSNRdB       float64
}

func DefaultConfig() Config {
	return Config{
		SampleRateHz:   10,
		MaxGapMs:       500,
		LowHz:          0.08,
		HighHz:         0.5,
		MinDurationS:   25,
		HoldWindowS:    2,
		HoldRatio:      0.05,
		EnterThreshold: 0.2,
		ExitThreshold:  0.1,
		MinPeriodicity: 0.5,
		MinSNRdB:       3,
	}
}

type Breath struct {
	RPM         float64
	SNRdB       float64
	Periodicity float64
	Phase       Phase
}

func Estimate(tsMs, shoulderY, shoulderW []float64, prev Phase, cfg Config) (Breath, error) {
	if len(shoulderY) != len(tsMs) || len(shoulderW) != len(tsMs) {
		return Breath{}, dsp.ErrLengthMismatch
	}
	for _, w := range shoulderW {
		if !(w > 0) {
			return Breath{}, ErrNoShoulders
		}
	}
	if len(tsMs) < 2 || (tsMs[len(tsMs)-1]-tsMs[0])/1000 < cfg.MinDurationS {
		return Breath{}, ErrTooShort
	}

	// Image y grows downward, so inhaling lifts the shoulders and raises v.
	v := make([]float64, len(tsMs))
	for i := range v {
		v[i] = -shoulderY[i] / shoulderW[i]
	}
	grid, err := dsp.Resample(tsMs, [][]float64{v}, cfg.SampleRateHz, cfg.MaxGapMs)
	if err != nil {
		return Breath{}, err
	}
	detrended := dsp.Detrend(grid[0])
	filtered := dsp.Bandpass(detrended, cfg.SampleRateHz, cfg.LowHz, cfg.HighHz)

	peak, err := dsp.DominantFrequency(filtered, cfg.SampleRateHz, cfg.LowHz, cfg.HighHz)
	if err != nil {
		return Breath{}, err
	}
	// The SNR band is wide for slow breathing, so noise can still score well; repeating at its own period cannot.
	periodicity := autocorrelation(filtered, int(math.Round(cfg.SampleRateHz/peak.FreqHz)))
	if periodicity < cfg.MinPeriodicity || peak.SNRdB < cfg.MinSNRdB {
		return Breath{}, ErrUnreliable
	}

	return Breath{
		RPM: peak.FreqHz * 60, SNRdB: peak.SNRdB, Periodicity: periodicity,
		Phase: phase(detrended, grid[0], peak.FreqHz, prev, cfg),
	}, nil
}

// phase reads the direction from a sinusoid fitted at the breathing frequency over the last cycle: a matched filter
// that rejects faster motion such as the heartbeat and does not ring at the window edge like a band-pass filter.
// Turning points come out as hold through the hysteresis on that direction. A real pause is a recent window that does
// not follow the breathing fitted just before it: fitting over the window itself would let the fit bend to the pause.
func phase(x, raw []float64, freqHz float64, prev Phase, cfg Config) Phase {
	fit, ok := fitSinusoid(x, freqHz, cfg.SampleRateHz)
	if !ok {
		return PhaseHold
	}
	heartHz := heartbeatHz(x, cfg.SampleRateHz)
	windowS := cfg.HoldWindowS
	if heartHz > 0 {
		// Two heartbeat cycles in the window keep its sinusoid from soaking up the slower breathing.
		windowS = max(windowS, 2/heartHz)
	}
	m := max(3, int(math.Round(windowS*cfg.SampleRateHz)))
	// Only a stretch that is actually still can be a pause, and the breathing is fitted before it, so a long pause
	// does not leak into the reference it is compared with.
	w := stillSamples(x, raw, m, heartHz, cfg)
	if w < m || len(x) <= w+fit.n {
		return nextPhase(fit.direction(), prev, cfg)
	}
	before, ok := fitSinusoid(x[:len(x)-w], freqHz, cfg.SampleRateHz)
	if !ok {
		return nextPhase(fit.direction(), prev, cfg)
	}
	// Noise can stretch w back into the breathing before the pause, so the comparison looks only at the most recent
	// part of the still stretch, against the breathing projected from before it.
	k := min(w, 2*m)
	observed, predicted := x[len(x)-k:], before.next(w)[w-k:]
	// A pause already called holds while the shoulders stay still for longer than any turning point does, so one noisy
	// window cannot flip it back; but only while a straight line still explains the recent stretch at least as well as
	// the breathing would, or noise that raises the stillness limits could latch the hold during breathing.
	if prev == PhaseHold && w >= 2*m && stillFavoured(observed, predicted, heartHz, cfg) {
		return PhaseHold
	}
	if paused(observed, predicted, before.amp, heartHz, cfg) {
		return PhaseHold
	}
	return nextPhase(fit.direction(), prev, cfg)
}

// stillSamples is the length of the trailing stretch, at least m samples and at most half the window, over which the
// shoulders hold still: what is left after a straight line and the heartbeat is small next to the breathing amplitude
// of the whole window (from the detrended x), and so is the drift of that line. Stillness is judged on raw, before the
// detrend, which would tilt a level stretch. It returns 0 when the last m samples already move.
func stillSamples(x, raw []float64, m int, heartHz float64, cfg Config) int {
	var mean, ss float64
	for _, v := range x {
		mean += v
	}
	mean /= float64(len(x))
	for _, v := range x {
		ss += (v - mean) * (v - mean)
	}
	amp := math.Sqrt(2 * ss / float64(len(x)))
	// Landmark noise must not count as motion: what is left after the line may reach a few times the noise floor,
	// and the line itself may tilt by a few times what noise alone tilts a line of n samples.
	noise := noiseFloor(withoutHeartbeat(raw, heartHz, cfg.SampleRateHz))
	limit := max(stillResidual*amp, 3*noise)
	step := max(1, int(math.Round(cfg.SampleRateHz/2)))
	still := 0
	for n := m; n <= len(raw)/2; n += step {
		seg := raw[len(raw)-n:]
		resid, drift := lineFit(seg, heartHz, cfg.SampleRateHz)
		if math.Sqrt(resid/float64(n)) > limit || math.Abs(drift) > max(stillDrift*amp, 3*noise*math.Sqrt(12/float64(n))) {
			break
		}
		still = n
	}
	return still
}

const (
	stillResidual = 0.1
	stillDrift    = 0.1
)

// withoutHeartbeat removes the heartbeat sinusoid from x, so the noise floor does not count it as noise.
func withoutHeartbeat(x []float64, heartHz, fs float64) []float64 {
	if heartHz <= 0 {
		return x
	}
	cols := heartColumns(len(x), heartHz, fs, false)
	coef, _ := leastSquares(x, cols)
	if coef == nil {
		return x
	}
	out := make([]float64, len(x))
	for k, v := range x {
		out[k] = v - coef[1]*cols[1][k] - coef[2]*cols[2][k]
	}
	return out
}

// noiseFloor estimates the per-sample noise from second differences, which cancel the slow breathing; the median
// keeps a pause or a turn from inflating it.
func noiseFloor(x []float64) float64 {
	if len(x) < 3 {
		return 0
	}
	d := make([]float64, 0, len(x)-2)
	for i := 2; i < len(x); i++ {
		d = append(d, math.Abs(x[i]-2*x[i-1]+x[i-2]))
	}
	slices.Sort(d)
	// For white noise the second difference has six times the variance; 1.4826 turns a median deviation into sigma.
	return 1.4826 * d[len(d)/2] / math.Sqrt(6)
}

// lineFit fits a straight line plus the heartbeat sinusoid to x and returns the squared error left and how far the
// line moves across x.
func lineFit(x []float64, heartHz, fs float64) (resid, drift float64) {
	n := len(x)
	cols := heartColumns(n, heartHz, fs, true)
	coef, err := leastSquares(x, cols)
	return err, coef[1] * float64(n-1)
}

// heartColumns are the regressors of a line (when withRamp) or a constant, plus the heartbeat sinusoid when known.
func heartColumns(n int, heartHz, fs float64, withRamp bool) [][]float64 {
	ones := make([]float64, n)
	for k := range ones {
		ones[k] = 1
	}
	cols := [][]float64{ones}
	if withRamp {
		ramp := make([]float64, n)
		for k := range ramp {
			ramp[k] = float64(k)
		}
		cols = append(cols, ramp)
	}
	if heartHz > 0 {
		omega := 2 * math.Pi * heartHz / fs
		hc, hs := make([]float64, n), make([]float64, n)
		for k := range hc {
			hc[k], hs[k] = math.Cos(omega*float64(k)), math.Sin(omega*float64(k))
		}
		cols = append(cols, hc, hs)
	}
	return cols
}

// heartbeatHz is the frequency at which the heartbeat visibly moves the shoulders, above the breathing band, or 0 when
// no such motion stands out.
func heartbeatHz(x []float64, fs float64) float64 {
	peak, err := dsp.DominantFrequency(x, fs, 0.6, min(3, 0.45*fs))
	if err != nil || peak.SNRdB < minHeartbeatSNRdB {
		return 0
	}
	return peak.FreqHz
}

const minHeartbeatSNRdB = 6

// paused asks whether a straight line explains the recent samples better than the breathing continued from before
// would, by a margin that is a share of how much that breathing moves in the window. Both models also get a sinusoid at
// the heartbeat frequency, so the heartbeat moving the shoulders is absorbed equally by both and cannot tip the
// balance. A window in which the predicted breathing barely moves (a turning point) is left to the hysteresis.
func paused(observed, predicted []float64, amp, heartHz float64, cfg Config) bool {
	n := len(observed)
	var mean float64
	for _, v := range predicted {
		mean += v
	}
	mean /= float64(n)
	var spread float64
	for _, v := range predicted {
		spread += (v - mean) * (v - mean)
	}
	if spread < float64(n)*cfg.HoldRatio*cfg.HoldRatio*amp*amp {
		return false
	}
	residual := make([]float64, n)
	for k := range residual {
		residual[k] = observed[k] - predicted[k]
	}
	_, errBreathing := leastSquares(residual, heartColumns(n, heartHz, cfg.SampleRateHz, false))
	_, errLine := leastSquares(observed, heartColumns(n, heartHz, cfg.SampleRateHz, true))
	// The ratio keeps noise alone, which both models explain about as badly, from passing for a pause.
	return errBreathing-errLine >= 0.5*spread && errBreathing >= 3*errLine
}

// stillFavoured is the relaxed form of paused used to keep a pause: the line does at least as well as the breathing.
func stillFavoured(observed, predicted []float64, heartHz float64, cfg Config) bool {
	n := len(observed)
	residual := make([]float64, n)
	for k := range residual {
		residual[k] = observed[k] - predicted[k]
	}
	_, errBreathing := leastSquares(residual, heartColumns(n, heartHz, cfg.SampleRateHz, false))
	_, errLine := leastSquares(observed, heartColumns(n, heartHz, cfg.SampleRateHz, true))
	return errLine <= errBreathing
}

// leastSquares fits y with a combination of the given columns and returns the coefficients and the squared error left.
func leastSquares(y []float64, columns [][]float64) ([]float64, float64) {
	p := len(columns)
	m := make([][]float64, p)
	for i := range m {
		m[i] = make([]float64, p+1)
		for j := range p {
			for k := range y {
				m[i][j] += columns[i][k] * columns[j][k]
			}
		}
		for k := range y {
			m[i][p] += columns[i][k] * y[k]
		}
	}
	coef, ok := solve(m)
	if !ok {
		coef = make([]float64, p)
	}
	var e float64
	for k, v := range y {
		for i, c := range coef {
			v -= c * columns[i][k]
		}
		e += v * v
	}
	return coef, e
}

func nextPhase(d float64, prev Phase, cfg Config) Phase {
	switch {
	case math.Abs(d) < cfg.ExitThreshold:
		return PhaseHold
	case d > cfg.EnterThreshold:
		return PhaseIn
	case d < -cfg.EnterThreshold:
		return PhaseOut
	case prev == PhaseUnknown:
		return PhaseHold
	}
	return prev
}

// sinusoid is a·cos(ωk) + b·sin(ωk) over sample index k, fitted to the last n samples of a signal.
type sinusoid struct {
	a, b, omega, amp float64
	n                int
}

func (s sinusoid) at(k int) float64 {
	return s.a*math.Cos(s.omega*float64(k)) + s.b*math.Sin(s.omega*float64(k))
}

// direction is the slope at the last sample over the peak slope: in [-1, 1], positive while rising.
func (s sinusoid) direction() float64 {
	end := s.omega * float64(s.n-1)
	return (-s.a*math.Sin(end) + s.b*math.Cos(end)) / s.amp
}

// next continues the fitted sinusoid for m samples past the segment it was fitted on.
func (s sinusoid) next(m int) []float64 {
	out := make([]float64, m)
	for i := range out {
		out[i] = s.at(s.n + i)
	}
	return out
}

// fitSinusoid fits a·cos(ωt) + b·sin(ωt) + c over the last period of x by least squares.
func fitSinusoid(x []float64, freqHz, fs float64) (sinusoid, bool) {
	n := min(len(x), max(4, int(math.Round(fs/freqHz))))
	seg := x[len(x)-n:]
	omega := 2 * math.Pi * freqHz / fs
	m := [][]float64{make([]float64, 4), make([]float64, 4), make([]float64, 4)}
	for k, v := range seg {
		basis := [3]float64{math.Cos(omega * float64(k)), math.Sin(omega * float64(k)), 1}
		for i := range 3 {
			for j := range 3 {
				m[i][j] += basis[i] * basis[j]
			}
			m[i][3] += basis[i] * v
		}
	}
	coef, ok := solve(m)
	if !ok {
		return sinusoid{}, false
	}
	s := sinusoid{a: coef[0], b: coef[1], omega: omega, amp: math.Hypot(coef[0], coef[1]), n: n}
	return s, s.amp > 0
}

// solve solves a square linear system given as an augmented matrix, by Gaussian elimination with partial pivoting.
func solve(m [][]float64) ([]float64, bool) {
	n := len(m)
	for col := range n {
		pivot := col
		for r := col + 1; r < n; r++ {
			if math.Abs(m[r][col]) > math.Abs(m[pivot][col]) {
				pivot = r
			}
		}
		if math.Abs(m[pivot][col]) < 1e-12 {
			return nil, false
		}
		m[col], m[pivot] = m[pivot], m[col]
		for r := col + 1; r < n; r++ {
			f := m[r][col] / m[col][col]
			for c := col; c <= n; c++ {
				m[r][c] -= f * m[col][c]
			}
		}
	}
	out := make([]float64, n)
	for r := n - 1; r >= 0; r-- {
		v := m[r][n]
		for c := r + 1; c < n; c++ {
			v -= m[r][c] * out[c]
		}
		out[r] = v / m[r][r]
	}
	return out, true
}

// autocorrelation is the normalised correlation of x with itself shifted by lag samples, in [-1, 1].
func autocorrelation(x []float64, lag int) float64 {
	if lag <= 0 || lag >= len(x) {
		return 0
	}
	var xy, xx, yy float64
	for i := 0; i+lag < len(x); i++ {
		xy += x[i] * x[i+lag]
		xx += x[i] * x[i]
		yy += x[i+lag] * x[i+lag]
	}
	if xx == 0 || yy == 0 {
		return 0
	}
	return xy / math.Sqrt(xx*yy)
}
