package indicators

import (
	"dropbear/decimal"
	"testing"
)

// TestWWMA_MatchesQuantConnect verifies our WWMA matches QuantConnect's implementation.
// QuantConnect formula (from Lean/Indicators/WilderMovingAverage.cs):
//   - During warmup (first period samples): SMA
//   - After warmup: WWMA = input * k + previous * (1 - k), where k = 1/period
func TestWWMA_MatchesQuantConnect(t *testing.T) {
	// Period 3, values: 10, 11, 12, 13, 14
	// Hand calculations:
	//   Step 1: samples=1, SMA = 10/1 = 10
	//   Step 2: samples=2, SMA = (10+11)/2 = 10.5
	//   Step 3: samples=3, SMA = (10+11+12)/3 = 11 (warmup complete)
	//   Step 4: k=1/3, WWMA = 13*(1/3) + 11*(2/3) = 4.333... + 7.333... = 11.666...
	//   Step 5: k=1/3, WWMA = 14*(1/3) + 11.666...*(2/3) = 4.666... + 7.777... = 12.444...
	w := NewWWMA(3)

	w.Add(decimal.FromInt(10))
	assertDecimalClose(t, "step 1", w.Value, decimal.FromInt(10))
	if w.IsReady() {
		t.Error("should not be ready after 1 sample")
	}

	w.Add(decimal.FromInt(11))
	assertDecimalClose(t, "step 2", w.Value, decimal.Parse("10.5"))
	if w.IsReady() {
		t.Error("should not be ready after 2 samples")
	}

	w.Add(decimal.FromInt(12))
	assertDecimalClose(t, "step 3", w.Value, decimal.FromInt(11))
	if !w.IsReady() {
		t.Error("should be ready after 3 samples")
	}

	w.Add(decimal.FromInt(13))
	// 13/3 + 11*2/3 = 13/3 + 22/3 = 35/3 = 11.666...
	assertDecimalClose(t, "step 4", w.Value, decimal.Parse("11.666666666666666666"))

	w.Add(decimal.FromInt(14))
	// 14/3 + (35/3)*2/3 = 14/3 + 70/9 = 42/9 + 70/9 = 112/9 = 12.444...
	assertDecimalClose(t, "step 5", w.Value, decimal.Parse("12.444444444444444444"))
}

func TestWWMA_Period14(t *testing.T) {
	// Standard ATR period, verify the smoothing factor
	w := NewWWMA(14)

	// Feed 14 values to complete warmup
	for i := 1; i <= 14; i++ {
		w.Add(decimal.FromInt(i))
	}
	if !w.IsReady() {
		t.Error("should be ready after 14 samples")
	}
	// SMA of 1..14 = (1+14)*14/2 / 14 = 7.5
	assertDecimalClose(t, "warmup SMA", w.Value, decimal.Parse("7.5"))

	// Add 15, k = 1/14
	// WWMA = 15/14 + 7.5*13/14 = 15/14 + 97.5/14 = 112.5/14 = 8.035714...
	w.Add(decimal.FromInt(15))
	assertDecimalClose(t, "after warmup", w.Value, decimal.Parse("8.035714285714285714"))
}

func TestWWMA_ConstantInput(t *testing.T) {
	// If input is constant, WWMA should converge to that constant
	w := NewWWMA(5)
	for i := 0; i < 20; i++ {
		w.Add(decimal.FromInt(100))
	}
	assertDecimalClose(t, "constant input", w.Value, decimal.FromInt(100))
}

func TestWWMA_SmoothingFactor(t *testing.T) {
	// Verify k = 1/period by checking the decay rate
	// After warmup with value X, adding Y should give: X + (Y-X)/period
	w := NewWWMA(10)

	// Warmup with 100
	for i := 0; i < 10; i++ {
		w.Add(decimal.FromInt(100))
	}
	if !w.IsReady() {
		t.Error("should be ready")
	}
	assertDecimalClose(t, "warmup", w.Value, decimal.FromInt(100))

	// Add 200: new = 200/10 + 100*9/10 = 20 + 90 = 110
	w.Add(decimal.FromInt(200))
	assertDecimalClose(t, "after spike", w.Value, decimal.FromInt(110))

	// Add 200 again: new = 200/10 + 110*9/10 = 20 + 99 = 119
	w.Add(decimal.FromInt(200))
	assertDecimalClose(t, "second spike", w.Value, decimal.FromInt(119))
}

func assertDecimalClose(t *testing.T, name string, got, want decimal.Decimal) {
	t.Helper()
	diff := got.Sub(want).Abs()
	tolerance := decimal.Parse("0.00000001") // 8 decimal places
	if diff.Cmp(tolerance) > 0 {
		t.Errorf("%s: got %s, want %s (diff %s)", name, got.String(), want.String(), diff.String())
	}
}
