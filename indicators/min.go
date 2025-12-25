package indicators

import (
	"dropbear/clocky"
	"dropbear/decimal"
)

// Min tracks the minimum price over a time window.
type Min struct {
	m     *minmax
	Value decimal.Decimal
}

// NewMin creates a new Min indicator with the given duration in microseconds.
func NewMin(duration clocky.Duration) *Min {
	return &Min{
		m: newMinmax(duration, func(a, b decimal.Decimal) bool {
			return a.Cmp(b) <= 0
		}),
	}
}

// IsReady returns true once we've run the full duration.
func (m *Min) IsReady() bool {
	return m.m.IsReady()
}

// Add adds a price at the given timestamp and updates the minimum.
func (m *Min) Add(ts clocky.Time, price decimal.Decimal) {
	m.m.Add(ts, price)
	m.Value = m.m.Value
}
