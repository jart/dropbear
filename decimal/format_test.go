package decimal

import "testing"

func TestFormat(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		n    int
		want string
	}{
		{"zero", Parse("0"), 3, "0.000"},
		{"simple integer", Parse("123"), 2, "123.00"},
		{"simple fractional", Parse("123.456"), 2, "123.46"},
		{"simple fractional rounding down", Parse("123.454"), 2, "123.45"},
		{"negative fractional", Parse("-0.00123"), 4, "-0.0012"},
		{"rounding up", Parse("1.9999"), 3, "2.000"},
		{"lots of digits", Parse("1.23456789"), 6, "1.234568"},
		{"bone", Parse("1.23456789"), 5, "1.23457"},
		{"int64 max 6 places", Decimal(Max), 6, "9223372036854.775807"},
		{"int64 max 5 places", Decimal(Max), 5, "9223372036854.77581"},
		{"int64 max 0 places", Decimal(Max), 0, "9223372036855"},
		{"int64 min 6 places", Decimal(Min), 6, "-9223372036854.775808"},
		{"int64 min 5 places", Decimal(Min), 5, "-9223372036854.77581"},
		{"int64 min 0 places", Decimal(Min), 0, "-9223372036855"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.Format(tt.n); got != tt.want {
				t.Errorf("Decimal.Format() = %v, want %v", got, tt.want)
			}
		})
	}
}
