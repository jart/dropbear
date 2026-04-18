package decimal

import (
	"testing"
)

func TestDecimal_Half(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		want Decimal
	}{
		// Exact results (even inputs need no rounding)
		{"half of 0", Decimal(0), Decimal(0)},
		{"half of 2", Decimal(2), Decimal(1)},
		{"half of 4", Decimal(4), Decimal(2)},
		{"half of 10", Decimal(10), Decimal(5)},
		{"half of -2", Decimal(-2), Decimal(-1)},
		{"half of -4", Decimal(-4), Decimal(-2)},

		// Banker's rounding: odd inputs hit exact half, round to even
		{"1/2 rounds to 0 (even)", Decimal(1), Decimal(0)},
		{"3/2 rounds to 2 (even)", Decimal(3), Decimal(2)},
		{"5/2 rounds to 2 (even)", Decimal(5), Decimal(2)},
		{"7/2 rounds to 4 (even)", Decimal(7), Decimal(4)},
		{"9/2 rounds to 4 (even)", Decimal(9), Decimal(4)},
		{"11/2 rounds to 6 (even)", Decimal(11), Decimal(6)},

		// Negative banker's rounding
		{"-1/2 rounds to 0 (even)", Decimal(-1), Decimal(0)},
		{"-3/2 rounds to -2 (even)", Decimal(-3), Decimal(-2)},
		{"-5/2 rounds to -2 (even)", Decimal(-5), Decimal(-2)},
		{"-7/2 rounds to -4 (even)", Decimal(-7), Decimal(-4)},
		{"-9/2 rounds to -4 (even)", Decimal(-9), Decimal(-4)},

		// Larger values
		{"half of 1.000000", Parse("1"), Parse("0.5")},
		{"half of 0.500000", Parse("0.5"), Parse("0.25")},
		{"half of 100", Parse("100"), Parse("50")},
		{"half of 0.000003", Decimal(3), Decimal(2)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.d.Half(); got != tt.want {
				t.Errorf("Decimal(%d).Half() = %d (%s), want %d (%s)",
					int64(tt.d), int64(got), got, int64(tt.want), tt.want)
			}
		})
	}
}

func TestDecimal_HalfEqualsDivInt2(t *testing.T) {
	min, max := int64(Min), int64(Max)
	for _, i := range []int64{
		min, min + 1, min + 2, min + 3, min + 4, min + 5,
		-5, -4, -3, -2, -1, 0, 1, 2, 3, 4, 5,
		max - 5, max - 4, max - 3, max - 2, max - 1, max,
	} {
		d := Decimal(i)
		got := d.Half()
		want := d.DivInt(2)
		if got != want {
			t.Fatalf("Decimal(%d): Half()=%d, DivInt(2)=%d", i, int64(got), int64(want))
		}
	}
}

func BenchmarkDecimal_Half(b *testing.B) {
	d := Parse("3.141593")
	for b.Loop() {
		_ = d.Half()
	}
}
