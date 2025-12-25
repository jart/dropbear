package indicators

import "dropbear/decimal"

// ATR computes the Average True Range indicator as defined by Welles Wilder.
//
// True Range is the maximum of:
//   - High - Low
//   - Abs(High - PreviousClose)
//   - Abs(Low - PreviousClose)
//
// ATR smooths the True Range using Wilder's moving average (k = 1/period).
type ATR struct {
	period    int
	k         decimal.Decimal // smoothing factor: 1/period
	prevClose decimal.Decimal
	smaSum    decimal.Decimal // used during warmup
	samples   int
	Value     decimal.Decimal
}

// NewATR creates a new ATR indicator with the given period.
func NewATR(period int) *ATR {
	return &ATR{
		period: period,
		k:      decimal.One.Div(decimal.FromInt(period)),
	}
}

// IsReady returns true when the Value is ready to be used.
func (a *ATR) IsReady() bool {
	return a.samples >= a.period
}

// Adds closed candle to the indicator.
func (a *ATR) Add(c *Candle) {
	// compute true range
	tr := c.High.Sub(c.Low)
	if a.samples > 0 {
		tr2 := c.High.Sub(a.prevClose).Abs()
		if tr2.Cmp(tr) > 0 {
			tr = tr2
		}
		tr3 := c.Low.Sub(a.prevClose).Abs()
		if tr3.Cmp(tr) > 0 {
			tr = tr3
		}
	}
	a.prevClose = c.Close
	a.samples++

	// during warmup, use simple moving average
	if a.samples <= a.period {
		a.smaSum = a.smaSum.Add(tr)
		a.Value = a.smaSum.Div(decimal.FromInt(a.samples))
		return
	}

	// after warmup, use Wilder's smoothing: ATR = TR * k + ATR * (1 - k)
	a.Value = tr.Mul(a.k).Add(a.Value.Mul(decimal.One.Sub(a.k)))
}

// Reset resets the indicator to its initial state.
func (a *ATR) Reset() {
	a.prevClose = decimal.Zero
	a.smaSum = decimal.Zero
	a.samples = 0
	a.Value = decimal.Zero
}
