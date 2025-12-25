package indicators

import "dropbear/decimal"

// Momentum tracks recent price changes to detect directional bias.
// Value is positive when price is trending up, negative when trending down.
type Momentum struct {
	buf     []decimal.Decimal
	idx     int
	samples int
	Value   decimal.Decimal
}

// NewMomentum creates a new Momentum indicator.
func NewMomentum(samples int) *Momentum {
	return &Momentum{
		buf:     make([]decimal.Decimal, samples),
		samples: samples,
	}
}

// IsReady returns true when the buffer is full and Value is valid.
func (m *Momentum) IsReady() bool {
	return m.idx >= m.samples
}

// Add adds a price and updates the Value to reflect momentum.
// Momentum is computed as the difference between current price and the oldest price in the buffer.
func (m *Momentum) Add(price decimal.Decimal) {
	oldest := m.buf[m.idx%len(m.buf)]
	m.buf[m.idx%len(m.buf)] = price
	m.idx++
	if m.IsReady() {
		m.Value = price.Sub(oldest)
	}
}
