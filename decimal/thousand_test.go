package decimal

import "testing"

func TestDecimal_FormatThousand(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		n    int
		want string
	}{
		// remember: Decimal has 9 implicit decimal places (Scale = 1e9)
		// Decimal(123456789) = 0.123456789
		{"fractional", Decimal(123456789), 2, "0.12"},
		{"fractional_neg", Decimal(-123456789), 3, "-0.123"},
		{"zero", Decimal(0), 2, "0.00"},

		// use Parse() for realistic values
		{"one", Parse("1"), 2, "1.00"},
		{"thousands", Parse("1234.56"), 2, "1,234.56"},
		{"millions", Parse("1234567.89"), 2, "1,234,567.89"},
		{"billions", Parse("1234567890.12"), 2, "1,234,567,890.12"},
		{"negative", Parse("-9876543.21"), 2, "-9,876,543.21"},

		// rounding
		{"round_up", Parse("1.235"), 2, "1.24"},
		{"round_down", Parse("1.234"), 2, "1.23"},
		{"round_half", Parse("1.225"), 2, "1.23"}, // 5 rounds up

		// edge cases
		{"no_decimals", Parse("1234567"), 0, "1,234,567"},
		{"many_decimals", Parse("1.123456789"), 8, "1.12345679"},
		{"small_value", Parse("0.00001234"), 5, "0.00001"},

		// push it to the limit
		{"int64 max 8 places", Decimal(Max), 8, "9,223,372,036.85477581"},
		{"int64 max 7 places", Decimal(Max), 7, "9,223,372,036.8547758"},
		{"int64 max 0 places", Decimal(Max), 0, "9,223,372,037"},
		{"int64 min 8 places", Decimal(Min), 8, "-9,223,372,036.85477581"},
		{"int64 min 7 places", Decimal(Min), 7, "-9,223,372,036.8547758"},
		{"int64 min 0 places", Decimal(Min), 0, "-9,223,372,037"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.FormatThousand(tt.n); got != tt.want {
				t.Errorf("FormatThousand(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}
