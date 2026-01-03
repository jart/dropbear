package decimal

import (
	"testing"
)

func TestDiv(t *testing.T) {
	tests := []struct {
		name string
		d, o Decimal
		want Decimal
	}{
		// Identity (anything / 1 = anything)
		{"0 / 1 = 0", Zero, One, Zero},
		{"1 / 1 = 1", One, One, One},
		{"-1 / 1 = -1", NegOne, One, NegOne},
		{"2 / 1 = 2", Two, One, Two},
		{"Max / 1 = Max", Max, One, Max},
		{"Min / 1 = Min", Min, One, Min},

		// Negation (anything / -1)
		{"1 / -1 = -1", One, NegOne, NegOne},
		{"-1 / -1 = 1", NegOne, NegOne, One},
		{"2 / -1 = -2", Two, NegOne, -Two},
		{"Max / -1 = -Max", Max, NegOne, -Max},
		// Min / -1 overflows (tested in panic tests)

		// Zero numerator
		{"0 / 1 = 0", Zero, One, Zero},
		{"0 / 2 = 0", Zero, Two, Zero},
		{"0 / -1 = 0", Zero, NegOne, Zero},
		{"0 / Max = 0", Zero, Max, Zero},
		{"0 / Min = 0", Zero, Min, Zero},

		// Self division
		{"1 / 1 = 1", One, One, One},
		{"2 / 2 = 1", Two, Two, One},
		{"-1 / -1 = 1", NegOne, NegOne, One},
		{"Max / Max = 1", Max, Max, One},
		{"Min / Min = 1", Min, Min, One},

		// Simple exact divisions
		{"2 / 1 = 2", Two, One, Two},
		{"4 / 2 = 2", Parse("4"), Two, Two},
		{"6 / 2 = 3", Parse("6"), Two, Parse("3")},
		{"6 / 3 = 2", Parse("6"), Parse("3"), Two},
		{"10 / 2 = 5", Parse("10"), Two, Parse("5")},
		{"100 / 10 = 10", Parse("100"), Parse("10"), Parse("10")},
		{"1000 / 100 = 10", Parse("1000"), Parse("100"), Parse("10")},

		// Fractional results
		{"1 / 2 = 0.5", One, Two, Half},
		{"1 / 4 = 0.25", One, Parse("4"), Parse("0.25")},
		{"1 / 5 = 0.2", One, Parse("5"), Parse("0.2")},
		{"1 / 8 = 0.125", One, Parse("8"), Parse("0.125")},
		{"1 / 10 = 0.1", One, Parse("10"), Tenth},
		{"1 / 100 = 0.01", One, Parse("100"), Cent},
		{"3 / 2 = 1.5", Parse("3"), Two, Parse("1.5")},
		{"5 / 2 = 2.5", Parse("5"), Two, Parse("2.5")},
		{"7 / 2 = 3.5", Parse("7"), Two, Parse("3.5")},

		// Negative divisions
		{"6 / -2 = -3", Parse("6"), -Two, Parse("-3")},
		{"-6 / 2 = -3", Parse("-6"), Two, Parse("-3")},
		{"-6 / -2 = 3", Parse("-6"), -Two, Parse("3")},
		{"1 / -2 = -0.5", One, -Two, -Half},
		{"-1 / 2 = -0.5", NegOne, Two, -Half},
		{"-1 / -2 = 0.5", NegOne, -Two, Half},

		// Repeating decimals (rounded)
		{"1 / 3 = 0.33333333", One, Parse("3"), Parse("0.33333333")},
		{"2 / 3 = 0.66666667", Two, Parse("3"), Parse("0.66666667")},
		{"1 / 6 = 0.16666667", One, Parse("6"), Parse("0.16666667")},
		{"1 / 7 = 0.14285714", One, Parse("7"), Parse("0.14285714")},
		{"1 / 9 = 0.11111111", One, Parse("9"), Parse("0.11111111")},
		{"1 / 11 = 0.09090909", One, Parse("11"), Parse("0.09090909")},

		// Powers of 10
		{"10 / 1 = 10", Parse("10"), One, Parse("10")},
		{"1 / 10 = 0.1", One, Parse("10"), Parse("0.1")},
		{"100 / 1 = 100", Parse("100"), One, Parse("100")},
		{"1 / 100 = 0.01", One, Parse("100"), Parse("0.01")},
		{"1 / 1000 = 0.001", One, Parse("1000"), Parse("0.001")},
		{"1 / 10000000 = 0.0000001", One, Parse("10000000"), Parse("0.0000001")},
		{"1 / 100000000 = 0.00000001", One, Parse("100000000"), Epsilon},

		// Near-epsilon results
		{"0.00000001 / 1 = 0.00000001", Epsilon, One, Epsilon},
		{"0.00000002 / 2 = 0.00000001", Decimal(2), Two, Epsilon},
		{"0.00000010 / 10 = 0.00000001", Decimal(10), Parse("10"), Epsilon},

		// Financial calculations
		{"100 / 1.05 = 95.23809524", Parse("100"), Parse("1.05"), Parse("95.23809524")},
		{"72.5 / 1000 = 0.0725", Parse("72.5"), Parse("1000"), Parse("0.0725")},
		{"1 / 365 = 0.00273973", One, Parse("365"), Parse("0.00273973")},

		// Large dividends
		{"1000000 / 1000 = 1000", Parse("1000000"), Parse("1000"), Parse("1000")},
		{"1000000000 / 1000 = 1000000", Parse("1000000000"), Parse("1000"), Parse("1000000")},

		// Small divisors (large results)
		{"1 / 0.1 = 10", One, Parse("0.1"), Parse("10")},
		{"1 / 0.01 = 100", One, Parse("0.01"), Parse("100")},
		{"1 / 0.001 = 1000", One, Parse("0.001"), Parse("1000")},
		{"1 / 0.00000001 = 100000000", One, Epsilon, Parse("100000000")},
		{"2 / 0.5 = 4", Two, Half, Parse("4")},
		{"10 / 0.5 = 20", Parse("10"), Half, Parse("20")},

		// Commutative property verification (a/b != b/a usually)
		{"6 / 2 = 3", Parse("6"), Two, Parse("3")},
		{"2 / 6 = 0.33333333", Two, Parse("6"), Parse("0.33333333")},

		// Half of various values
		{"1 / 2 = 0.5", One, Two, Half},
		{"2 / 2 = 1", Two, Two, One},
		{"3 / 2 = 1.5", Parse("3"), Two, Parse("1.5")},
		{"4 / 2 = 2", Parse("4"), Two, Two},
		{"5 / 2 = 2.5", Parse("5"), Two, Parse("2.5")},
		{"99 / 2 = 49.5", Parse("99"), Two, Parse("49.5")},
		{"100 / 2 = 50", Parse("100"), Two, Parse("50")},

		// Quarter divisions
		{"1 / 4 = 0.25", One, Parse("4"), Parse("0.25")},
		{"2 / 4 = 0.5", Two, Parse("4"), Half},
		{"3 / 4 = 0.75", Parse("3"), Parse("4"), Parse("0.75")},

		// Percentage calculations
		{"25 / 100 = 0.25", Parse("25"), Parse("100"), Parse("0.25")},
		{"50 / 100 = 0.5", Parse("50"), Parse("100"), Half},
		{"75 / 100 = 0.75", Parse("75"), Parse("100"), Parse("0.75")},
		{"1 / 100 = 0.01", One, Parse("100"), Cent},

		// BPS calculations
		{"1 / 10000 = 0.0001", One, Parse("10000"), Parse("0.0001")},
		{"25 / 10000 = 0.0025", Parse("25"), Parse("10000"), Parse("0.0025")},
		{"100 / 10000 = 0.01", Parse("100"), Parse("10000"), Cent},

		// bankers' rounding demonstration
		{"0.00000001 / 2 = 0 (0.5 -> 0, even)", Epsilon, Two, Zero},
		{"0.00000003 / 2 = 0.00000002 (1.5 -> 2, odd rounds up)", Decimal(3), Two, Decimal(2)},
		{"0.00000005 / 2 = 0.00000002 (2.5 -> 2, even stays)", Decimal(5), Two, Decimal(2)},
		{"0.00000007 / 2 = 0.00000004 (3.5 -> 4, odd rounds up)", Decimal(7), Two, Decimal(4)},
		{"0.00000009 / 2 = 0.00000004 (4.5 -> 4, even stays)", Decimal(9), Two, Decimal(4)},
		{"0.00000011 / 2 = 0.00000006 (5.5 -> 6, odd rounds up)", Decimal(11), Two, Decimal(6)},
		{"0.00000013 / 2 = 0.00000006 (6.5 -> 6, even stays)", Decimal(13), Two, Decimal(6)},
		{"0.00000015 / 2 = 0.00000008 (7.5 -> 8, odd rounds up)", Decimal(15), Two, Decimal(8)},
		{"0.00000017 / 2 = 0.00000008 (8.5 -> 8, even stays)", Decimal(17), Two, Decimal(8)},
		{"0.00000019 / 2 = 0.00000010 (9.5 -> 10, odd rounds up)", Decimal(19), Two, Decimal(10)},
		{"-0.00000003 / 2 = -0.00000002 (-1.5 -> -2)", Decimal(-3), Two, Decimal(-2)},
		{"-0.00000005 / 2 = -0.00000002 (-2.5 -> -2)", Decimal(-5), Two, Decimal(-2)},
		{"-0.00000007 / 2 = -0.00000004 (-3.5 -> -4)", Decimal(-7), Two, Decimal(-4)},
		{"-0.00000009 / 2 = -0.00000004 (-4.5 -> -4)", Decimal(-9), Two, Decimal(-4)},
		{"0.00000003 / -2 = -0.00000002", Decimal(3), -Two, Decimal(-2)},
		{"0.00000005 / -2 = -0.00000002", Decimal(5), -Two, Decimal(-2)},
		{"-0.00000003 / -2 = 0.00000002", Decimal(-3), -Two, Decimal(2)},
		{"-0.00000005 / -2 = 0.00000002", Decimal(-5), -Two, Decimal(2)},

		// exact results (no rounding needed)
		{"1.5 / 1 = 1.5 (exact)", Parse("1.5"), One, Parse("1.5")},
		{"2.5 / 1 = 2.5 (exact)", Parse("2.5"), One, Parse("2.5")},
		{"0.15 / 10 = 0.015 (exact)", Parse("0.15"), Parse("10"), Parse("0.015")},

		// division by odd numbers (no exact half possible)
		{"1 / 3 = 0.33333333", One, Parse("3"), Parse("0.33333333")},
		{"2 / 3 = 0.66666667", Two, Parse("3"), Parse("0.66666667")},
		{"10 / 3 = 3.33333333", Parse("10"), Parse("3"), Parse("3.33333333")},
		{"20 / 3 = 6.66666667", Parse("20"), Parse("3"), Parse("6.66666667")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.d.Div(tt.o)
			if got != tt.want {
				t.Errorf("Div() = %v (%d), want %v (%d)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestDivPanic(t *testing.T) {
	tests := []struct {
		name string
		d, o Decimal
	}{
		// Division by zero
		{"1 / 0", One, Zero},
		{"-1 / 0", NegOne, Zero},
		{"Max / 0", Max, Zero},
		{"Min / 0", Min, Zero},
		{"0 / 0", Zero, Zero},

		// Min / -1 = +2^63 which overflows
		{"Min / -1", Min, NegOne},

		// Large quotients that overflow
		{"Max / 0.5", Max, Half},
		{"Max / 0.1", Max, Tenth},
		{"Max / 0.00000001", Max, Epsilon},
		{"1000000000 / 0.00000001", Parse("1000000000"), Epsilon},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("Div(%v, %v) should have panicked", tt.d, tt.o)
				}
			}()
			tt.d.Div(tt.o)
		})
	}
}

func BenchmarkDivBoundary(b *testing.B) {
	d := Parse("123.456")
	o := Parse("78.9")
	for i := 0; i < b.N; i++ {
		_ = d.Div(o)
	}
}
