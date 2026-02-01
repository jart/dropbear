package indicators

import "dropbear/decimal"

// EMA computes an Exponential Moving Average.
type EMA struct {
	period  int
	k       decimal.Decimal // smoothing factor 2/(period+1)
	samples int
	Value   decimal.Decimal
}

// NewEMA creates a new EMA indicator for a given period.
func NewEMA(period int) *EMA {
	return &EMA{
		period: period,
		k:      decimal.FromInt(2).DivInt(period + 1),
	}
}

// IsReady returns true when the EMA has enough samples.
func (e *EMA) IsReady() bool {
	return e.samples >= e.period
}

// Add adds a value to the indicator.
func (e *EMA) Add(value decimal.Decimal) {
	e.samples++
	if e.samples == 1 {
		e.Value = value
		return
	}
	// EMA = value * k + EMA * (1 - k)
	e.Value = value.Mul(e.k).Add(e.Value.Mul(decimal.One.Sub(e.k)))
}

// DEMA computes a Double Exponential Moving Average (reduced lag).
type DEMA struct {
	ema1    *EMA
	ema2    *EMA
	samples int
	Value   decimal.Decimal
}

// NewDEMA creates a new DEMA indicator for a given period.
func NewDEMA(period int) *DEMA {
	return &DEMA{
		ema1: NewEMA(period),
		ema2: NewEMA(period),
	}
}

// IsReady returns true when the DEMA has enough samples.
func (d *DEMA) IsReady() bool {
	return d.ema2.IsReady()
}

// Add adds a value to the indicator.
func (d *DEMA) Add(value decimal.Decimal) {
	d.samples++
	d.ema1.Add(value)
	d.ema2.Add(d.ema1.Value)
	// DEMA = 2 * EMA1 - EMA2
	d.Value = d.ema1.Value.MulInt(2).Sub(d.ema2.Value)
}

// TEMA computes a Triple Exponential Moving Average (even less lag).
type TEMA struct {
	ema1    *EMA
	ema2    *EMA
	ema3    *EMA
	samples int
	Value   decimal.Decimal
}

// NewTEMA creates a new TEMA indicator for a given period.
func NewTEMA(period int) *TEMA {
	return &TEMA{
		ema1: NewEMA(period),
		ema2: NewEMA(period),
		ema3: NewEMA(period),
	}
}

// IsReady returns true when the TEMA has enough samples.
func (t *TEMA) IsReady() bool {
	return t.ema3.IsReady()
}

// Add adds a value to the indicator.
func (t *TEMA) Add(value decimal.Decimal) {
	t.samples++
	t.ema1.Add(value)
	t.ema2.Add(t.ema1.Value)
	t.ema3.Add(t.ema2.Value)
	// TEMA = 3*EMA1 - 3*EMA2 + EMA3
	t.Value = t.ema1.Value.MulInt(3).Sub(t.ema2.Value.MulInt(3)).Add(t.ema3.Value)
}
