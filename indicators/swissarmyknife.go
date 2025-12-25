package indicators

import (
	"dropbear/decimal"
	"math"
)

// FilterType specifies which DSP filter to use.
type FilterType int

const (
	FilterGauss FilterType = iota
	FilterButter
	FilterHighPass
	FilterTwoPoleHighPass
	FilterBandPass
)

// SwissArmyKnife implements John Ehlers' digital signal processing filters.
// These are IIR (Infinite Impulse Response) filters that run in O(1) per sample.
//
// Reference: https://www.mesasoftware.com/papers/SwissArmyKnife.pdf
type SwissArmyKnife struct {
	period  int
	samples int

	// filter coefficients (float64 for efficiency)
	c0 float64
	b0 float64
	b1 float64
	b2 float64
	a1 float64
	a2 float64

	// ring buffer for prices (newest at idx 0)
	price [3]float64

	// ring buffer for outputs (newest at idx 0)
	filt [2]float64

	Value decimal.Decimal
}

// NewSwissArmyKnife creates a new DSP filter indicator.
// period controls the cutoff frequency (higher = smoother/slower).
// delta is the bandwidth parameter for BandPass filter (ignored for others).
func NewSwissArmyKnife(filterType FilterType, period int, delta float64) *SwissArmyKnife {
	s := &SwissArmyKnife{period: period, b0: 1}

	twoPi := 2 * math.Pi
	periodF := float64(period)

	switch filterType {
	case FilterGauss:
		// Two-pole Gaussian smoothing
		beta := 2.415 * (1 - math.Cos(twoPi/periodF))
		alpha := -beta + math.Sqrt(beta*beta+2*beta)
		oneMinusAlpha := 1 - alpha
		s.c0 = alpha * alpha
		s.a1 = 2 * oneMinusAlpha
		s.a2 = -oneMinusAlpha * oneMinusAlpha

	case FilterButter:
		// Butterworth - maximally flat frequency response
		beta := 2.415 * (1 - math.Cos(twoPi/periodF))
		alpha := -beta + math.Sqrt(beta*beta+2*beta)
		oneMinusAlpha := 1 - alpha
		s.c0 = alpha * alpha / 4
		s.b1 = 2
		s.b2 = 1
		s.a1 = 2 * oneMinusAlpha
		s.a2 = -oneMinusAlpha * oneMinusAlpha

	case FilterHighPass:
		// Single-pole high pass - removes low frequency (trend)
		cosine := math.Cos(twoPi / periodF)
		sine := math.Sin(twoPi / periodF)
		alpha := (cosine + sine - 1) / cosine
		s.c0 = (1 + alpha) / 2
		s.b1 = -1
		s.a1 = 1 - alpha

	case FilterTwoPoleHighPass:
		// Two-pole high pass - sharper cutoff
		beta := 2.415 * (1 - math.Cos(twoPi/periodF))
		alpha := -beta + math.Sqrt(beta*beta+2*beta)
		oneMinusAlpha := 1 - alpha
		onePlusAlpha := 1 + alpha
		s.c0 = onePlusAlpha * onePlusAlpha / 4
		s.b1 = -2
		s.b2 = 1
		s.a1 = 2 * oneMinusAlpha
		s.a2 = -oneMinusAlpha * oneMinusAlpha

	case FilterBandPass:
		// Band pass - isolates specific frequency band
		beta := math.Cos(twoPi / periodF)
		gamma := 1 / math.Cos(4*math.Pi*delta/periodF)
		alpha := gamma - math.Sqrt(gamma*gamma-1)
		s.c0 = (1 - alpha) / 2
		s.b2 = -1
		s.a1 = -beta * (1 - alpha)
		s.a2 = alpha
	}

	return s
}

// NewGaussFilter creates a Gaussian smoothing filter.
func NewGaussFilter(period int) *SwissArmyKnife {
	return NewSwissArmyKnife(FilterGauss, period, 0)
}

// NewButterFilter creates a Butterworth low-pass filter.
func NewButterFilter(period int) *SwissArmyKnife {
	return NewSwissArmyKnife(FilterButter, period, 0)
}

// NewHighPassFilter creates a single-pole high-pass filter.
func NewHighPassFilter(period int) *SwissArmyKnife {
	return NewSwissArmyKnife(FilterHighPass, period, 0)
}

// NewTwoPoleHighPassFilter creates a two-pole high-pass filter.
func NewTwoPoleHighPassFilter(period int) *SwissArmyKnife {
	return NewSwissArmyKnife(FilterTwoPoleHighPass, period, 0)
}

// NewBandPassFilter creates a band-pass filter.
// delta controls bandwidth (typically 0.1 to 0.5).
func NewBandPassFilter(period int, delta float64) *SwissArmyKnife {
	return NewSwissArmyKnife(FilterBandPass, period, delta)
}

// IsReady returns true when the filter has enough samples.
func (s *SwissArmyKnife) IsReady() bool {
	return s.samples >= s.period
}

// Add adds a price sample and updates the filter output.
func (s *SwissArmyKnife) Add(price decimal.Decimal) {
	s.samples++
	p := price.Float64()

	// shift price buffer
	s.price[2] = s.price[1]
	s.price[1] = s.price[0]
	s.price[0] = p

	// during warmup, initialize with price to avoid transients
	if s.samples <= 3 {
		s.Value = price
		s.filt[1] = p
		s.filt[0] = p
		return
	}

	// IIR filter equation:
	// signal = c0 * (b0*price[0] + b1*price[1] + b2*price[2]) + a1*filt[0] + a2*filt[1]
	feedForward := s.b0*s.price[0] + s.b1*s.price[1] + s.b2*s.price[2]
	feedBack := s.a1*s.filt[0] + s.a2*s.filt[1]
	output := s.c0*feedForward + feedBack

	// shift output buffer
	s.filt[1] = s.filt[0]
	s.filt[0] = output

	s.Value = decimal.Decimal(int64(output * 1e9))
}
