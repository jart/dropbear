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
	Chain *Options
}

func (s *Strike) String() string {
	return s.Price.String()
}

func (s *Strike) IsReady() bool {
	return s.Call != nil && s.Put != nil
}

// Probability returns the smoothed chance (e.g. 0.01 for 1%) that
// options expire at this strike. Raw Breeden-Litzenberger probabilities
// are quantized to ~1% steps due to tick size relative to strike spacing.
// We apply a [1,2,1]/4 kernel to reconstruct the smooth underlying
// distribution that the exchange tick sizes obscure.
func (s *Strike) Probability() decimal.Decimal {
	c := s.rawProbability()
	if !c.IsPositive() {
		return decimal.Zero
	}
	l := decimal.Zero
	r := decimal.Zero
	if s.Prev != nil {
		l = s.Prev.rawProbability()
	}
	if s.Next != nil {
		r = s.Next.rawProbability()
	}
	// [1, 2, 1] / 4 Gaussian kernel
	return l.Add(c.MulInt(2)).Add(r).DivInt(4)
}

// rawProbability returns the unsmoothed probability at this strike,
// interpolating from neighbors if the local butterfly is non-positive.
func (s *Strike) rawProbability() decimal.Decimal {
	p := s.computeProbability()
	if p.IsPositive() {
		return p
	}
	if !s.IsReady() || s.Prev == nil || s.Next == nil {
		return decimal.Zero
	}
	l := s.Prev.computeProbability()
	r := s.Next.computeProbability()
	if l.IsPositive() && r.IsPositive() {
		return l.Add(r).DivInt(2)
	}
	return decimal.Zero
}

// computeProbability returns the probability density at this strike,
// which is the second derivative of call/put price with respect to strike.
func (s *Strike) computeProbability() decimal.Decimal {
	// Breeden and Litzenberger say risk-neutral density is
	// second derivative of call (or put) price w.r.t. strike.
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
		a = s.Prev.Call.MidPrice()
		b = s.Call.MidPrice()
		c = s.Next.Call.MidPrice()
	} else {
		a = s.Prev.Put.MidPrice()
		b = s.Put.MidPrice()
		c = s.Next.Put.MidPrice()
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
	return s.Price.Add(s.Call.MidPrice()).Sub(s.Put.MidPrice())
}
