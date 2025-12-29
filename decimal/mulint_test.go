package decimal

import (
	"math"
	"testing"
)

func TestDecimal_MulInt_Boundary(t *testing.T) {
	tests := []struct {
		name      string
		d         Decimal
		n         int
		want      Decimal
		wantPanic bool
	}{
		// --- Sanity Checks ---
		{
			name: "Small: 10 * 2 = 20",
			d:    Decimal(10),
			n:    2,
			want: Decimal(20),
		},
		{
			name: "Signs: 10 * -2 = -20",
			d:    Decimal(10),
			n:    -2,
			want: Decimal(-20),
		},
		{
			name: "Zero: 10 * 0 = 0",
			d:    Decimal(10),
			n:    0,
			want: Decimal(0),
		},

		// --- MaxInt64 Boundaries ---
		{
			name: "Max * 1 = Max",
			d:    Decimal(math.MaxInt64),
			n:    1,
			want: Decimal(math.MaxInt64),
		},
		{
			name: "Max * -1 = -Max",
			d:    Decimal(math.MaxInt64),
			n:    -1,
			want: Decimal(-math.MaxInt64),
		},
		{
			name:      "Max * 2 = Overflow",
			d:         Decimal(math.MaxInt64),
			n:         2,
			wantPanic: true,
		},

		// --- MinInt64 Boundaries ---
		{
			name: "Min * 1 = Min",
			d:    Decimal(math.MinInt64),
			n:    1,
			want: Decimal(math.MinInt64),
		},
		{
			name: "Min * -1 = Overflow",
			// -2^63 * -1 = +2^63. Max is 2^63 - 1.
			d:         Decimal(math.MinInt64),
			n:         -1,
			wantPanic: true,
		},
		{
			name: "Min * 0 = 0",
			d:    Decimal(math.MinInt64),
			n:    0,
			want: Decimal(0),
		},

		// --- Mixed Boundaries ---
		{
			name: "MaxInt32 * MaxInt32 = Fits in Int64",
			// 2^31-1 * 2^31-1 approx 2^62. Fits safely.
			d:    Decimal(math.MaxInt32),
			n:    math.MaxInt32,
			want: Decimal(int64(math.MaxInt32) * int64(math.MaxInt32)),
		},
		{
			name: "Overflow by 1",
			// We need numbers that multiply to exactly MaxInt64 + 1.
			// Hard to find exact integer factors, so we use bounds.
			// 3,037,000,500 * 3,037,000,500 is roughly 9.2233...e18
			// Let's use simpler logic: MinInt64 * -1 is the canonical 'by 1' overflow.
			d:         Decimal(math.MinInt64),
			n:         -1,
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("MulInt() panic = %v, wantPanic %v", r, tt.wantPanic)
				}
			}()

			got := tt.d.MulInt(tt.n)
			if got != tt.want {
				t.Errorf("MulInt() = %v, want %v", got, tt.want)
			}
		})
	}
}
