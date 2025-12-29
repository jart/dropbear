package decimal

import "testing"

func TestDecimal_Format(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		n    int
		want string
	}{
		{"simple integer", Parse("123"), 2, "123.00"},
		{"simple fractional", Parse("123.456"), 2, "123.46"},
		{"simple fractional rounding down", Parse("123.454"), 2, "123.45"},
		{"negative fractional", Parse("-0.00123"), 4, "-0.0012"},
		{"zero", Parse("0"), 3, "0.000"},
		{"rounding up", Parse("1.9999"), 3, "2.000"},
		{"lots of digits", Parse("1.23456789"), 9, "1.234567890"},
		{"bone", Parse("1.23456789"), 5, "1.23457"},
		{"int64 max 9 places", Decimal(Max), 9, "9223372036.854775807"},
		{"int64 max 8 places", Decimal(Max), 8, "9223372036.85477581"},
		{"int64 max 0 places", Decimal(Max), 0, "9223372037"},
		{"int64 min 9 places", Decimal(Min), 9, "-9223372036.854775808"},
		{"int64 min 8 places", Decimal(Min), 8, "-9223372036.85477581"},
		{"int64 min 0 places", Decimal(Min), 0, "-9223372037"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.Format(tt.n); got != tt.want {
				t.Errorf("Decimal.Format() = %v, want %v", got, tt.want)
			}
		})
	}
}
