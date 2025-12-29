package decimal

import (
	"math"
	"testing"
)

func TestDecimal_QuantizeAway_Overflows(t *testing.T) {
	tests := []struct {
		name      string
		d, q      Decimal
		wantPanic bool
	}{
		{
			// MaxInt64 is odd.
			// q = 2.
			// rem = 1.
			// QuantizeAway MUST round up to the next even number (Max + 1).
			// This overflows MaxInt64.
			name:      "MaxInt64 round away (q=2)",
			d:         Decimal(math.MaxInt64),
			q:         Decimal(2),
			wantPanic: true,
		},
		{
			// MaxInt64 with Scale (10^9).
			// Max / Scale = 9,223,372,036.
			// Remainder exists.
			// QuantizeAway rounds up to 9,223,372,037 * 10^9.
			// This exceeds MaxInt64.
			name:      "MaxInt64 round away (Scale)",
			d:         Decimal(math.MaxInt64),
			q:         Decimal(Scale),
			wantPanic: true,
		},
		{
			// MinInt64 (-9...808).
			// q = Scale.
			// Remainder exists (it's not an exact multiple of 10^9).
			// QuantizeAway rounds away (more negative) to next billion.
			// Result < MinInt64.
			name:      "MinInt64 round away (Scale)",
			d:         Decimal(math.MinInt64),
			q:         Decimal(Scale),
			wantPanic: true,
		},
		{
			// SAFE CASE: Exact Multiple
			// If d is already a multiple of q, no rounding occurs.
			// 100 is a multiple of 10.
			name:      "Exact multiple (Safe)",
			d:         Decimal(100),
			q:         Decimal(10),
			wantPanic: false,
		},
		{
			// SAFE CASE: Truncation Direction
			// If d = 11, q = 10.
			// QuantizeAway -> 20. Safe.
			name:      "Small numbers (Safe)",
			d:         Decimal(11),
			q:         Decimal(10),
			wantPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("QuantizeAway() panic = %v, wantPanic %v", r, tt.wantPanic)
				}
			}()

			tt.d.QuantizeAway(tt.q)
		})
	}
}
