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
	assertEq(t, "step 1", w.Value, decimal.FromInt(10))
	if w.IsReady() {
		t.Error("should not be ready after 1 sample")
	}

	w.Add(decimal.FromInt(11))
	assertEq(t, "step 2", w.Value, decimal.Parse("10.5"))
	if w.IsReady() {
		t.Error("should not be ready after 2 samples")
	}

	w.Add(decimal.FromInt(12))
	assertEq(t, "step 3", w.Value, decimal.FromInt(11))
	if !w.IsReady() {
		t.Error("should be ready after 3 samples")
	}

	w.Add(decimal.FromInt(13))
	// 13/3 + 11*2/3 = 13/3 + 22/3 = 35/3 = 11.666...
	assertEq(t, "step 4", w.Value, decimal.Parse("11.66666666"))

	w.Add(decimal.FromInt(14))
	// 14/3 + (35/3)*2/3 = 14/3 + 70/9 = 42/9 + 70/9 = 112/9 = 12.444...
	assertEq(t, "step 5", w.Value, decimal.Parse("12.44444443"))
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
	assertEq(t, "warmup SMA", w.Value, decimal.Parse("7.5"))

	// Add 15, k = 1/14
	// WWMA = 15/14 + 7.5*13/14 = 15/14 + 97.5/14 = 112.5/14 = 8.035714...
	w.Add(decimal.FromInt(15))
	assertEq(t, "after warmup", w.Value, decimal.Parse("8.03571427"))
}

func TestWWMA_ConstantInput(t *testing.T) {
	// If input is constant, WWMA should converge to that constant
	w := NewWWMA(5)
	for i := 0; i < 20; i++ {
		w.Add(decimal.FromInt(100))
	}
	assertEq(t, "constant input", w.Value, decimal.FromInt(100))
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
	assertEq(t, "warmup", w.Value, decimal.FromInt(100))

	// Add 200: new = 200/10 + 100*9/10 = 20 + 90 = 110
	w.Add(decimal.FromInt(200))
	assertEq(t, "after spike", w.Value, decimal.FromInt(110))

	// Add 200 again: new = 200/10 + 110*9/10 = 20 + 99 = 119
	w.Add(decimal.FromInt(200))
	assertEq(t, "second spike", w.Value, decimal.FromInt(119))
}

func assertEq(t *testing.T, name string, got, want decimal.Decimal) {
	t.Helper()
	if got.Cmp(want) != 0 {
		t.Errorf("%s: got %s, want %s", name, got.String(), want.String())
	}
}

func BenchmarkWWMA(b *testing.B) {
	w := NewWWMA(14)
	v := decimal.Parse("123.456789")
	for b.Loop() {
		w.Add(v)
	}
}

func BenchmarkWWMA_DivIntEven(b *testing.B) {
	v := decimal.Parse("123.456789")
	for b.Loop() {
		_ = v.DivIntEven(14)
	}
}

func BenchmarkWWMA_DivFromInt(b *testing.B) {
	v := decimal.Parse("123.456789")
	d := decimal.FromInt(14)
	for b.Loop() {
		_ = v.Div(d)
	}
}
