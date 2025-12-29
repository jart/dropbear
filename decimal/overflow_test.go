package decimal

import (
	"math"
	"testing"
)

func TestAddOverflow(t *testing.T) {
	// positive + positive overflow
	assertPanics(t, "MaxInt64 + 1", func() {
		Decimal(math.MaxInt64).Add(1)
	})
	// negative + negative overflow
	assertPanics(t, "MinInt64 + -1", func() {
		Decimal(math.MinInt64).Add(-1)
	})
	// large positive + large positive
	assertPanics(t, "Max + Max", func() {
		Max.Add(Max)
	})
}

func TestAddNoOverflow(t *testing.T) {
	// these should not panic
	_ = Max.Add(Zero)
	_ = Min.Add(Zero)
	_ = Max.Add(Min) // cancels out
	_ = One.Add(One)
	_ = Parse("1000000000").Add(Parse("1000000000"))
}

func TestSubOverflow(t *testing.T) {
	// positive - negative overflow
	assertPanics(t, "MaxInt64 - -1", func() {
		Decimal(math.MaxInt64).Sub(-1)
	})
	// negative - positive overflow
	assertPanics(t, "MinInt64 - 1", func() {
		Decimal(math.MinInt64).Sub(1)
	})
	// Max - Min
	assertPanics(t, "Max - Min", func() {
		Max.Sub(Min)
	})
}

func TestSubNoOverflow(t *testing.T) {
	// these should not panic
	_ = Max.Sub(Zero)
	_ = Min.Sub(Zero)
	_ = Max.Sub(Max)
	_ = One.Sub(One)
}

func TestMulOverflow(t *testing.T) {
	// large * large
	assertPanics(t, "Max * 2", func() {
		Max.Mul(Two)
	})
	assertPanics(t, "1000000 * 1000000000", func() {
		Parse("1000000").Mul(Parse("1000000000"))
	})
	// negative overflow
	assertPanics(t, "Max * -2", func() {
		Max.Mul(Two.Neg())
	})
}

func TestMulNoOverflow(t *testing.T) {
	// these should not panic
	_ = One.Mul(One)
	_ = Parse("10000").Mul(Parse("10000")) // 100 million, fits easily
	_ = Parse("0.5").Mul(Max)              // half of max
	_ = Zero.Mul(Max)
	_ = Max.Mul(Zero)
	_ = Parse("-1").Mul(Parse("1"))
	_ = Parse("94000").Mul(Parse("94000")) // ~8.8 billion, under Max
	_ = Max.Mul(One)                       // Max * 1 = Max
}

func assertPanics(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("%s: expected panic, got none", name)
		}
	}()
	f()
}

func TestMulIntOverflow(t *testing.T) {
	assertPanics(t, "Max * 2", func() {
		Max.MulInt(2)
	})
	assertPanics(t, "large * large", func() {
		Parse("1000000000").MulInt(100)
	})
}

func TestMulIntNoOverflow(t *testing.T) {
	_ = One.MulInt(1000000000)
	_ = Max.MulInt(1)
	_ = Max.MulInt(0)
	_ = Zero.MulInt(1000000000)
	_ = Parse("-1").MulInt(1)
}

func TestDivIntOverflow(t *testing.T) {
	assertPanics(t, "MinInt64 / -1", func() {
		Decimal(math.MinInt64).DivInt(-1)
	})
}

func TestDivIntNoOverflow(t *testing.T) {
	_ = Max.DivInt(1)
	_ = Max.DivInt(-1)
	_ = Min.DivInt(1)
	_ = One.DivInt(1000000000)
}

func TestExpOverflow(t *testing.T) {
	assertPanics(t, "Exp(26)", func() {
		Parse("26").Exp() // e^26 ≈ 1.9e11 > Max/Scale (9.2e10)
	})
	assertPanics(t, "Exp(100)", func() {
		Parse("100").Exp()
	})
	assertPanics(t, "Exp(25.24762514)", func() {
		Parse("25.24762514").Exp() // Just above max safe exponent
	})
}

func TestExpNoOverflow(t *testing.T) {
	_ = Parse("22").Exp()  // e^22 ≈ 3.6e9, fits
	_ = Parse("0").Exp()   // e^0 = 1
	_ = Parse("-10").Exp() // e^-10 ≈ 0.0000454
}

func BenchmarkMulWithOverflowCheck(b *testing.B) {
	a := Parse("12345.67")
	c := Parse("89.012")
	for i := 0; b.Loop(); i++ {
		_ = a.Mul(c)
	}
}
