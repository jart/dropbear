package indicators

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"math/rand"
	"testing"
)

const (
	minute                 = 60_000_000 // microseconds
	realWorldIndicatorSize = 200 * minute
)

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
	m := NewMax(minute)
	m.Add(0, one)
	if m.Value.Cmp(one) != 0 {
		t.Errorf("got %s, want 1", m.Value.String())
	}
	m.Add(minute/2, two)
	if m.Value.Cmp(two) != 0 {
		t.Errorf("got %s, want 2", m.Value.String())
	}
	// one minute later, first entry falls out
	m.Add(minute+1, one)
	if m.Value.Cmp(two) != 0 {
		t.Errorf("got %s, want 2 (still in window)", m.Value.String())
	}
	// another half minute, two falls out
	m.Add(minute+minute/2+1, one)
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
	for _, windowMinutes := range []int64{1, 2, 3, 5, 10} {
		duration := clocky.Duration(windowMinutes * minute)
		naive := newNaiveMax(duration)
		deque := NewMax(duration)
		for i, price := range data {
			ts := clocky.Time(int64(i) * minute)
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
	m := NewMax(3 * minute)
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
	m := NewMax(5 * minute)
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
	m := NewMax(5 * minute)
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
	m := NewMax(10 * minute)
	// Burst of trades at start
	m.Add(0, decimal.FromInt(100))
	m.Add(1000, decimal.FromInt(105))      // 1ms later
	m.Add(2000, decimal.FromInt(103))      // 2ms later
	m.Add(5*minute, decimal.FromInt(110))  // 5 minutes later
	m.Add(8*minute, decimal.FromInt(95))   // 8 minutes later
	m.Add(12*minute, decimal.FromInt(108)) // 12 minutes - first entries fall out
	if m.Value.Cmp(decimal.FromInt(110)) != 0 {
		t.Errorf("got %s, want 110", m.Value.String())
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
		m.Add(clocky.Time(int64(i)*int64(clocky.Minute)), data[i%len(data)])
	}
	ts := clocky.Time(int64(200 * clocky.Minute))
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
		m.Add(clocky.Time(int64(i)*int64(clocky.Minute)), data[i%len(data)])
	}
	ts := clocky.Time(int64(200 * clocky.Minute))
	i := 0
	for b.Loop() {
		m.Add(ts, data[i%len(data)])
		ts += minute
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
