package decimal

import (
	"math"
	"testing"
)

func TestDecimal_DivInt_Boundary(t *testing.T) {
	tests := []struct {
		name      string
		d         Decimal
		n         int
		want      Decimal
		wantPanic bool
	}{
		// --- Rounding Mechanics (Round Half Away From Zero) ---
		{
			name: "Exact Division: 6 / 3 = 2",
			d:    Decimal(6),
			n:    3,
			want: Decimal(2),
		},
		{
			name: "Round Down: 10 / 3 = 3.33... -> 3",
			d:    Decimal(10),
			n:    3,
			want: Decimal(3),
		},
		{
			name: "Round Up: 20 / 3 = 6.66... -> 7",
			d:    Decimal(20),
			n:    3,
			want: Decimal(7),
		},
		{
			name: "Halfway Round Up (Positive): 5 / 2 = 2.5 -> 3",
			d:    Decimal(5),
			n:    2,
			want: Decimal(3),
		},
		{
			name: "Halfway Round Down (Negative): -5 / 2 = -2.5 -> -3",
			d:    Decimal(-5),
			n:    2,
			want: Decimal(-3),
		},
		{
			name: "Mixed Signs Rounding: 5 / -2 = -2.5 -> -3",
			d:    Decimal(5),
			n:    -2,
			want: Decimal(-3),
		},

		// --- Odd Divisors (The (N+1)/2 logic check) ---
		{
			name: "Odd Divisor Halfway Check: 2 / 5 = 0.4 -> 0",
			d:    Decimal(2),
			n:    5,
			want: Decimal(0),
		},
		{
			name: "Odd Divisor Halfway Check: 3 / 5 = 0.6 -> 1",
			d:    Decimal(3),
			n:    5,
			want: Decimal(1),
		},

		// --- Extreme Boundaries ---
		{
			name: "MinInt64 / 1 = MinInt64",
			d:    Decimal(math.MinInt64),
			n:    1,
			want: Decimal(math.MinInt64),
		},
		{
			name: "MinInt64 / MaxInt64 = -1 (Round Down)",
			d:    Decimal(math.MinInt64),
			n:    math.MaxInt64,
			// -9223372036854775808 / 9223372036854775807 approx -1.000...001
			// Should round to -1
			want: Decimal(-1),
		},
		{
			name:      "MinInt64 / -1 = Overflow Panic",
			d:         Decimal(math.MinInt64),
			n:         -1,
			wantPanic: true, // The only integer division overflow
		},
		{
			name: "MaxInt64 / -1 = -MaxInt64",
			d:    Decimal(math.MaxInt64),
			n:    -1,
			want: Decimal(-math.MaxInt64),
		},
		{
			name:      "Division by Zero",
			d:         Decimal(100),
			n:         0,
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("DivInt() panic = %v, wantPanic %v", r, tt.wantPanic)
				}
			}()

			got := tt.d.DivInt(tt.n)
			if got != tt.want {
				t.Errorf("DivInt() = %v, want %v", got, tt.want)
			}
		})
	}
}
