package indicators

import "dropbear/decimal"

// Spike tracks recent prices and computes the high-low range for instant volatility detection.
type Spike struct {
	buf   [8]decimal.Decimal
	idx   int
	Value decimal.Decimal
}

// NewSpike creates a new Spike indicator.
func NewSpike() *Spike {
	return &Spike{}
}

// Add adds a price and updates the Value to the high-low range of recent prices.
func (s *Spike) Add(price decimal.Decimal) {
	s.buf[s.idx%len(s.buf)] = price
	s.idx++
	var lo, hi decimal.Decimal
	for _, p := range s.buf {
		if p.IsPositive() {
			if lo.IsZero() || p.Cmp(lo) < 0 {
				lo = p
			}
			if p.Cmp(hi) > 0 {
				hi = p
			}
		}
	}
	s.Value = hi.Sub(lo)
}
