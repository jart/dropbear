package decimal

import "testing"

func TestDecimal_Precision(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		want int
	}{
		{name: "0", d: Parse("0"), want: 0},
		{name: "10", d: Parse("10"), want: 0},
		{name: "1", d: Parse("1"), want: 0},
		{name: "0.1", d: Parse("0.1"), want: 1},
		{name: "0.01", d: Parse("0.01"), want: 2},
		{name: "0.001", d: Parse("0.001"), want: 3},
		{name: "0.0001", d: Parse("0.0001"), want: 4},
		{name: "0.00001", d: Parse("0.00001"), want: 5},
		{name: "0.000001", d: Parse("0.000001"), want: 6},
		{name: "-10", d: Parse("-10"), want: 0},
		{name: "-1", d: Parse("-1"), want: 0},
		{name: "-0.1", d: Parse("-0.1"), want: 1},
		{name: "-0.01", d: Parse("-0.01"), want: 2},
		{name: "-0.001", d: Parse("-0.001"), want: 3},
		{name: "-0.0001", d: Parse("-0.0001"), want: 4},
		{name: "-0.00001", d: Parse("-0.00001"), want: 5},
		{name: "-0.000001", d: Parse("-0.000001"), want: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.Precision(); got != tt.want {
				t.Errorf("Decimal.Precision() = %v, want %v", got, tt.want)
			}
		})
	}
}
