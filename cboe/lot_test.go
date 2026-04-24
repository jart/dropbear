package cboe

import (
	"dropbear/decimal"
	"testing"
)

func TestLotSize(t *testing.T) {
	type args struct {
		price decimal.Decimal
	}
	tests := []struct {
		name string
		arg  decimal.Decimal
		want decimal.Decimal
	}{
		{"10000", decimal.Parse("10000"), decimal.One},
		{"9999.99", decimal.Parse("9999.99"), decimal.Ten},
		{"1000", decimal.Parse("1000"), decimal.Ten},
		{"999.99", decimal.Parse("999.99"), k40},
		{"250", decimal.Parse("250"), k40},
		{"249.99", decimal.Parse("249.99"), k100},
		{"0.01", decimal.Parse("0.01"), k100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LotSize(tt.arg); got.Cmp(tt.want) != 0 {
				t.Errorf("LotSize() = %v, want %v", got, tt.want)
			}
		})
	}
}
