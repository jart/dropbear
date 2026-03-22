package options

import (
	"dropbear/decimal"
)

type Strike struct {
	Call *Option
	Put  *Option
}

func (s *Strike) IsReady() bool {
	return s.Call != nil && s.Put != nil
}

func (s *Strike) Strike() decimal.Decimal {
	if s.Call != nil {
		return s.Call.Strike
	}
	return s.Put.Strike
}

// UnderlyingPrice returns the current inferred spot price.
func (s *Strike) UnderlyingPrice() decimal.Decimal {
	return s.Call.Strike.Add(s.Call.MarketPrice()).Sub(s.Put.MarketPrice())
}
