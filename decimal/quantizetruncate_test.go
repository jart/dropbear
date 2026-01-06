package decimal

import "testing"

func TestDecimal_QuantizeTruncate(t *testing.T) {
	tests := []struct {
		d, q, want Decimal
	}{
		// Basic cases with quantum of 40
		{FromInt(100), FromInt(40), FromInt(80)},
		{FromInt(120), FromInt(40), FromInt(120)},
		{FromInt(159), FromInt(40), FromInt(120)},
		{FromInt(160), FromInt(40), FromInt(160)},
		{FromInt(39), FromInt(40), FromInt(0)},
		{FromInt(40), FromInt(40), FromInt(40)},
		{FromInt(41), FromInt(40), FromInt(40)},
		{FromInt(79), FromInt(40), FromInt(40)},
		{FromInt(80), FromInt(40), FromInt(80)},
		{FromInt(0), FromInt(40), FromInt(0)},

		// Negative values truncate toward zero
		{FromInt(-40), FromInt(40), FromInt(-40)},
		{FromInt(-41), FromInt(40), FromInt(-40)},
		{FromInt(-79), FromInt(40), FromInt(-40)},
		{FromInt(-80), FromInt(40), FromInt(-80)},
		{FromInt(-39), FromInt(40), FromInt(0)},

		// Fractional quantum
		{Parse("1.25"), Parse("0.5"), Parse("1")},
		{Parse("1.75"), Parse("0.5"), Parse("1.5")},
		{Parse("-1.25"), Parse("0.5"), Parse("-1")},
		{Parse("-1.75"), Parse("0.5"), Parse("-1.5")},

		// Standard decimal places
		{Parse("123.456"), Parse("0.01"), Parse("123.45")},
		{Parse("123.459"), Parse("0.01"), Parse("123.45")},
		{Parse("-123.456"), Parse("0.01"), Parse("-123.45")},
	}

	for _, tt := range tests {
		got := tt.d.QuantizeTruncate(tt.q)
		if got != tt.want {
			t.Errorf("%s.QuantizeTruncate(%s) = %s, want %s", tt.d, tt.q, got, tt.want)
		}
	}
}
