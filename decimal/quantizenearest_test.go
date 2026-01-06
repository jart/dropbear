package decimal

import "testing"

func TestDecimal_QuantizeNearest(t *testing.T) {
	tests := []struct {
		d, q, want Decimal
	}{
		// Basic cases with quantum of 40
		{FromInt(100), FromInt(40), FromInt(120)},  // 100 is closer to 120 than 80
		{FromInt(99), FromInt(40), FromInt(80)},    // 99 is closer to 80 than 120
		{FromInt(120), FromInt(40), FromInt(120)},
		{FromInt(159), FromInt(40), FromInt(160)},  // 159 is closer to 160 than 120
		{FromInt(140), FromInt(40), FromInt(160)},  // 140 is exactly halfway, rounds away from zero
		{FromInt(160), FromInt(40), FromInt(160)},
		{FromInt(19), FromInt(40), FromInt(0)},     // 19 is closer to 0 than 40
		{FromInt(20), FromInt(40), FromInt(40)},    // 20 is exactly halfway, rounds away from zero
		{FromInt(21), FromInt(40), FromInt(40)},    // 21 is closer to 40 than 0
		{FromInt(40), FromInt(40), FromInt(40)},
		{FromInt(60), FromInt(40), FromInt(80)},    // 60 is exactly halfway, rounds away from zero
		{FromInt(61), FromInt(40), FromInt(80)},
		{FromInt(0), FromInt(40), FromInt(0)},

		// Negative values - halfway rounds away from zero (more negative)
		{FromInt(-19), FromInt(40), FromInt(0)},
		{FromInt(-20), FromInt(40), FromInt(-40)},  // halfway rounds away from zero
		{FromInt(-21), FromInt(40), FromInt(-40)},
		{FromInt(-40), FromInt(40), FromInt(-40)},
		{FromInt(-60), FromInt(40), FromInt(-80)},  // halfway rounds away from zero
		{FromInt(-61), FromInt(40), FromInt(-80)},

		// Quantum of 1 (standard rounding)
		{Parse("1.4"), FromInt(1), FromInt(1)},
		{Parse("1.5"), FromInt(1), FromInt(2)},
		{Parse("1.6"), FromInt(1), FromInt(2)},
		{Parse("-1.4"), FromInt(1), FromInt(-1)},
		{Parse("-1.5"), FromInt(1), FromInt(-2)},

		// Fractional quantum
		{Parse("1.24"), Parse("0.5"), Parse("1")},
		{Parse("1.25"), Parse("0.5"), Parse("1.5")},  // halfway rounds away
		{Parse("1.26"), Parse("0.5"), Parse("1.5")},
		{Parse("1.74"), Parse("0.5"), Parse("1.5")},
		{Parse("1.75"), Parse("0.5"), Parse("2")},    // halfway rounds away
		{Parse("-1.25"), Parse("0.5"), Parse("-1.5")},

		// Standard decimal places
		{Parse("123.454"), Parse("0.01"), Parse("123.45")},
		{Parse("123.455"), Parse("0.01"), Parse("123.46")},  // halfway rounds away
		{Parse("123.456"), Parse("0.01"), Parse("123.46")},
	}

	for _, tt := range tests {
		got := tt.d.QuantizeNearest(tt.q)
		if got != tt.want {
			t.Errorf("%s.QuantizeNearest(%s) = %s, want %s", tt.d, tt.q, got, tt.want)
		}
	}
}
