package options

import (
	"dropbear/decimal"
)

type Strike struct {
	Price decimal.Decimal
	Call  *Option
	Put   *Option
	Prev  *Strike
	Next  *Strike
}

func (s *Strike) String() string {
	return s.Price.String()
}

func (s *Strike) IsReady() bool {
	return s.Call != nil && s.Put != nil
}

// UnderlyingPrice returns the current inferred spot price.
func (s *Strike) UnderlyingPrice() decimal.Decimal {
	if !s.IsReady() || !s.Call.HasQuotes() || !s.Put.HasQuotes() {
		return decimal.Zero
	}
	return s.Price.Add(s.Call.MarketPrice()).Sub(s.Put.MarketPrice())
}

// Probability returns the chance (e.g. 0.01 for 1%) that options expire at this strike.
func (s *Strike) Probability() decimal.Decimal {
	// The Breeden-Litzenberger result says the risk-neutral density is the
	// second derivative of call (or puts) price with respect to strike.
	if !s.IsReady() || s.Prev == nil || s.Next == nil {
		return decimal.Zero
	}
	dk_left := s.Price.Sub(s.Prev.Price)
	dk_right := s.Next.Price.Sub(s.Price)
	putHasQuotes := s.Prev.Put.HasQuotes() && s.Put.HasQuotes() && s.Next.Put.HasQuotes()
	callHasQuotes := s.Prev.Call.HasQuotes() && s.Call.HasQuotes() && s.Next.Call.HasQuotes()
	var a, b, c decimal.Decimal
	if callHasQuotes && putHasQuotes && s.Call.HasGreeks() && s.Put.HasGreeks() {
		if s.Call.Vega.Cmp(s.Put.Vega) >= 0 {
			a = s.Prev.Call.MarketPrice()
			b = s.Call.MarketPrice()
			c = s.Next.Call.MarketPrice()
		} else {
			a = s.Prev.Put.MarketPrice()
			b = s.Put.MarketPrice()
			c = s.Next.Put.MarketPrice()
		}
	} else if callHasQuotes {
		a = s.Prev.Call.MarketPrice()
		b = s.Call.MarketPrice()
		c = s.Next.Call.MarketPrice()
	} else if putHasQuotes {
		a = s.Prev.Put.MarketPrice()
		b = s.Put.MarketPrice()
		c = s.Next.Put.MarketPrice()
	} else {
		return decimal.Zero
	}
	butterfly := a.Div(dk_left).
		Sub(b.Mul(decimal.One.Div(dk_left).Add(decimal.One.Div(dk_right)))).
		Add(c.Div(dk_right))
	return butterfly
}
