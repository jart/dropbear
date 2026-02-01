package decimal

import (
	"testing"
)

func TestMul(t *testing.T) {
	tests := []struct {
		name string
		d, o Decimal
		want Decimal
	}{
		// Zero multiplications
		{"0 * 0 = 0", Zero, Zero, Zero},
		{"1 * 0 = 0", One, Zero, Zero},
		{"0 * 1 = 0", Zero, One, Zero},
		{"-1 * 0 = 0", NegOne, Zero, Zero},
		{"Max * 0 = 0", Max, Zero, Zero},
		{"Min * 0 = 0", Min, Zero, Zero},

		// Identity (anything * 1 = anything)
		{"1 * 1 = 1", One, One, One},
		{"-1 * 1 = -1", NegOne, One, NegOne},
		{"2 * 1 = 2", Two, One, Two},
		{"Max * 1 = Max", Max, One, Max},
		{"Min * 1 = Min", Min, One, Min},

		// Negation (anything * -1)
		{"1 * -1 = -1", One, NegOne, NegOne},
		{"-1 * -1 = 1", NegOne, NegOne, One},
		{"2 * -1 = -2", Two, NegOne, -Two},
		{"Max * -1 = -Max", Max, NegOne, -Max},
		// Min * -1 overflows (tested in panic tests)

		// Small positive * positive
		{"2 * 2 = 4", Two, Two, Parse("4")},
		{"2 * 3 = 6", Two, Parse("3"), Parse("6")},
		{"1.5 * 2 = 3", Parse("1.5"), Two, Parse("3")},
		{"1.5 * 1.5 = 2.25", Parse("1.5"), Parse("1.5"), Parse("2.25")},
		{"0.5 * 0.5 = 0.25", Half, Half, Parse("0.25")},
		{"0.1 * 0.1 = 0.01", Tenth, Tenth, Parse("0.01")},
		{"10 * 10 = 100", Parse("10"), Parse("10"), Parse("100")},
		{"100 * 100 = 10000", Parse("100"), Parse("100"), Parse("10000")},
		{"1000 * 1000 = 1000000", Parse("1000"), Parse("1000"), Parse("1000000")},

		// Small positive * negative
		{"2 * -2 = -4", Two, -Two, Parse("-4")},
		{"1.5 * -2 = -3", Parse("1.5"), -Two, Parse("-3")},
		{"0.5 * -0.5 = -0.25", Half, -Half, Parse("-0.25")},

		// Small negative * positive
		{"-2 * 2 = -4", -Two, Two, Parse("-4")},
		{"-1.5 * 2 = -3", Parse("-1.5"), Two, Parse("-3")},

		// Small negative * negative
		{"-2 * -2 = 4", -Two, -Two, Parse("4")},
		{"-1.5 * -1.5 = 2.25", Parse("-1.5"), Parse("-1.5"), Parse("2.25")},
		{"-0.5 * -0.5 = 0.25", -Half, -Half, Parse("0.25")},

		// Fractional precision
		{"0.000000001 * 1 = 0.000000001", Epsilon, One, Epsilon},
		{"0.000000001 * 2 = 0.000000002", Epsilon, Two, Decimal(2)},
		{"0.000000001 * 1000000000 = 1", Epsilon, Parse("1000000000"), One},
		{"0.01 * 100 = 1", Cent, Parse("100"), One},
		{"0.001 * 1000 = 1", Parse("0.001"), Parse("1000"), One},

		// Commutative property
		{"3 * 7 = 21", Parse("3"), Parse("7"), Parse("21")},
		{"7 * 3 = 21", Parse("7"), Parse("3"), Parse("21")},
		{"1.23 * 4.56", Parse("1.23"), Parse("4.56"), Parse("5.6088")},
		{"4.56 * 1.23", Parse("4.56"), Parse("1.23"), Parse("5.6088")},

		// Associative-like checks
		{"(2*3)*4 = 24", Parse("6"), Parse("4"), Parse("24")},
		{"2*(3*4) = 24", Two, Parse("12"), Parse("24")},

		// Powers and squares
		{"10 * 10 = 100", Parse("10"), Parse("10"), Parse("100")},
		{"100 * 100 = 10000", Parse("100"), Parse("100"), Parse("10000")},
		{"0.1 * 10 = 1", Parse("0.1"), Parse("10"), One},
		{"0.01 * 100 = 1", Parse("0.01"), Parse("100"), One},

		// Financial calculations
		{"100 * 1.05 = 105", Parse("100"), Parse("1.05"), Parse("105")},
		{"1000 * 0.0725 = 72.5", Parse("1000"), Parse("0.0725"), Parse("72.5")},
		{"99.99 * 1.0825 = 108.239175", Parse("99.99"), Parse("1.0825"), Parse("108.239175")},

		// Near-boundary products
		{"Max/2 * 2 = Max-ish", Max / 2, Two, (Max / 2).Mul(Two)},
		{"Min/2 * 2 = Min", Min / 2, Two, Min},

		// Half multiplications
		{"1 * 0.5 = 0.5", One, Half, Half},
		{"2 * 0.5 = 1", Two, Half, One},
		{"3 * 0.5 = 1.5", Parse("3"), Half, Parse("1.5")},
		{"0.5 * 0.5 = 0.25", Half, Half, Parse("0.25")},

		// Tenth multiplications
		{"1 * 0.1 = 0.1", One, Tenth, Tenth},
		{"10 * 0.1 = 1", Parse("10"), Tenth, One},
		{"0.1 * 0.1 = 0.01", Tenth, Tenth, Parse("0.01")},

		// Large safe products
		{"1000000 * 1000", Parse("1000000"), Parse("1000"), Parse("1000000000")},
		{"50000000 * 100", Parse("50000000"), Parse("100"), Parse("5000000000")},

		// Epsilon-scale products
		{"0.000000001 * 0.000000001 = 0", Epsilon, Epsilon, Zero}, // underflow to zero
		{"0.0001 * 0.0001 = 0.00000001", Parse("0.0001"), Parse("0.0001"), Parse("0.00000001")},
		{"0.001 * 0.00001 = 0.00000001", Parse("0.001"), Parse("0.00001"), Parse("0.00000001")},

		// bankers' rounding demonstration
		{"0.5 * 0.000000001 = 0 (0.5 -> 0)", Half, Epsilon, Zero},
		{"1.5 * 0.000000001 = 0.000000002 (1.5 -> 2)", Parse("1.5"), Epsilon, Decimal(2)},
		{"2.5 * 0.000000001 = 0.000000002 (2.5 -> 2)", Parse("2.5"), Epsilon, Decimal(2)},
		{"3.5 * 0.000000001 = 0.000000004 (3.5 -> 4)", Parse("3.5"), Epsilon, Decimal(4)},
		{"4.5 * 0.000000001 = 0.000000004 (4.5 -> 4)", Parse("4.5"), Epsilon, Decimal(4)},
		{"5.5 * 0.000000001 = 0.000000006 (5.5 -> 6)", Parse("5.5"), Epsilon, Decimal(6)},
		{"6.5 * 0.000000001 = 0.000000006 (6.5 -> 6)", Parse("6.5"), Epsilon, Decimal(6)},
		{"7.5 * 0.000000001 = 0.000000008 (7.5 -> 8)", Parse("7.5"), Epsilon, Decimal(8)},
		{"8.5 * 0.000000001 = 0.000000008 (8.5 -> 8)", Parse("8.5"), Epsilon, Decimal(8)},
		{"9.5 * 0.000000001 = 0.000000010 (9.5 -> 10)", Parse("9.5"), Epsilon, Decimal(10)},
		{"0.000000003 * 0.5 = 0.000000002 (1.5 -> 2)", Decimal(3), Half, Decimal(2)},
		{"0.000000005 * 0.5 = 0.000000002 (2.5 -> 2)", Decimal(5), Half, Decimal(2)},
		{"0.000000007 * 0.5 = 0.000000004 (3.5 -> 4)", Decimal(7), Half, Decimal(4)},
		{"0.000000009 * 0.5 = 0.000000004 (4.5 -> 4)", Decimal(9), Half, Decimal(4)},
		{"0.000000011 * 0.5 = 0.000000006 (5.5 -> 6)", Decimal(11), Half, Decimal(6)},
		{"-0.000000003 * 0.5 = -0.000000002 (-1.5 -> -2)", Decimal(-3), Half, Decimal(-2)},
		{"-0.000000005 * 0.5 = -0.000000002 (-2.5 -> -2)", Decimal(-5), Half, Decimal(-2)},
		{"-0.000000007 * 0.5 = -0.000000004 (-3.5 -> -4)", Decimal(-7), Half, Decimal(-4)},
		{"-0.000000009 * 0.5 = -0.000000004 (-4.5 -> -4)", Decimal(-9), Half, Decimal(-4)},
		{"0.000000003 * -0.5 = -0.000000002", Decimal(3), -Half, Decimal(-2)},
		{"0.000000005 * -0.5 = -0.000000002", Decimal(5), -Half, Decimal(-2)},
		{"-0.000000003 * -0.5 = 0.000000002", Decimal(-3), -Half, Decimal(2)},
		{"-0.000000005 * -0.5 = 0.000000002", Decimal(-5), -Half, Decimal(2)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.d.Mul(tt.o)
			if got != tt.want {
				t.Errorf("Mul() = %v (%d), want %v (%d)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestMulPanic(t *testing.T) {
	tests := []struct {
		name string
		d, o Decimal
	}{
		// Min * -1 = +2^63 which overflows
		{"Min * -1", Min, NegOne},

		// Max overflow cases
		{"Max * 2", Max, Two},
		{"Max * -2", Max, -Two},
		{"Max * Max", Max, Max},
		{"Max * 1.00000001", Max, One.Add(Epsilon)},

		// Min overflow cases
		{"Min * 2", Min, Two},
		{"Min * -2", Min, -Two},
		{"Min * Min", Min, Min},

		// Large products that overflow
		{"5000000000 * 2", Parse("5000000000"), Two},
		{"1000000000 * 10", Parse("1000000000"), Parse("10")},
		{"1000000000 * 100", Parse("1000000000"), Parse("100")},

		// Near-max products
		{"9000000000 * 1.1", Parse("9000000000"), Parse("1.1")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("Mul(%v, %v) should have panicked", tt.d, tt.o)
				}
			}()
			tt.d.Mul(tt.o)
		})
	}
}

func BenchmarkMul(b *testing.B) {
	d := Parse("123.456")
	o := Parse("789.012")
	for b.Loop() {
		_ = d.Mul(o)
	}
}
