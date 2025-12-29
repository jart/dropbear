package decimal

import (
	"math"
	"testing"
)

func TestDivIntEven(t *testing.T) {
	tests := []struct {
		name     string
		d        Decimal
		n        int
		expected Decimal
	}{
		// --- Basic Exact Division ---
		{"Exact Positive", 10, 2, 5},
		{"Exact Negative", -10, 2, -5},

		// --- Normal Rounding (Not Half) ---
		{"Round Down Positive", 10, 3, 3},   // +0.000000010 / 3 = +0.000000003
		{"Round Up Positive", 11, 3, 4},     // +0.000000011 / 3 = +0.000000004
		{"Round Down Negative", -10, 3, -3}, // -0.000000010 / 3 = -0.000000003
		{"Round Up Negative", -11, 3, -4},   // -0.000000011 / 3 = -0.000000004

		// --- Banker's Rounding (Tie Breaking) ---

		// Case 1: Quotient is Even, Remainder is Half -> Keep Quotient

		// Raw Math:      5 / 2 = 2.5. Nearest even integer is 2.
		// Fixed Point:   0.000000005 / 2 = 0.0000000025 -> rounds to 0.000000002
		{"Tie/EvenQuo/Pos", 5, 2, 2},

		// Raw Math:      13 / 2 = 6.5. Nearest even integer is 6.
		// Fixed Point:   0.000000013 / 2 = 0.0000000065 -> rounds to 0.000000006
		{"Tie/EvenQuo/Pos2", 13, 2, 6},

		// Raw Math:      -5 / 2 = -2.5. Nearest even integer is -2.
		// Fixed Point:   -0.000000005 / 2 = -0.0000000025 -> rounds to -0.000000002
		{"Tie/EvenQuo/Neg", -5, 2, -2},

		// Case 2: Quotient is Odd, Remainder is Half -> Round Away to make Even

		// Raw Math:      3 / 2 = 1.5. Nearest even integer is 2.
		// Fixed Point:   0.000000003 / 2 = 0.0000000015 -> rounds to 0.000000002
		{"Tie/OddQuo/Pos", 3, 2, 2},

		// Raw Math:      -3 / 2 = -1.5. Nearest even integer is -2.
		// Fixed Point:   -0.000000003 / 2 = -0.0000000015 -> rounds to -0.000000002
		{"Tie/OddQuo/Neg", -3, 2, -2},

		// --- Negative Divisor Logic (Triggering "if y < 0") ---

		// 1. Standard Rounding with Negative Divisor
		// 5 / -3 = -1.66... -> rounds to -2
		{"NegDiv/RoundAway", 5, -3, -2},

		// 2. Banker's Rounding (Tie) with Negative Divisor

		// Case A: Result is -2.5. Nearest even is -2.
		// logic: quo starts at -2. rem is 1. absRem(1) == half(1).
		// quo is even (-2), so we do NOT round away.
		{"NegDiv/Tie/EvenQuo", 5, -2, -2},

		// Case B: Result is -1.5. Nearest even is -2.
		// logic: quo starts at -1. rem is 1. absRem(1) == half(1).
		// quo is odd (-1), so we ROUND AWAY.
		// (x<0) is false, (y<0) is true -> mismatch -> quo-- -> -2
		{"NegDiv/Tie/OddQuo", 3, -2, -2},

		// 3. Signs Mismatch (Negative d, Negative n)
		// -5 / -2 = 2.5. Nearest even is 2.
		// quo starts at 2. rem is -1.
		// quo is even. No round.
		{"NegDiv/NegD/Tie", -5, -2, 2},

		// --- Large Number / Magnitude Logic Checks ---
		{
			"MaxInt64 Exact",
			Decimal(math.MaxInt64), 1,
			Decimal(math.MaxInt64),
		},
		{
			"MinInt64 Exact",
			Decimal(math.MinInt64), 1,
			Decimal(math.MinInt64),
		},
		{
			// MinInt64 is even, so it divides by 2 perfectly.
			"MinInt64 / 2",
			Decimal(math.MinInt64), 2,
			Decimal(math.MinInt64 / 2),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.d.DivIntEven(tt.n)
			if got != tt.expected {
				t.Errorf("DivIntEven(%d, %d) = %d; want %d", tt.d, tt.n, got, tt.expected)
			}
			got2 := tt.d.DivEven(FromInt(tt.n))
			if got2 != tt.expected {
				t.Errorf("DivIntEven inconsistent with DivEven(%d, %d) = %d; want %d", tt.d, tt.n, got2, tt.expected)
			}
		})
	}
}

func TestDivIntEven_Panics(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		n    int
	}{
		{
			name: "Division by Zero",
			d:    100,
			n:    0,
		},
		{
			name: "Overflow (MinInt64 / -1)",
			d:    Decimal(math.MinInt64),
			n:    -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("did not panic")
				}
			}()
			tt.d.DivIntEven(tt.n)
		})
		t.Run(tt.name+" vs. DivEven", func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("did not panic")
				}
			}()
			tt.d.DivEven(FromInt(tt.n))
		})
	}
}
