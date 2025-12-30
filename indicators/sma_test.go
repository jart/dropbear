package indicators

import (
	"dropbear/decimal"
	"testing"
)

func TestSMA(t *testing.T) {
	sma := NewSMA(3)

	// Not ready initially
	if sma.IsReady() {
		t.Error("SMA should not be ready with 0 samples")
	}

	// Add first value
	sma.Add(decimal.FromInt(10))
	if sma.Count != 1 {
		t.Errorf("expected count 1, got %d", sma.Count)
	}
	if sma.Value().Cmp(decimal.FromInt(10)) != 0 {
		t.Errorf("expected SMA 10, got %s", sma.Value())
	}

	// Add second value
	sma.Add(decimal.FromInt(20))
	if sma.Count != 2 {
		t.Errorf("expected count 2, got %d", sma.Count)
	}
	// (10 + 20) / 2 = 15
	if sma.Value().Cmp(decimal.FromInt(15)) != 0 {
		t.Errorf("expected SMA 15, got %s", sma.Value())
	}

	// Add third value - now ready
	sma.Add(decimal.FromInt(30))
	if !sma.IsReady() {
		t.Error("SMA should be ready with 3 samples")
	}
	// (10 + 20 + 30) / 3 = 20
	if sma.Value().Cmp(decimal.FromInt(20)) != 0 {
		t.Errorf("expected SMA 20, got %s", sma.Value())
	}

	// Add fourth value - should drop first
	sma.Add(decimal.FromInt(40))
	// (20 + 30 + 40) / 3 = 30
	if sma.Value().Cmp(decimal.FromInt(30)) != 0 {
		t.Errorf("expected SMA 30, got %s", sma.Value())
	}

	// Add fifth value
	sma.Add(decimal.FromInt(50))
	// (30 + 40 + 50) / 3 = 40
	if sma.Value().Cmp(decimal.FromInt(40)) != 0 {
		t.Errorf("expected SMA 40, got %s", sma.Value())
	}
}

func TestSMAWithDecimals(t *testing.T) {
	sma := NewSMA(2)

	sma.Add(decimal.Parse("100.50"))
	sma.Add(decimal.Parse("101.50"))

	// (100.50 + 101.50) / 2 = 101.00
	expected := decimal.Parse("101")
	if sma.Value().Cmp(expected) != 0 {
		t.Errorf("expected SMA %s, got %s", expected, sma.Value())
	}
}

func BenchmarkSMA(b *testing.B) {
	sma := NewSMA(20)
	price := decimal.FromInt(100)
	for b.Loop() {
		sma.Add(price)
	}
}

// TestSMA_HasNoBias verifies that SMA division doesn't accumulate
// systematic rounding bias over many operations.
func TestSMA_HasNoBias(t *testing.T) {
	// Use period=2 so every Value() call divides by 2.
	// Feed odd nano-unit values to create tie situations.
	sma := NewSMA(2)

	var smaSum int64
	var trueSum int64

	// Feed 1000 pairs of values that create ties when averaged
	for i := 1; i <= 1000; i++ {
		// Add two values that sum to an odd number (creates tie when /2)
		// e.g., 0 + 1 = 1, 0 + 3 = 3, etc.
		sma.Add(decimal.Decimal(0))
		sma.Add(decimal.Decimal(2*i - 1))

		trueSum += int64(2*i - 1)
		smaSum += int64(sma.Value())
	}

	// True average of all the (0 + odd)/2 values
	// Each true average is (2i-1)/2 = i - 0.5
	// But we're summing the SMA outputs which should ideally sum to trueSum/2
	trueHalf := trueSum / 2

	bias := smaSum - trueHalf
	if bias != 0 {
		t.Errorf("SMA has systematic bias: sum=%d, expected=%d, bias=%d", smaSum, trueHalf, bias)
	}
}
