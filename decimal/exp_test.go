package decimal

import (
	"math"
	"testing"
)

func TestExp(t *testing.T) {
	tests := []float64{
		0,
		0.5,
		1,
		-1,
		2,
		-2,
		0.1,
		-0.1,
		0.01,
		3,
		7,
	}
	for _, x := range tests {
		d := Parse(formatFloat(x))
		got := d.Exp().Float64()
		want := math.Exp(x)
		diff := math.Abs(got - want)
		if diff > 0.000001 {
			t.Errorf("Exp(%v): got %v, want %v (diff %v)", x, got, want, diff)
		}
	}
}

func formatFloat(f float64) string {
	return Parse("0").Add(Decimal(int64(f * Scale))).String()
}

func BenchmarkExp(b *testing.B) {
	d := Parse("0.5")
	for i := 0; i < b.N; i++ {
		d.Exp()
	}
}

func BenchmarkExpNegative(b *testing.B) {
	d := Parse("-0.5")
	for i := 0; i < b.N; i++ {
		d.Exp()
	}
}

func BenchmarkExpOne(b *testing.B) {
	d := One
	for i := 0; i < b.N; i++ {
		d.Exp()
	}
}

func BenchmarkExpSeven(b *testing.B) {
	d := FromInt(7)
	for i := 0; i < b.N; i++ {
		d.Exp()
	}
}

func BenchmarkExpSmall(b *testing.B) {
	d := Parse("0.01")
	for i := 0; i < b.N; i++ {
		d.Exp()
	}
}
