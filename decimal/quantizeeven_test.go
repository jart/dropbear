package decimal

import (
	"math"
	"testing"
)

func TestDecimal_QuantizeEven_Overflows(t *testing.T) {
	tests := []struct {
		name      string
		d, q      Decimal
		wantPanic bool
	}{
		{
			// MaxInt64 is ODD (...807).
			// Scale (10^9).
			// Max / Scale = 9,223,372,036 (EVEN).
			// Remainder = 854,775,807 (> 500,000,000).
			// Logic: Remainder > Half -> Round Away.
			// Result: 9,223,372,037 * 10^9.
			// 9...037 * 10^9 > MaxInt64.
			// MUST PANIC.
			name:      "MaxInt64 round even overflow (Scale)",
			d:         Decimal(math.MaxInt64),
			q:         Decimal(Scale),
			wantPanic: true,
		},
		{
			// MinInt64 is EVEN (...808).
			// Min / Scale = -9,223,372,036 (EVEN).
			// Remainder = -854,775,808.
			// AbsRem > Half -> Round Away (more negative).
			// Result: -9,223,372,037 * 10^9.
			// < MinInt64.
			// MUST PANIC.
			name:      "MinInt64 round even overflow (Scale)",
			d:         Decimal(math.MinInt64),
			q:         Decimal(Scale),
			wantPanic: true,
		},
		{
			// EDGE CASE: "Half to Even" forces rounding UP into overflow
			// d = MaxInt64. q = 2.
			// quo = Max/2 = ...403 (ODD).
			// rem = 1 (Exact Half).
			// RoundEven: quo is ODD -> Round UP to even (...404).
			// ...404 * 2 = ...808 (Overflow).
			name:      "MaxInt64 round even (q=2, quo is odd)",
			d:         Decimal(math.MaxInt64),
			q:         Decimal(2),
			wantPanic: true,
		},
		{
			// SAFE CASE: "Half to Even" saves us
			// d = MaxInt64 - 2 (Ends in ...805).
			// q = 10.
			// quo = ...80 (Even). rem = 5 (Half).
			// RoundEven: quo is even, keep it. Result ...800.
			// Safe.
			name:      "MaxInt64-2 round even safe (Half to Even)",
			d:         Decimal(math.MaxInt64 - 2),
			q:         Decimal(10),
			wantPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("QuantizeEven() panic = %v, wantPanic %v", r, tt.wantPanic)
				}
			}()

			tt.d.QuantizeEven(tt.q)
		})
	}
}
