package decimal

import "testing"

func TestDecimal_String(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		want string
	}{
		{"zero", Zero, "0"},
		{"one", One, "1"},
		{"satoshi", Satoshi, "0.00000001"},
		{"max", Max, "92233720368.54775807"},
		{"min", Min, "-92233720368.54775808"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.String(); got != tt.want {
				t.Errorf("Decimal.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
