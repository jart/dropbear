package indicators

import (
	"dropbear/decimal"
	"math"
	"testing"
)

func TestWelford_BasicStddev(t *testing.T) {
	// Test with simple values: 1, 2, 3, 4, 5
	// Mean = 3, Variance = 2, Stddev = sqrt(2) = 1.41421356...
	w := NewWelford(5)

	w.Add(decimal.FromInt(1))
	w.Add(decimal.FromInt(2))
	w.Add(decimal.FromInt(3))
	w.Add(decimal.FromInt(4))

	if w.IsReady() {
		t.Error("should not be ready after 4 samples for period 5")
	}

	w.Add(decimal.FromInt(5))

	if !w.IsReady() {
		t.Error("should be ready after 5 samples")
	}

	// Check mean = 3
	gotMean := w.Mean()
	wantMean := decimal.FromInt(3)
	if gotMean.Cmp(wantMean) != 0 {
		t.Errorf("mean: got %s, want %s", gotMean, wantMean)
	}

	// Check stddev = sqrt(2) ≈ 1.41421356
	gotStddev := w.Stddev()
	wantStddev := decimal.Parse("1.41421356")
	diff := gotStddev.Sub(wantStddev).Abs()
	if diff.Cmp(decimal.Parse("0.00000001")) > 0 {
		t.Errorf("stddev: got %s, want ~%s", gotStddev, wantStddev)
	}
}

func TestWelford_Rolling(t *testing.T) {
	// Test rolling window behavior
	// Window: [1, 2, 3] -> mean=2, variance=2/3, stddev=0.8165...
	// After adding 4: [2, 3, 4] -> mean=3, variance=2/3, stddev=0.8165...
	w := NewWelford(3)

	w.Add(decimal.FromInt(1))
	w.Add(decimal.FromInt(2))
	w.Add(decimal.FromInt(3))

	if !w.IsReady() {
		t.Error("should be ready after 3 samples")
	}

	// Mean should be 2
	if w.Mean().Cmp(decimal.FromInt(2)) != 0 {
		t.Errorf("mean before roll: got %s, want 2", w.Mean())
	}

	// Add 4, window becomes [2, 3, 4]
	w.Add(decimal.FromInt(4))

	// Mean should now be 3
	if w.Mean().Cmp(decimal.FromInt(3)) != 0 {
		t.Errorf("mean after roll: got %s, want 3", w.Mean())
	}

	// Stddev should be same since variance is same for consecutive integers
	// variance = ((2-3)^2 + (3-3)^2 + (4-3)^2) / 3 = (1+0+1)/3 = 2/3
	// stddev = sqrt(2/3) ≈ 0.81649658
	wantStddev := decimal.Parse("0.81649658")
	diff := w.Stddev().Sub(wantStddev).Abs()
	if diff.Cmp(decimal.Parse("0.00000001")) > 0 {
		t.Errorf("stddev after roll: got %s, want ~%s", w.Stddev(), wantStddev)
	}
}

func TestWelford_ConstantValues(t *testing.T) {
	// If all values are the same, stddev should be 0
	w := NewWelford(5)

	for i := 0; i < 10; i++ {
		w.Add(decimal.FromInt(42))
	}

	if !w.IsReady() {
		t.Error("should be ready")
	}

	if w.Mean().Cmp(decimal.FromInt(42)) != 0 {
		t.Errorf("mean: got %s, want 42", w.Mean())
	}

	if !w.Stddev().IsZero() {
		t.Errorf("stddev of constant values: got %s, want 0", w.Stddev())
	}
}

func TestWelford_Progress(t *testing.T) {
	w := NewWelford(4)

	if w.Progress() != 0.0 {
		t.Errorf("initial progress: got %f, want 0", w.Progress())
	}

	w.Add(decimal.FromInt(1))
	if w.Progress() != 0.25 {
		t.Errorf("progress after 1: got %f, want 0.25", w.Progress())
	}

	w.Add(decimal.FromInt(2))
	if w.Progress() != 0.5 {
		t.Errorf("progress after 2: got %f, want 0.5", w.Progress())
	}

	w.Add(decimal.FromInt(3))
	if w.Progress() != 0.75 {
		t.Errorf("progress after 3: got %f, want 0.75", w.Progress())
	}

	w.Add(decimal.FromInt(4))
	if w.Progress() != 1.0 {
		t.Errorf("progress after 4: got %f, want 1.0", w.Progress())
	}

	// After being ready, progress stays at 1.0
	w.Add(decimal.FromInt(5))
	if w.Progress() != 1.0 {
		t.Errorf("progress after 5: got %f, want 1.0", w.Progress())
	}
}

func TestWelford_MatchesBruteForce(t *testing.T) {
	// Compare against brute force calculation over many iterations
	w := NewWelford(10)
	values := []decimal.Decimal{}

	for i := 0; i < 50; i++ {
		val := decimal.FromInt(i * 17 % 100) // pseudo-random values
		w.Add(val)
		values = append(values, val)

		if len(values) > 10 {
			values = values[1:]
		}

		if w.IsReady() {
			// Calculate expected mean and stddev manually
			sum := decimal.Zero
			for _, v := range values {
				sum = sum.Add(v)
			}
			expectedMean := sum.DivInt(len(values))

			sumSq := decimal.Zero
			for _, v := range values {
				diff := v.Sub(expectedMean)
				sumSq = sumSq.Add(diff.Mul(diff))
			}
			expectedVar := sumSq.DivInt(len(values))
			expectedStddev := expectedVar.Sqrt()

			// Check mean
			meanDiff := w.Mean().Sub(expectedMean).Abs()
			if meanDiff.Cmp(decimal.Parse("0.00000001")) > 0 {
				t.Errorf("iteration %d mean: got %s, want %s", i, w.Mean(), expectedMean)
			}

			// Check stddev
			stddevDiff := w.Stddev().Sub(expectedStddev).Abs()
			if stddevDiff.Cmp(decimal.Parse("0.00001")) > 0 {
				t.Errorf("iteration %d stddev: got %s, want %s", i, w.Stddev(), expectedStddev)
			}
		}
	}
}

func TestWelford_PanicOnSmallPeriod(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for period < 2")
		}
	}()
	NewWelford(1)
}

func TestWelford_PercentReturns(t *testing.T) {
	// Simulate computing stddev of percent returns like we would for stink bids
	// Prices: 100, 101, 99, 102, 98 (alternating pattern)
	// Returns: 1%, -1.98%, 3.03%, -3.92%
	w := NewWelford(4)

	prices := []decimal.Decimal{
		decimal.FromInt(100),
		decimal.FromInt(101),
		decimal.FromInt(99),
		decimal.FromInt(102),
		decimal.FromInt(98),
	}

	var lastPrice decimal.Decimal
	for i, price := range prices {
		if i > 0 {
			// percent return = (price - lastPrice) / lastPrice
			ret := price.Sub(lastPrice).Div(lastPrice)
			w.Add(ret)
		}
		lastPrice = price
	}

	if !w.IsReady() {
		t.Error("should be ready after 4 returns")
	}

	// The stddev should be positive and reasonable
	stddev := w.Stddev()
	if !stddev.IsPositive() {
		t.Errorf("stddev should be positive, got %s", stddev)
	}

	// Convert to percent and check it's in a reasonable range
	pct := stddev.MulInt(100).Float64()
	if pct < 1.0 || pct > 5.0 {
		t.Errorf("stddev percent should be between 1-5%%, got %.2f%%", pct)
	}
}

func BenchmarkWelford(b *testing.B) {
	w := NewWelford(1440) // 24 hours of minute data
	v := decimal.Parse("0.001234")

	// Warm up
	for i := 0; i < 1440; i++ {
		w.Add(v)
	}

	b.ResetTimer()
	for b.Loop() {
		w.Add(v)
	}
}

func BenchmarkWelfordVsBruteForce(b *testing.B) {
	const period = 100

	b.Run("Welford", func(b *testing.B) {
		w := NewWelford(period)
		v := decimal.Parse("0.001234")
		for i := 0; i < period; i++ {
			w.Add(v)
		}
		b.ResetTimer()
		for b.Loop() {
			w.Add(v)
			_ = w.Stddev()
		}
	})

	b.Run("BruteForce", func(b *testing.B) {
		ring := make([]float64, period)
		idx := 0
		v := 0.001234
		for i := 0; i < period; i++ {
			ring[i] = v
		}
		b.ResetTimer()
		for b.Loop() {
			ring[idx] = v
			idx = (idx + 1) % period
			// Calculate mean
			sum := 0.0
			for _, x := range ring {
				sum += x
			}
			mean := sum / float64(period)
			// Calculate variance
			sumSq := 0.0
			for _, x := range ring {
				d := x - mean
				sumSq += d * d
			}
			_ = math.Sqrt(sumSq / float64(period))
		}
	})
}
