package indicators

import "dropbear/decimal"

// WWMA computes Wilder's Weighted Moving Average.
type WWMA struct {
	count   int
	k       decimal.Decimal
	smaSum  decimal.Decimal
	samples int
	Value   decimal.Decimal
}

// NewWWMA creates a new WWMA indicator for a given number of samples.
func NewWWMA(count int) *WWMA {
	return &WWMA{
		count: count,
		k:     decimal.One.DivInt(count),
	}
}

// IsReady returns true when the Value is ready to be used.
// Before it's ready your Value is a simple moving average.
func (w *WWMA) IsReady() bool {
	return w.samples >= w.count
}

// Progress returns the progress towards being ready as a decimal between 0 and 1.
func (w *WWMA) Progress() float64 {
	if w.samples >= w.count {
		return 1.0
	}
	return float64(w.samples) / float64(w.count)
}

// Add adds a value to the indicator.
func (w *WWMA) Add(value decimal.Decimal) {
	w.samples++

	// during warmup, use simple moving average
	if w.samples <= w.count {
		w.smaSum = w.smaSum.Add(value)
		w.Value = w.smaSum.DivInt(w.samples)
		return
	}

	// after warmup use wilder smoothing
	// wwma = value * k + wwma * (1 - k)
	w.Value = value.Mul(w.k).Add(w.Value.Mul(decimal.One.Sub(w.k)))
}
