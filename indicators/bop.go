package indicators

import "dropbear/decimal"

// BOP computes the Balance of Power indicator.
// BOP = (Close - Open) / (High - Low), smoothed with WWMA.
// Ranges from -1 (sellers in control) to +1 (buyers in control).
type BOP struct {
	wwma  *WWMA
	Value decimal.Decimal
}

// NewBOP creates a new BOP indicator with the given smoothing period.
func NewBOP(period int) *BOP {
	return &BOP{
		wwma: NewWWMA(period),
	}
}

// IsReady returns true when the Value is ready to be used.
func (b *BOP) IsReady() bool {
	return b.wwma.IsReady()
}

// Add adds a candle to the indicator.
func (b *BOP) Add(c *Candle) {
	rang := c.High.Sub(c.Low)
	if rang.IsZero() {
		return
	}
	bop := c.Close.Sub(c.Open).DivEven(rang)
	b.wwma.Add(bop)
	b.Value = b.wwma.Value
}
