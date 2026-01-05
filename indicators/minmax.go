package indicators

import (
	"dropbear/clocky"
	"dropbear/decimal"
)

// minmax tracks min/max price with a monotonic deque over a time window.
type minmax struct {
	Value    decimal.Decimal
	deque    []minmaxEntry
	duration clocky.Duration
	first    clocky.Time
	last     clocky.Time
	// cmp returns true if a should replace b in the deque.
	// For min: a <= b (new smaller values push out larger ones)
	// For max: a >= b (new larger values push out smaller ones)
	cmp func(a, b decimal.Decimal) bool
}

type minmaxEntry struct {
	ts    clocky.Time
	value decimal.Decimal
}

func newMinmax(duration clocky.Duration, cmp func(a, b decimal.Decimal) bool) *minmax {
	if duration <= 0 {
		panic("duration must be positive")
	}
	return &minmax{
		deque:    make([]minmaxEntry, 0, 64),
		duration: duration,
		cmp:      cmp,
		first:    -1,
	}
}

func (m *minmax) IsReady() bool {
	return m.last.Sub(m.first) >= m.duration
}

func (m *minmax) Add(ts clocky.Time, price decimal.Decimal) {
	// figure out how long we've been running
	m.last = ts
	if m.first == -1 {
		m.first = ts
	}
	// remove from back any entries that new price dominates
	for len(m.deque) > 0 && m.cmp(price, m.deque[len(m.deque)-1].value) {
		m.deque = m.deque[:len(m.deque)-1]
	}
	// add new entry to back
	m.deque = append(m.deque, minmaxEntry{ts: ts, value: price})
	// remove from front any entries outside window
	windowStart := clocky.Time(int64(ts) - int64(m.duration))
	for len(m.deque) > 0 && m.deque[0].ts < windowStart {
		m.deque = m.deque[1:]
	}
	// front of deque has answer
	m.Value = m.deque[0].value
}
