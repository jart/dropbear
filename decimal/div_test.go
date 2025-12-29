package decimal

import (
	"testing"
)

func TestDiv_Boundary(t *testing.T) {
	tests := []struct {
		name      string
		d, o      Decimal
		want      Decimal
		wantPanic bool
	}{
		// --- Sanity Checks ---
		{"1 / 1 = 1", One, One, One, false},
		{"1 / -1 = -1", One, NegOne, NegOne, false},

		// --- MaxInt64 Boundaries ---
		{
			name: "Max / 1 = Max",
			d:    Max,
			o:    One,
			want: Max,
		},
		{
			name: "Max / -1 = -Max",
			d:    Max,
			o:    NegOne,
			want: -Max,
		},
		{
			name:      "Max / 0.5 = Overflow",
			d:         Max,
			o:         Decimal(Scale / 2),
			wantPanic: true,
		},

		// --- MinInt64 Boundaries ---
		{
			name: "Min / 1 = Min",
			d:    Min,
			o:    One,
			want: Min,
		},
		{
			name: "Min / -1 = Overflow",
			// Explain: -2^63 / -1 = +2^63. Max is 2^63-1. Must panic.
			d:         Min,
			o:         NegOne,
			wantPanic: true,
		},
		{
			name: "Min / 0.5 = Overflow",
			// Explain: -2^63 / 0.5 = -2^64. Way out of bounds.
			d:         Min,
			o:         Decimal(Scale / 2),
			wantPanic: true,
		},

		// --- Rounding Checks (Half Away From Zero) ---
		{
			name: "1 / 3 (Round Down)",
			d:    One,
			o:    Decimal(3 * Scale),
			want: Decimal(33_333_333), // 0.333...
		},
		{
			name: "2 / 3 (Round Up)",
			d:    Two,
			o:    Decimal(3 * Scale),
			want: Decimal(66_666_667), // 0.666...7
		},
		{
			name: "1 / 2 (Round Up)",
			d:    One,
			o:    Two,
			want: Decimal(Scale / 2), // 0.5
		},
		{
			name: "-5 / 2 == -3",
			d:    -5,
			o:    2 * Scale,
			want: -3,
		},

		// --- Zero Division ---
		{
			name:      "Division by Zero",
			d:         One,
			o:         Decimal(0),
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("Div(%v, %v) panic = %v, wantPanic %v", tt.d, tt.o, r, tt.wantPanic)
				}
			}()

			got := tt.d.Div(tt.o)
			if got != tt.want {
				t.Errorf("Div(%v, %v) = %v, want %v", tt.d, tt.o, got, tt.want)
			}
		})
	}
}
