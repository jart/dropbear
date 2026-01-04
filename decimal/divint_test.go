package decimal

import (
	"math"
	"testing"
)

func TestDivInt(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		n    int
		want Decimal
	}{
		// identity
		{"0 / 1 = 0", Zero, 1, Zero},
		{"1 / 1 = 1", One, 1, One},
		{"-1 / 1 = -1", NegOne, 1, NegOne},
		{"2 / 1 = 2", Two, 1, Two},
		{"Max / 1 = Max", Max, 1, Max},
		{"Min / 1 = Min", Min, 1, Min},

		// negation
		{"1 / -1 = -1", One, -1, NegOne},
		{"-1 / -1 = 1", NegOne, -1, One},
		{"2 / -1 = -2", Two, -1, -Two},
		{"Max / -1 = -Max", Max, -1, -Max},

		// zero numerator
		{"0 / 2 = 0", Zero, 2, Zero},
		{"0 / -1 = 0", Zero, -1, Zero},
		{"0 / 100 = 0", Zero, 100, Zero},

		// exact divisions
		{"4 / 2 = 2", Parse("4"), 2, Two},
		{"6 / 2 = 3", Parse("6"), 2, Parse("3")},
		{"6 / 3 = 2", Parse("6"), 3, Two},
		{"10 / 2 = 5", Parse("10"), 2, Parse("5")},
		{"100 / 10 = 10", Parse("100"), 10, Parse("10")},
		{"1000 / 100 = 10", Parse("1000"), 100, Parse("10")},

		// fractional results
		{"0.00000010 / 2 = 0.00000005", Decimal(10), 2, Decimal(5)},
		{"1 / 2 = 0.5", One, 2, Half},
		{"1 / 4 = 0.25", One, 4, Parse("0.25")},
		{"1 / 5 = 0.2", One, 5, Parse("0.2")},
		{"1 / 10 = 0.1", One, 10, Tenth},
		{"1 / 100 = 0.01", One, 100, Cent},

		// negative divisions
		{"6 / -2 = -3", Parse("6"), -2, Parse("-3")},
		{"-6 / 2 = -3", Parse("-6"), 2, Parse("-3")},
		{"-6 / -2 = 3", Parse("-6"), -2, Parse("3")},
		{"1 / -2 = -0.5", One, -2, -Half},
		{"-1 / 2 = -0.5", NegOne, 2, -Half},
		{"-1 / -2 = 0.5", NegOne, -2, Half},

		// repeating decimals
		{"1 / 3 = 0.33333333", One, 3, Parse("0.33333333")},
		{"2 / 3 = 0.66666667", Two, 3, Parse("0.66666667")},
		{"1 / 6 = 0.16666667", One, 6, Parse("0.16666667")},
		{"1 / 7 = 0.14285714", One, 7, Parse("0.14285714")},
		{"1 / 9 = 0.11111111", One, 9, Parse("0.11111111")},

		// powers of 2
		{"1 / 8 = 0.125", One, 8, Parse("0.125")},
		{"1 / 16 = 0.0625", One, 16, Parse("0.0625")},
		{"1 / 32 = 0.03125", One, 32, Parse("0.03125")},
		{"1 / 64 = 0.015625", One, 64, Parse("0.015625")},

		// powers of 10
		{"1 / 1000 = 0.001", One, 1000, Parse("0.001")},
		{"1 / 10000 = 0.0001", One, 10000, Parse("0.0001")},
		{"1 / 100000000 = 0.00000001", One, 100000000, Epsilon},

		// large values
		{"1000000 / 1000 = 1000", Parse("1000000"), 1000, Parse("1000")},
		{"1000000000 / 1000 = 1000000", Parse("1000000000"), 1000, Parse("1000000")},
		{"Max / 2 (rounds up)", Max, 2, Max/2 + 1},
		{"Max / MaxInt32", Max, math.MaxInt32, Max.DivInt(math.MaxInt32)},
		{"Min / 2", Min, 2, Min / 2},

		// bankers' rounding demonstration
		{"1 / 2 = 0 (0.5 -> 0)", Epsilon, 2, Zero},
		{"3 / 2 = 2 (1.5 -> 2)", Decimal(3), 2, Decimal(2)},
		{"5 / 2 = 2 (2.5 -> 2)", Decimal(5), 2, Decimal(2)},
		{"7 / 2 = 4 (3.5 -> 4)", Decimal(7), 2, Decimal(4)},
		{"9 / 2 = 4 (4.5 -> 4)", Decimal(9), 2, Decimal(4)},
		{"11 / 2 = 6 (5.5 -> 6)", Decimal(11), 2, Decimal(6)},
		{"13 / 2 = 6 (6.5 -> 6)", Decimal(13), 2, Decimal(6)},
		{"15 / 2 = 8 (7.5 -> 8)", Decimal(15), 2, Decimal(8)},
		{"17 / 2 = 8 (8.5 -> 8)", Decimal(17), 2, Decimal(8)},
		{"19 / 2 = 10 (9.5 -> 10)", Decimal(19), 2, Decimal(10)},
		{"-3 / 2 = -2", Decimal(-3), 2, Decimal(-2)},
		{"-5 / 2 = -2", Decimal(-5), 2, Decimal(-2)},
		{"-7 / 2 = -4", Decimal(-7), 2, Decimal(-4)},
		{"-9 / 2 = -4", Decimal(-9), 2, Decimal(-4)},
		{"3 / -2 = -2", Decimal(3), -2, Decimal(-2)},
		{"5 / -2 = -2", Decimal(5), -2, Decimal(-2)},
		{"-3 / -2 = 2", Decimal(-3), -2, Decimal(2)},
		{"-5 / -2 = 2", Decimal(-5), -2, Decimal(2)},
		{"5 / 4 = 1 (1.25 -> 1)", Decimal(5), 4, Decimal(1)},
		{"6 / 4 = 2 (1.5 -> 2)", Decimal(6), 4, Decimal(2)},
		{"10 / 4 = 2 (2.5 -> 2)", Decimal(10), 4, Decimal(2)},
		{"14 / 4 = 4 (3.5 -> 4)", Decimal(14), 4, Decimal(4)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.d.DivInt(tt.n)
			if got != tt.want {
				t.Errorf("DivInt() = %v (%d), want %v (%d)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestDivIntPanic(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		n    int
	}{
		{"1 / 0", One, 0},
		{"-1 / 0", NegOne, 0},
		{"Max / 0", Max, 0},
		{"Min / 0", Min, 0},
		{"0 / 0", Zero, 0},
		{"Min / -1", Min, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("DivInt(%v, %v) should have panicked", tt.d, tt.n)
				}
			}()
			tt.d.DivInt(tt.n)
		})
	}
}

func BenchmarkDivInt(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		a := randomNumbers[(i+0)&31]
		_ = a.DivInt(2)
	}
}
