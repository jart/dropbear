package decimal

import "testing"

func TestString(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		want string
	}{
		{"zero", Zero, "0"},
		{"one", One, "1"},
		{"1.1", Parse("1.10"), "1.1"},
		{"epsilon", Epsilon, "0.000001"},
		{"max", Max, "9223372036854.775807"},
		{"min", Min, "-9223372036854.775808"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.String(); got != tt.want {
				t.Errorf("Decimal.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
