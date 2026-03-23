package options

import (
	"dropbear/decimal"
)

const kMinProbPrice = decimal.Half

type Strike struct {
	Price decimal.Decimal
	Call  *Option
	Put   *Option
	Prev  *Strike
	Next  *Strike
	Chain *Options
}

func (s *Strike) String() string {
	return s.Price.String()
}

func (s *Strike) IsReady() bool {
	return s.Call != nil && s.Put != nil
}

// Probability returns the chance (e.g. 0.01 for 1%) that options expire at this strike.
// This should return a positive value. If the data is missing or toxic that might change.
func (s *Strike) Probability() decimal.Decimal {
	// The Breeden-Litzenberger result says the risk-neutral density is the
	// second derivative of call (or puts) price with respect to strike.
	if !s.IsReady() || s.Prev == nil || s.Next == nil ||
		!s.Prev.Put.HasQuotes() || !s.Put.HasQuotes() && !s.Next.Put.HasQuotes() ||
		!s.Prev.Call.HasQuotes() || !s.Call.HasQuotes() && !s.Next.Call.HasQuotes() {
		return decimal.Zero
	}
	dk_left := s.Price.Sub(s.Prev.Price)
	dk_right := s.Next.Price.Sub(s.Price)
	var a, b, c decimal.Decimal
	// out of the money strikes tend to have more reliable prices
	if s.Price.Cmp(s.bestUnderlyingPrice()) > 0 {
		a = s.Prev.Call.MarketPrice()
		b = s.Call.MarketPrice()
		c = s.Next.Call.MarketPrice()
	} else {
		a = s.Prev.Put.MarketPrice()
		b = s.Put.MarketPrice()
		c = s.Next.Put.MarketPrice()
	}
	// prices below $1 are dominated by the tick size and produce
	// unreliable butterflies due to convexity violations
	if b.Cmp(kMinProbPrice) < 0 {
		return decimal.Zero
	}
	// we compute butterfly
	//   a/l - b*(1/l + 1/r) + c/r
	// which can be simplified to
	//   ((a-b)*r + (c-b)*l) / (l*r)
	num := a.Sub(b).Mul(dk_right).Add(c.Sub(b).Mul(dk_left))
	den := dk_left.Mul(dk_right)
	return num.Div(den).Max(decimal.Zero)
}

// bestUnderlyingPrice returns the best guess of the current underlying price.
func (s *Strike) bestUnderlyingPrice() decimal.Decimal {
	if s.Chain != nil && s.Chain.Price.IsPositive() {
		return s.Chain.Price
	}
	return s.underlyingPrice()
}

// underlyingPrice returns the current inferred spot price from this strike.
// This value could be weird if this strike isn't close to the money.
func (s *Strike) underlyingPrice() decimal.Decimal {
	if !s.IsReady() || !s.Call.HasQuotes() || !s.Put.HasQuotes() {
		return decimal.Zero
	}
	return s.Price.Add(s.Call.MarketPrice()).Sub(s.Put.MarketPrice())
}
