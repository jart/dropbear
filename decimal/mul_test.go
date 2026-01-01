package decimal

import (
	"math"
	"testing"
)

func TestDecimal_Mul_Boundary(t *testing.T) {
	tests := []struct {
		name      string
		d, o      Decimal
		want      Decimal
		wantPanic bool
	}{
		// --- Sanity Checks ---
		{"1 * 1 = 1", One, One, One, false},
		{"1 * -1 = -1", One, NegOne, NegOne, false},
		{"-1 * -1 = 1", NegOne, NegOne, One, false},

		// --- MaxInt64 Boundaries ---
		{
			name: "Max * 1 = Max",
			d:    Max,
			o:    One,
			want: Max,
		},
		{
			name: "Max * -1 = -Max",
			d:    Max,
			o:    NegOne,
			want: -Max,
		},
		{
			name:      "Max * 2 = Overflow",
			d:         Max,
			o:         Decimal(2 * Scale),
			wantPanic: true,
		},

		// --- MinInt64 Boundaries ---
		{
			name: "Min * 1 = Min",
			d:    Min,
			o:    One,
			want: Min, // Valid in Pixel Perfect impl
		},
		{
			name: "Min * -1 = Overflow",
			// Explain: Min is -2^63. Result should be +2^63.
			// MaxInt64 is 2^63 - 1. Therefore, this MUST panic.
			d:         Min,
			o:         NegOne,
			wantPanic: true,
		},
		{
			name: "Min * 0.5 = Min / 2",
			d:    Min,
			o:    Decimal(Scale / 2),
			want: Decimal(math.MinInt64 / 2),
		},

		// --- Precision / Rounding ---
		{
			name:      "Max * (1+epsilon) = Overflow",
			d:         Max,
			o:         One.Add(Epsilon),
			wantPanic: true,
		},
		{
			name: "Smallest Non-Zero Product",
			d:    Epsilon,
			o:    One,
			want: Epsilon,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("Mul() panic = %v, wantPanic %v", r, tt.wantPanic)
				}
			}()

			got := tt.d.Mul(tt.o)
			if got != tt.want {
				t.Errorf("Mul() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGeminiBreak(t *testing.T) {
	// 1. Rounding Consistency
	// Mul now uses Banker's Rounding, same as Parse.
	t.Run("RoundingConsistency", func(t *testing.T) {
		// 1.00000001 * 1.5 = 1.500000015
		// Parse("1.500000015") -> 1.50000002 (Round to Even)
		// Mul should now be 1.50000002

		d1 := Parse("1.00000001")
		d2 := Parse("1.5")

		mulResult := d1.Mul(d2)
		expectedMath := "1.500000015"
		parseResult := Parse(expectedMath)

		if mulResult != parseResult {
			t.Errorf("Still inconsistent! Mul: %s, Parse: %s", mulResult, parseResult)
		} else {
			t.Logf("Consistent! Mul: %s, Parse: %s", mulResult, parseResult)
		}
	})

	// 2. Check for Overflow in Parse
	// The comment in degenerate_test.go suggested silent overflow.
	// Our tests show it DOES panic with "toasted number".
	t.Run("ParseOverflow", func(t *testing.T) {
		inputs := []string{"1e11", "1e12", "1e19"}
		for _, input := range inputs {
			t.Run(input, func(t *testing.T) {
				defer func() {
					if r := recover(); r != nil {
						// Expected panic
						if r != "toasted number" {
							t.Logf("Parse(%s) panicked with unexpected message: %v", input, r)
						}
					}
				}()
				_ = Parse(input)
				// If we reach here, it didn't panic
				t.Errorf("Parse(\"%s\") did not panic!", input)
			})
		}
	})

	// Check FromFloat64 vs Mul consistency
	t.Run("FromFloat64Consistency", func(t *testing.T) {
		// 1.000000015
		// FromFloat64(1.500000015) uses math.Round (Half Away From Zero) -> 1.50000002
		// Mul should now match (Banker's rounding also yields .02 for .015 if odd)
		// Wait, 1.50000001 (odd) -> .015 rounds up to even .02? No.
		// 1.50000001 is 150000001.
		// 150000001 * 1.5?
		// d1=1.00000001. d2=1.5.
		// 100000001 * 150000000 = 15000000150000000
		// Div 10^8 -> 150000001.5
		// Remainder is exactly half (Scale/2).
		// Quotient is 150000001 (odd).
		// Banker's Rounding: Odd + 0.5 -> Round Up -> 150000002.
		// So it should be 1.50000002.

		dMul := Parse("1.00000001").Mul(Parse("1.5"))

		dExpected := FromFloat64(1.500000015)

		if dMul != dExpected {
			t.Errorf("Mismatch: Mul=%s, FromFloat64=%s", dMul, dExpected)
		} else {
			t.Logf("Match: Mul=%s, FromFloat64=%s", dMul, dExpected)
		}
	})

	// 3. Check Mul Overflow behavior (Runtime Panic vs panicOverflow)
	t.Run("MulRuntimePanic", func(t *testing.T) {
		// MaxInt64 * MaxInt64 / Scale should panic.
		// If it causes a runtime panic (integer overflow) instead of "decimal overflow",
		// it might be what the user meant.
		d1 := Max
		d2 := Max

		defer func() {
			r := recover()
			if r == nil {
				t.Error("Mul did not panic")
			} else {
				t.Logf("Mul panicked with: %v", r)
				if r == "decimal overflow" {
					t.Log("Panic type: Custom decimal overflow")
				} else {
					t.Log("Panic type: Runtime/Other")
				}
			}
		}()
		_ = d1.Mul(d2)
	})

	// 4. Check Mul Overflow without Panic?
	// Is there a case where hi < Scale but quo > MaxInt64 AND sign check fails?
	// quo > MaxInt64 covers positive overflow.
	// We need quo > MaxInt64 but logic doesn't catch it? No, logic is simple.
}

func TestStringFormat(t *testing.T) {
	// Test if we can generate a wrong string
	// e.g. -0
	// d = 0.
	if Zero.String() != "0" {
		t.Errorf("Zero.String() = %s", Zero.String())
	}
}
