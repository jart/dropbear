package decimal

import "testing"

func TestDecimal_FormatThousand(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		n    int
		want string
	}{
		// remember: Decimal has 6 implicit decimal places (Scale = 1e6)
		// Decimal(1234567) = 1.234567
		{"fractional", Decimal(1234567), 2, "1.23"},
		{"fractional_neg", Decimal(-1234567), 3, "-1.235"},
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
		{"many_decimals", Parse("1.123456789"), 6, "1.123457"},
		{"small_value", Parse("0.00001234"), 5, "0.00001"},

		// push it to the limit
		{"int64 max 6 places", Decimal(Max), 6, "9,223,372,036,854.775807"},
		{"int64 max 5 places", Decimal(Max), 5, "9,223,372,036,854.77581"},
		{"int64 max 0 places", Decimal(Max), 0, "9,223,372,036,855"},
		{"int64 min 6 places", Decimal(Min), 6, "-9,223,372,036,854.775808"},
		{"int64 min 5 places", Decimal(Min), 5, "-9,223,372,036,854.77581"},
		{"int64 min 0 places", Decimal(Min), 0, "-9,223,372,036,855"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.FormatThousand(tt.n); got != tt.want {
				t.Errorf("FormatThousand(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}
