package indicators

import "dropbear/decimal"

// SMA computes a Simple Moving Average.
type SMA struct {
	period int
	values []decimal.Decimal
	sum    decimal.Decimal
	Count  int
}

// NewSMA creates a new SMA indicator for a given period.
func NewSMA(period int) *SMA {
	return &SMA{
		period: period,
		values: make([]decimal.Decimal, 0, period),
	}
}

// IsReady returns true when the SMA has enough samples.
func (s *SMA) IsReady() bool {
	return s.Count >= s.period
}

// Add adds a value to the indicator.
func (s *SMA) Add(value decimal.Decimal) {
	s.Count++
	if len(s.values) < s.period {
		s.values = append(s.values, value)
		s.sum = s.sum.Add(value)
	} else {
		// Remove oldest value, add new one
		idx := (s.Count - 1) % s.period
		s.sum = s.sum.Sub(s.values[idx])
		s.values[idx] = value
		s.sum = s.sum.Add(value)
	}
}

// Value returns the current SMA value.
func (s *SMA) Value() decimal.Decimal {
	if len(s.values) == 0 {
		return decimal.Zero
	}
	return s.sum.Div(decimal.FromInt(len(s.values)))
}
