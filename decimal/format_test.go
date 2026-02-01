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
		{"lots of digits", Parse("1.23456789"), 8, "1.23456789"},
		{"bone", Parse("1.23456789"), 5, "1.23457"},
		{"int64 max 8 places", Decimal(Max), 8, "9223372036.85477581"},
		{"int64 max 7 places", Decimal(Max), 7, "9223372036.8547758"},
		{"int64 max 0 places", Decimal(Max), 0, "9223372037"},
		{"int64 min 8 places", Decimal(Min), 8, "-9223372036.85477581"},
		{"int64 min 7 places", Decimal(Min), 7, "-9223372036.8547758"},
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
