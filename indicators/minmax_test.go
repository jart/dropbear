package indicators

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"math/rand"
	"testing"
)

const realWorldIndicatorSize = 200 * clocky.Minute

var (
	one = decimal.FromInt(1)
	two = decimal.FromInt(2)
)

// naiveMax is the O(n) reference implementation for time-based windows.
type naiveMax struct {
	entries  []naiveEntry
	duration clocky.Duration
	Value    decimal.Decimal
}

type naiveEntry struct {
	ts    clocky.Time
	value decimal.Decimal
}

func newNaiveMax(duration clocky.Duration) *naiveMax {
	return &naiveMax{duration: duration}
}

func (m *naiveMax) IsReady() bool {
	return len(m.entries) > 0
}

func (m *naiveMax) Add(ts clocky.Time, price decimal.Decimal) {
	m.entries = append(m.entries, naiveEntry{ts, price})
	// remove old entries
	windowStart := clocky.Time(int64(ts) - int64(m.duration))
	for len(m.entries) > 0 && m.entries[0].ts < windowStart {
		m.entries = m.entries[1:]
	}
	// find max
	m.Value = m.entries[0].value
	for _, e := range m.entries[1:] {
		m.Value = m.Value.Max(e.value)
	}
}

func TestMax_SingleEntry(t *testing.T) {
	m := NewMax(clocky.Minute)
	m.Add(0, one)
	if m.Value.Cmp(one) != 0 {
		t.Errorf("got %s, want 1", m.Value.String())
	}
	m.Add(clocky.Time(clocky.Minute/2), two)
	if m.Value.Cmp(two) != 0 {
		t.Errorf("got %s, want 2", m.Value.String())
	}
	// one minute later, first entry falls out
	m.Add(clocky.Time(clocky.Minute+1), one)
	if m.Value.Cmp(two) != 0 {
		t.Errorf("got %s, want 2 (still in window)", m.Value.String())
	}
	// another half minute, two falls out
	m.Add(clocky.Time(clocky.Minute+clocky.Minute/2+1), one)
	if m.Value.Cmp(one) != 0 {
		t.Errorf("got %s, want 1", m.Value.String())
	}
}

func TestMax_MatchesNaive(t *testing.T) {
	data := []decimal.Decimal{
		decimal.FromInt(10),
		decimal.FromInt(5),
		decimal.FromInt(8),
		decimal.FromInt(3),
		decimal.FromInt(12),
		decimal.FromInt(7),
		decimal.FromInt(2),
		decimal.FromInt(9),
		decimal.FromInt(4),
		decimal.FromInt(11),
	}
	for _, windowMinutes := range []int{1, 2, 3, 5, 10} {
		duration := clocky.Duration(windowMinutes) * clocky.Minute
		naive := newNaiveMax(duration)
		deque := NewMax(duration)
		for i, price := range data {
			ts := clocky.Time(i) * clocky.Time(clocky.Minute)
			naive.Add(ts, price)
			deque.Add(ts, price)
			if deque.Value.Cmp(naive.Value) != 0 {
				t.Errorf("window=%d step %d: deque=%s, naive=%s",
					windowMinutes, i, deque.Value.String(), naive.Value.String())
			}
		}
	}
}

func TestMax_RandomData(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	duration := 20 * clocky.Minute
	naive := newNaiveMax(duration)
	deque := NewMax(duration)
	ts := clocky.Time(0)
	for i := range 10000 {
		price := decimal.FromInt(rng.Intn(10))
		naive.Add(ts, price)
		deque.Add(ts, price)
		if deque.Value.Cmp(naive.Value) != 0 {
			t.Errorf("step %d: deque=%s, naive=%s",
				i, deque.Value.String(), naive.Value.String())
		}
		ts = ts.Add(clocky.Minute)
	}
}

func TestMax_IsReady(t *testing.T) {
	m := NewMax(3 * clocky.Minute)
	if m.IsReady() {
		t.Error("should not be ready initially")
	}
	m.Add(0, decimal.FromInt(1))
	if m.IsReady() {
		t.Error("should not be ready after first sample")
	}
	m.Add(clocky.Time(2*clocky.Minute), decimal.FromInt(1))
	if m.IsReady() {
		t.Error("should not be ready after second sample")
	}
	m.Add(clocky.Time(3*clocky.Minute), decimal.FromInt(1))
	if !m.IsReady() {
		t.Error("should be ready after duration")
	}
}

func TestMax_AscendingInput(t *testing.T) {
	// When input is strictly ascending, the max should always be the latest value
	m := NewMax(5 * clocky.Minute)
	ts := clocky.Time(0)
	for i := 1; i <= 10; i++ {
		m.Add(ts, decimal.FromInt(i))
		if m.Value.Cmp(decimal.FromInt(i)) != 0 {
			t.Errorf("step %d: got %s, want %d", i, m.Value.String(), i)
		}
		ts = ts.Add(clocky.Minute)
	}
}

func TestMax_ConstantInput(t *testing.T) {
	m := NewMax(5 * clocky.Minute)
	ts := clocky.Time(0)
	for i := range 20 {
		m.Add(ts, decimal.FromInt(100))
		if m.Value.Cmp(decimal.FromInt(100)) != 0 {
			t.Errorf("step %d: got %s, want 100", i, m.Value.String())
		}
		ts = ts.Add(clocky.Minute)
	}
}

func TestMax_WindowSliding(t *testing.T) {
	// Test that old maximum correctly falls out of window
	// 3 minute window, entries at 0, 1, 2, 4 minutes
	m := NewMax(3 * clocky.Minute)
	ts := clocky.Time(0)
	m.Add(ts, decimal.FromInt(20))
	if m.Value.Cmp(decimal.FromInt(20)) != 0 {
		t.Errorf("step 0: got %s, want 20", m.Value.String())
	}
	m.Add(ts.Add(clocky.Minute), decimal.FromInt(15))
	if m.Value.Cmp(decimal.FromInt(20)) != 0 {
		t.Errorf("step 1: got %s, want 20", m.Value.String())
	}
	m.Add(ts.Add(2*clocky.Minute), decimal.FromInt(10))
	if m.Value.Cmp(decimal.FromInt(20)) != 0 {
		t.Errorf("step 2: got %s, want 20", m.Value.String())
	}
	// at 4 minutes, entry at 0 falls out (0 < 4-3=1)
	m.Add(ts.Add(4*clocky.Minute), decimal.FromInt(5))
	if m.Value.Cmp(decimal.FromInt(15)) != 0 {
		t.Errorf("step 3: got %s, want 15", m.Value.String())
	}
}

func TestMax_IrregularTimestamps(t *testing.T) {
	// Test with irregular/sparse data like real trades
	m := NewMax(10 * clocky.Minute)
	// Burst of trades at start
	m.Add(0, decimal.FromInt(100))
	m.Add(clocky.Time(clocky.Millisecond), decimal.FromInt(105))   // 1ms later
	m.Add(clocky.Time(2*clocky.Millisecond), decimal.FromInt(103)) // 2ms later
	m.Add(clocky.Time(5*clocky.Minute), decimal.FromInt(110))      // 5 minutes later
	m.Add(clocky.Time(8*clocky.Minute), decimal.FromInt(95))       // 8 minutes later
	m.Add(clocky.Time(12*clocky.Minute), decimal.FromInt(108))     // 12 minutes - first entries fall out
	if m.Value.Cmp(decimal.FromInt(110)) != 0 {
		t.Errorf("got %s, want 110", m.Value.String())
	}
}

func TestMinMax_CandlePreseeding(t *testing.T) {
	// Simulates preseeding from historical candles like mm does on startup.
	// 343 candles spanning 348 minutes should make a 42m indicator ready.
	window := 42 * clocky.Minute
	min := NewMin(window)
	max := NewMax(window)

	// Simulate candles from 13:01 to 18:49 (348 minute span)
	startTime := clocky.MustParseTime("2025-12-25T13:01:00")
	endTime := clocky.MustParseTime("2025-12-25T18:49:00")
	span := endTime.Sub(startTime)
	if span < window {
		t.Fatalf("test data span %s < window %s", span, window)
	}

	// Add first candle
	min.Add(startTime, decimal.Parse("100"))
	max.Add(startTime, decimal.Parse("100"))
	if min.IsReady() || max.IsReady() {
		t.Error("should not be ready after single candle")
	}

	// Add last candle
	min.Add(endTime, decimal.Parse("110"))
	max.Add(endTime, decimal.Parse("110"))
	if !min.IsReady() {
		t.Errorf("min should be ready after span %s >= window %s", span, window)
	}
	if !max.IsReady() {
		t.Errorf("max should be ready after span %s >= window %s", span, window)
	}
}

func BenchmarkMaxAdd_Naive(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	data := make([]decimal.Decimal, 1000)
	for i := range data {
		data[i] = decimal.FromInt(rng.Intn(10))
	}
	m := newNaiveMax(realWorldIndicatorSize)
	// warm up
	for i := range 200 {
		m.Add(clocky.Time(i)*clocky.Time(clocky.Minute), data[i%len(data)])
	}
	ts := clocky.Time(200 * clocky.Minute)
	i := 0
	for b.Loop() {
		m.Add(ts, data[i%len(data)])
		ts = ts.Add(clocky.Minute)
		i++
	}
}

func BenchmarkMaxAdd_Deque(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	data := make([]decimal.Decimal, 1000)
	for i := range data {
		data[i] = decimal.FromInt(rng.Intn(10))
	}
	m := NewMax(realWorldIndicatorSize)
	// warm up
	for i := range 200 {
		m.Add(clocky.Time(i)*clocky.Time(clocky.Minute), data[i%len(data)])
	}
	ts := clocky.Time(200 * clocky.Minute)
	i := 0
	for b.Loop() {
		m.Add(ts, data[i%len(data)])
		ts = ts.Add(clocky.Minute)
		i++
	}
}

func BenchmarkMaxAdd_AllOnes(b *testing.B) {
	m := NewMax(realWorldIndicatorSize)
	ts := clocky.Time(0)
	for b.Loop() {
		m.Add(ts, decimal.One)
		ts = ts.Add(clocky.Minute)
	}
}

func BenchmarkMaxAdd_Sequential(b *testing.B) {
	m := NewMax(realWorldIndicatorSize)
	ts := clocky.Time(0)
	i := 0
	for b.Loop() {
		m.Add(ts, decimal.FromInt(i))
		ts = ts.Add(clocky.Minute)
		i++
	}
}

func BenchmarkMaxAdd_Reverse(b *testing.B) {
	m := NewMax(realWorldIndicatorSize)
	ts := clocky.Time(0)
	i := 0
	for b.Loop() {
		m.Add(ts, decimal.FromInt(-i))
		ts = ts.Add(clocky.Minute)
		i++
	}
}
