package decimal

import (
	"math"
	"testing"
)

func TestSqrt(t *testing.T) {
	tests := []float64{
		0,
		1,
		2,
		4,
		9,
		16,
		0.5,
		0.25,
		0.01,
		0.0001,
		100,
		10000,
		1.5,
		2.25,
	}
	for _, x := range tests {
		d := Parse(formatFloat(x))
		got := d.Sqrt().Float64()
		want := math.Sqrt(x)
		diff := math.Abs(got - want)
		if diff > 0.000001 {
			t.Errorf("Sqrt(%v): got %v, want %v (diff %v)", x, got, want, diff)
		}
	}
}

func TestSqrtNegative(t *testing.T) {
	d := Parse("-1")
	got := d.Sqrt()
	if got != 0 {
		t.Errorf("Sqrt(-1): got %v, want 0", got)
	}
}

func BenchmarkSqrt(b *testing.B) {
	d := Parse("2")
	for i := 0; i < b.N; i++ {
		d.Sqrt()
	}
}

func BenchmarkSqrtSmall(b *testing.B) {
	d := Parse("0.0001")
	for i := 0; i < b.N; i++ {
		d.Sqrt()
	}
}

func BenchmarkSqrtLarge(b *testing.B) {
	d := FromInt(10000)
	for i := 0; i < b.N; i++ {
		d.Sqrt()
	}
}
