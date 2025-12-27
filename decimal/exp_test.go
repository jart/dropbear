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
	for b.Loop() {
		_ = d.Exp()
	}
}

func BenchmarkExpNegative(b *testing.B) {
	d := Parse("-0.5")
	for b.Loop() {
		_ = d.Exp()
	}
}

func BenchmarkExpOne(b *testing.B) {
	d := One
	for b.Loop() {
		_ = d.Exp()
	}
}

func BenchmarkExpSeven(b *testing.B) {
	d := FromInt(7)
	for b.Loop() {
		_ = d.Exp()
	}
}

func BenchmarkExpSmall(b *testing.B) {
	d := Parse("0.01")
	for b.Loop() {
		_ = d.Exp()
	}
}

func TestExpMax(t *testing.T) {
	// 22.945006538 is the largest 9-decimal-place input that doesn't overflow
	// 22.945006539 would overflow (e^x > MaxInt64/Scale)
	d := Parse("22.945006538")
	got := d.Exp().String()
	want := "9223372033.80975104"
	if got != want {
		t.Errorf("exp(22.945006538) = %s, want %s", got, want)
	}
}

func TestExpMin(t *testing.T) {
	d := Parse("-21.416413017")
	got := d.Exp().String()
	want := "0.000000001"
	if got != want {
		t.Errorf("exp(-21.416413017) = %s, want %s", got, want)
	}
}

func TestExpUnderflow(t *testing.T) {
	d := Parse("-21.416413018")
	got := d.Exp().String()
	want := "0"
	if got != want {
		t.Errorf("exp(22.945006538) = %s, want %s", got, want)
	}
}
