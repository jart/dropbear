package indicators

import "dropbear/decimal"

// Welford computes rolling standard deviation using Welford's algorithm.
// This provides O(1) updates for mean and variance over a fixed window.
type Welford struct {
	period int               // window size
	ring   []decimal.Decimal // circular buffer of values
	idx    int               // current position in ring buffer
	n      int               // number of samples added (up to period)
	mean   decimal.Decimal   // current mean
	m2     decimal.Decimal   // sum of squared deviations from mean
	Value  decimal.Decimal   // current stddev (only valid when IsReady)
}

// NewWelford creates a new Welford indicator for rolling stddev over period samples.
func NewWelford(period int) *Welford {
	if period < 2 {
		panic("welford period must be at least 2")
	}
	return &Welford{
		period: period,
		ring:   make([]decimal.Decimal, period),
	}
}

// Add adds a value to the indicator, updating mean and stddev.
func (w *Welford) Add(value decimal.Decimal) {
	if w.n < w.period {
		// warmup phase: accumulate samples
		w.ring[w.idx] = value
		w.idx = (w.idx + 1) % w.period
		w.n++

		// update mean incrementally
		delta := value.Sub(w.mean)
		w.mean = w.mean.Add(delta.DivInt(w.n))
		delta2 := value.Sub(w.mean)
		w.m2 = w.m2.Add(delta.Mul(delta2))

		// compute stddev if we have enough samples
		if w.n >= 2 {
			variance := w.m2.DivInt(w.n)
			w.Value = variance.Sqrt()
		}
	} else {
		// rolling phase: remove oldest, add newest
		oldest := w.ring[w.idx]
		w.ring[w.idx] = value
		w.idx = (w.idx + 1) % w.period

		// update mean for removal and addition
		oldMean := w.mean
		w.mean = w.mean.Add(value.Sub(oldest).DivInt(w.period))

		// update m2: remove oldest contribution, add newest
		// the formula is: m2 += (value - oldest) * (value - mean + oldest - oldMean)
		w.m2 = w.m2.Add(value.Sub(oldest).Mul(value.Sub(w.mean).Add(oldest.Sub(oldMean))))

		// clamp m2 to zero if numerical error made it negative
		if w.m2.IsNegative() {
			w.m2 = decimal.Zero
		}

		// compute stddev
		variance := w.m2.DivInt(w.period)
		w.Value = variance.Sqrt()
	}
}

// Mean returns the current mean of values in the window.
func (w *Welford) Mean() decimal.Decimal {
	return w.mean
}

// Stddev returns the current standard deviation.
func (w *Welford) Stddev() decimal.Decimal {
	return w.Value
}

// IsReady returns true when enough samples have been collected.
func (w *Welford) IsReady() bool {
	return w.n >= w.period
}

// Progress returns progress towards being ready as a value between 0 and 1.
func (w *Welford) Progress() float64 {
	if w.n >= w.period {
		return 1.0
	}
	return float64(w.n) / float64(w.period)
}
