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
	// 29.853102434 is approx ln(MaxInt64/Scale) for Scale=1e6
	d := Parse("29.853102434")
	got := d.Exp().String()
	want := "9223372036854.775807" // close to MaxInt64
	if got != want {
		t.Errorf("exp(29.853102434) = %s, want %s", got, want)
	}
}

func TestExpMin(t *testing.T) {
	d := Parse("-21.416413017")
	got := d.Exp().String()
	want := "0"
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
