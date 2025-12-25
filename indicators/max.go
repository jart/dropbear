package indicators

import (
	"dropbear/clocky"
	"dropbear/decimal"
)

// Max tracks the maximum price over a time window.
type Max struct {
	m     *minmax
	Value decimal.Decimal
}

// NewMax creates a new Max indicator with the given duration in microseconds.
func NewMax(duration clocky.Duration) *Max {
	return &Max{
		m: newMinmax(duration, func(a, b decimal.Decimal) bool {
			return a.Cmp(b) >= 0
		}),
	}
}

// IsReady returns true once we've run the full duration.
func (m *Max) IsReady() bool {
	return m.m.IsReady()
}

// Add adds a price at the given timestamp and updates the maximum.
func (m *Max) Add(ts clocky.Time, price decimal.Decimal) {
	m.m.Add(ts, price)
	m.Value = m.m.Value
}
