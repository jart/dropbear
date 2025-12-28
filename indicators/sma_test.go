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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sma.Add(price)
	}
}
