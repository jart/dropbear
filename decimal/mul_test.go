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
