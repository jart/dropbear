package decimal

import (
	"math"
	"testing"
)

func TestMulInt(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		n    int
		want Decimal
	}{
		// Zero multiplications (anything * 0 = 0)
		{"0 * 0 = 0", 0, 0, 0},
		{"1 * 0 = 0", 1, 0, 0},
		{"-1 * 0 = 0", -1, 0, 0},
		{"100 * 0 = 0", 100, 0, 0},
		{"-100 * 0 = 0", -100, 0, 0},
		{"Max * 0 = 0", Max, 0, 0},
		{"Min * 0 = 0", Min, 0, 0},
		{"0 * MaxInt = 0", 0, math.MaxInt, 0},
		{"0 * MinInt = 0", 0, math.MinInt, 0},

		// Identity (anything * 1 = anything)
		{"0 * 1 = 0", 0, 1, 0},
		{"1 * 1 = 1", 1, 1, 1},
		{"-1 * 1 = -1", -1, 1, -1},
		{"100 * 1 = 100", 100, 1, 100},
		{"-100 * 1 = -100", -100, 1, -100},
		{"Max * 1 = Max", Max, 1, Max},
		{"Min * 1 = Min", Min, 1, Min},

		// Negation (anything * -1)
		{"0 * -1 = 0", 0, -1, 0},
		{"1 * -1 = -1", 1, -1, -1},
		{"-1 * -1 = 1", -1, -1, 1},
		{"100 * -1 = -100", 100, -1, -100},
		{"-100 * -1 = 100", -100, -1, 100},
		{"Max * -1 = -Max", Max, -1, -Max},
		// Min * -1 overflows (tested in panic tests)

		// Small positive * positive
		{"2 * 2 = 4", 2, 2, 4},
		{"3 * 3 = 9", 3, 3, 9},
		{"7 * 11 = 77", 7, 11, 77},
		{"13 * 17 = 221", 13, 17, 221},
		{"100 * 100 = 10000", 100, 100, 10000},
		{"1000 * 1000 = 1000000", 1000, 1000, 1000000},
		{"12345 * 6789 = 83810205", 12345, 6789, 83810205},

		// Small positive * negative
		{"2 * -2 = -4", 2, -2, -4},
		{"3 * -3 = -9", 3, -3, -9},
		{"7 * -11 = -77", 7, -11, -77},
		{"100 * -100 = -10000", 100, -100, -10000},
		{"12345 * -6789 = -83810205", 12345, -6789, -83810205},

		// Small negative * positive
		{"-2 * 2 = -4", -2, 2, -4},
		{"-3 * 3 = -9", -3, 3, -9},
		{"-7 * 11 = -77", -7, 11, -77},
		{"-100 * 100 = -10000", -100, 100, -10000},
		{"-12345 * 6789 = -83810205", -12345, 6789, -83810205},

		// Small negative * negative
		{"-2 * -2 = 4", -2, -2, 4},
		{"-3 * -3 = 9", -3, -3, 9},
		{"-7 * -11 = 77", -7, -11, 77},
		{"-100 * -100 = 10000", -100, -100, 10000},
		{"-12345 * -6789 = 83810205", -12345, -6789, 83810205},

		// Powers of 2
		{"1 * 2 = 2", 1, 2, 2},
		{"1 * 4 = 4", 1, 4, 4},
		{"1 * 8 = 8", 1, 8, 8},
		{"1 * 16 = 16", 1, 16, 16},
		{"1 * 1024 = 1024", 1, 1024, 1024},
		{"1 * 1048576 = 1048576", 1, 1048576, 1048576},
		{"2 * 2 = 4", 2, 2, 4},
		{"4 * 4 = 16", 4, 4, 16},
		{"256 * 256 = 65536", 256, 256, 65536},
		{"65536 * 65536 = 4294967296", 65536, 65536, 4294967296},

		// Int32 boundary products
		{"Max32 * 1", Decimal(math.MaxInt32), 1, Decimal(math.MaxInt32)},
		{"Max32 * -1", Decimal(math.MaxInt32), -1, Decimal(-math.MaxInt32)},
		{"Min32 * 1", Decimal(math.MinInt32), 1, Decimal(math.MinInt32)},
		{"Min32 * -1", Decimal(math.MinInt32), -1, Decimal(-math.MinInt32)},
		{"Max32 * Max32", Decimal(math.MaxInt32), math.MaxInt32, Decimal(int64(math.MaxInt32) * int64(math.MaxInt32))},
		{"Min32 * Min32", Decimal(math.MinInt32), math.MinInt32, Decimal(int64(math.MinInt32) * int64(math.MinInt32))},

		// Large products that fit (just under sqrt(MaxInt64))
		{"3037000499 * 3037000499", 3037000499, 3037000499, 3037000499 * 3037000499},
		{"-3037000499 * 3037000499", -3037000499, 3037000499, -3037000499 * 3037000499},
		{"3037000499 * -3037000499", 3037000499, -3037000499, -3037000499 * 3037000499},
		{"-3037000499 * -3037000499", -3037000499, -3037000499, 3037000499 * 3037000499},

		// Products near MaxInt64
		{"Max/2 * 2", Max / 2, 2, (Max / 2) * 2},
		{"Max/3 * 3", Max / 3, 3, (Max / 3) * 3},
		{"Max/7 * 7", Max / 7, 7, (Max / 7) * 7},
		{"Max/10 * 10", Max / 10, 10, (Max / 10) * 10},
		{"Max/100 * 100", Max / 100, 100, (Max / 100) * 100},
		{"Max/1000 * 1000", Max / 1000, 1000, (Max / 1000) * 1000},

		// Products near MinInt64
		{"Min/2 * 2 = Min", Min / 2, 2, Min},
		{"Min/3 * 3", Min / 3, 3, (Min / 3) * 3},
		{"Min/7 * 7", Min / 7, 7, (Min / 7) * 7},
		{"Min/10 * 10", Min / 10, 10, (Min / 10) * 10},
		{"Min/100 * 100", Min / 100, 100, (Min / 100) * 100},

		// Exact MinInt64 via powers of 2
		{"Min/2 * 2", Min / 2, 2, Min},
		{"Min/4 * 4", Min / 4, 4, Min},
		{"Min/8 * 8", Min / 8, 8, Min},
		{"(1<<62) * -2 = Min", 1 << 62, -2, Min},
		{"(1<<61) * -4 = Min", 1 << 61, -4, Min},
		{"(1<<60) * -8 = Min", 1 << 60, -8, Min},

		// Large multiplier with small decimal
		{"1 * MaxInt", 1, math.MaxInt, Decimal(math.MaxInt)},
		{"-1 * MaxInt", -1, math.MaxInt, Decimal(-math.MaxInt)},
		{"1 * MinInt", 1, math.MinInt, Decimal(math.MinInt)},
		{"2 * (MaxInt/2)", 2, math.MaxInt / 2, Decimal(2 * (math.MaxInt / 2))},

		// Commutative property verification
		{"7 * 13 = 91", 7, 13, 91},
		{"13 * 7 = 91", 13, 7, 91},
		{"1000000 * 1000", 1000000, 1000, 1000000000},

		// Doubling chain
		{"1 * 2", 1, 2, 2},
		{"2 * 2", 2, 2, 4},
		{"4 * 2", 4, 2, 8},
		{"(1<<30) * 2", 1 << 30, 2, 1 << 31},
		{"(1<<31) * 2", 1 << 31, 2, 1 << 32},
		{"(1<<32) * 2", 1 << 32, 2, 1 << 33},
		{"(1<<61) * 2", 1 << 61, 2, 1 << 62},

		// Near-overflow that succeeds
		{"(Max-1) * 1", Max - 1, 1, Max - 1},
		{"(Min+1) * 1", Min + 1, 1, Min + 1},
		{"(Min+1) * -1", Min + 1, -1, -(Min + 1)},

		// Human-readable decimal cases using Parse
		{"1.00 * 2", Parse("1.00"), 2, Parse("2.00")},
		{"2.50 * 4", Parse("2.50"), 4, Parse("10.00")},
		{"0.01 * 100", Parse("0.01"), 100, Parse("1.00")},
		{"0.001 * 1000", Parse("0.001"), 1000, Parse("1.00")},
		{"0.00000001 * 100000000", Parse("0.00000001"), 100000000, Parse("1.00")},
		{"99.99 * 1", Parse("99.99"), 1, Parse("99.99")},
		{"99.99 * 2", Parse("99.99"), 2, Parse("199.98")},
		{"99.99 * 10", Parse("99.99"), 10, Parse("999.90")},
		{"99.99 * 100", Parse("99.99"), 100, Parse("9999.00")},
		{"123.456 * 1", Parse("123.456"), 1, Parse("123.456")},
		{"123.456 * 2", Parse("123.456"), 2, Parse("246.912")},
		{"123.456 * 10", Parse("123.456"), 10, Parse("1234.56")},
		{"0.5 * 2", Parse("0.5"), 2, Parse("1.0")},
		{"0.25 * 4", Parse("0.25"), 4, Parse("1.0")},
		{"0.125 * 8", Parse("0.125"), 8, Parse("1.0")},
		{"0.1 * 10", Parse("0.1"), 10, Parse("1.0")},
		{"0.01 * 10", Parse("0.01"), 10, Parse("0.1")},
		{"1000000.00 * 1000", Parse("1000000.00"), 1000, Parse("1000000000.00")},
		{"0.00000001 * 1", Parse("0.00000001"), 1, Parse("0.00000001")},
		{"0.00000001 * 2", Parse("0.00000001"), 2, Parse("0.00000002")},
		{"-1.00 * 2", Parse("-1.00"), 2, Parse("-2.00")},
		{"-2.50 * 4", Parse("-2.50"), 4, Parse("-10.00")},
		{"1.00 * -2", Parse("1.00"), -2, Parse("-2.00")},
		{"-1.00 * -2", Parse("-1.00"), -2, Parse("2.00")},

		// Financial calculations
		{"100.00 * 365", Parse("100.00"), 365, Parse("36500.00")},
		{"1.05 * 100", Parse("1.05"), 100, Parse("105.00")},
		{"0.0001 * 10000", Parse("0.0001"), 10000, Parse("1.0")},

		// BPS conversions
		{"0.0001 * 10000 (1 bps)", Parse("0.0001"), 10000, Parse("1.0")},
		{"0.0025 * 10000 (25 bps)", Parse("0.0025"), 10000, Parse("25.0")},
		{"0.01 * 10000 (100 bps)", Parse("0.01"), 10000, Parse("100.0")},

		// Round numbers
		{"1000000 * 1000000", 1000000, 1000000, 1000000000000},
		{"1000000000 * 1000", 1000000000, 1000, 1000000000000},
		{"999999999 * 9", 999999999, 9, 8999999991},

		// Prime products
		{"2 * 3 = 6", 2, 3, 6},
		{"3 * 5 = 15", 3, 5, 15},
		{"5 * 7 = 35", 5, 7, 35},
		{"97 * 101 = 9797", 97, 101, 9797},
		{"997 * 1009 = 1005973", 997, 1009, 1005973},
		{"99991 * 99989 = 9998000099", 99991, 99989, 9998000099},

		// Fibonacci-ish products
		{"5 * 8 = 40", 5, 8, 40},
		{"8 * 13 = 104", 8, 13, 104},
		{"13 * 21 = 273", 13, 21, 273},
		{"21 * 34 = 714", 21, 34, 714},

		// More Parse-based readable cases
		{"10.0 * 10", Parse("10.0"), 10, Parse("100.0")},
		{"100.0 * 100", Parse("100.0"), 100, Parse("10000.0")},
		{"1.23456789 * 1", Parse("1.23456789"), 1, Parse("1.23456789")},
		{"1.23456789 * 2", Parse("1.23456789"), 2, Parse("2.46913578")},
		{"9.99999999 * 1", Parse("9.99999999"), 1, Parse("9.99999999")},
		{"50000000.0 * 100", Parse("50000000.0"), 100, Parse("5000000000.0")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.d.MulInt(tt.n)
			if got != tt.want {
				t.Errorf("MulInt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMulIntPanic(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		n    int
	}{
		// MinInt64 * -1 = +2^63 which overflows
		{"Min * -1", Min, -1},
		{"-1 * MinInt", -1, math.MinInt},

		// MaxInt64 overflow cases
		{"Max * 2", Max, 2},
		{"Max * -2", Max, -2},
		{"Max * 3", Max, 3},
		{"Max * 10", Max, 10},
		{"Max * MaxInt", Max, math.MaxInt},

		// MinInt64 overflow cases
		{"Min * 2", Min, 2},
		{"Min * -2", Min, -2},
		{"Min * 3", Min, 3},
		{"Min * 10", Min, 10},
		{"Min * MinInt", Min, math.MinInt},

		// Just over sqrt(MaxInt64) squared
		{"3037000500 * 3037000500", 3037000500, 3037000500},
		{"3037000500 * -3037000500", 3037000500, -3037000500},
		{"-3037000500 * 3037000500", -3037000500, 3037000500},

		// Large values that overflow
		{"(1<<32) * (1<<32)", 1 << 32, 1 << 32},
		{"(1<<62) * 2", 1 << 62, 2},
		{"(1<<62) * 3", 1 << 62, 3},
		{"(1<<62) * 4", 1 << 62, 4},

		// Negative large values that overflow
		{"-(1<<62) * 3", -(1 << 62), 3},
		{"-(1<<62) * -3", -(1 << 62), -3},
		{"(1<<62) * -3", 1 << 62, -3},

		// Products just over MaxInt64
		{"(Max/2 + 1) * 2", Max/2 + 1, 2},
		{"(Max/3 + 1) * 3", Max/3 + 1, 3},
		{"(Max/10 + 1) * 10", Max/10 + 1, 10},

		// Symmetric overflow
		{"(1<<40) * (1<<40)", 1 << 40, 1 << 40},
		{"(1<<33) * (1<<33)", 1 << 33, 1 << 33},
		{"(1<<35) * (1<<35)", 1 << 35, 1 << 35},

		// Human-readable overflow cases
		{"9000000000.0 * 2", Parse("9000000000.0"), 2},
		{"5000000000.0 * 3", Parse("5000000000.0"), 3},
		{"1000000000.0 * 10", Parse("1000000000.0"), 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("MulInt(%v, %v) should have panicked", tt.d, tt.n)
				}
			}()
			tt.d.MulInt(tt.n)
		})
	}
}

func BenchmarkMulInt(b *testing.B) {
	d := Parse("123.456")
	for b.Loop() {
		_ = d.MulInt(123)
	}
}
