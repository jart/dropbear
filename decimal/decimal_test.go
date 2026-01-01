package decimal

import (
	"math/rand"
	"testing"
)

var randomNumbers []Decimal

func init() {
	rng := rand.New(rand.NewSource(42))
	randomNumbers = make([]Decimal, 32)
	for i := range 32 {
		// Use realistic trading values (prices up to $100k, quantities up to 1000)
		// to avoid overflow in multiplication benchmarks
		randomNumbers[i] = FromFloat64(rng.Float64() * 100000)
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"0", 0},
		{"1", 100_000_000},
		{"1.23", 123_000_000},
		{"123.45", 12_345_000_000},
		{"-123.45", -12_345_000_000},
		{"0.01", 1_000_000},
		{"0.1", 10_000_000},
		{".5", 50_000_000},
		{"0.000000001", 0},
		{"1.123456789", 1_123_456_79},
		{"1.23456789123456789", 1_234_567_89},
	}
	for _, tt := range tests {
		d := Parse(tt.input)
		if int64(d) != tt.want {
			t.Errorf("Parse(%q) = %d, want %d", tt.input, d, tt.want)
		}
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		value int64
		want  string
	}{
		{0, "0"},
		{1_000_000_000, "10"},
		{1_230_000_000, "12.3"},
		{123_450_000_000, "1234.5"},
		{-123_450_000_000, "-1234.5"},
		{10_000_000, "0.1"},
		{100_000_000, "1"},
		{1, "0.00000001"},
		{1_123_456_789, "11.23456789"},
	}
	for _, tt := range tests {
		d := Decimal(tt.value)
		if got := d.String(); got != tt.want {
			t.Errorf("Decimal(%d).String() = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"0", "0"},
		{"1", "1"},
		{"1.23", "1.23"},
		{"123.45", "123.45"},
		{"-123.45", "-123.45"},
		{"0.01", "0.01"},
		{"0.10", "0.1"},
	}
	for _, tt := range tests {
		d := Parse(tt.input)
		if got := d.String(); got != tt.want {
			t.Errorf("roundtrip %q -> %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAdd(t *testing.T) {
	a := Parse("1.25")
	b := Parse("2.50")
	got := a.Add(b).String()
	if got != "3.75" {
		t.Errorf("1.25 + 2.50 = %s, want 3.75", got)
	}
}

func TestSub(t *testing.T) {
	a := Parse("5.00")
	b := Parse("3.25")
	got := a.Sub(b).String()
	if got != "1.75" {
		t.Errorf("5.00 - 3.25 = %s, want 1.75", got)
	}
}

func TestMul(t *testing.T) {
	tests := []struct {
		a, b, want string
	}{
		{"2.50", "4.00", "10"},
		{"0.001", "0.9985", "0.0009985"},    // small numbers that caused divide-by-zero
		{"0.0001", "0.9985", "0.00009985"},  // even smaller
		{"100", "100", "10000"},             // large numbers
		{"0.000000001", "1", "0"},           // smallest representable is now 0 (truncation)
		{"0.1876", "0.9995", "0.1875062"},   // medium fractions that caused overflow
		{"0.5", "0.5", "0.25"},              // simple fractions
		{"0.999999999", "0.999999999", "1"}, // near-1 fractions (banker's rounds 0.999999999 to 1)
		{"1e9", "8", "8000000000"},          // max representable without overflow
	}
	for _, tt := range tests {
		a := Parse(tt.a)
		b := Parse(tt.b)
		got := a.Mul(b).String()
		if got != tt.want {
			t.Errorf("%s * %s = %s, want %s", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestMulInt(t *testing.T) {
	a := Parse("1.50")
	got := a.MulInt(3).String()
	if got != "4.5" {
		t.Errorf("1.50 * 3 = %s, want 4.5", got)
	}
}

func TestNeg(t *testing.T) {
	a := Parse("5.00")
	if got := a.Neg().String(); got != "-5" {
		t.Errorf("neg(5.00) = %s, want -5", got)
	}
	b := Parse("-3.00")
	if got := b.Neg().String(); got != "3" {
		t.Errorf("neg(-3.00) = %s, want 3", got)
	}
}

func TestAbs(t *testing.T) {
	a := Parse("-5.00")
	if got := a.Abs().String(); got != "5" {
		t.Errorf("abs(-5.00) = %s, want 5", got)
	}
	b := Parse("3.00")
	if got := b.Abs().String(); got != "3" {
		t.Errorf("abs(3.00) = %s, want 3", got)
	}
}

func TestComparisons(t *testing.T) {
	a := Parse("1.00")
	b := Parse("2.00")
	if a.Cmp(b) >= 0 {
		t.Errorf("1.00 should be < 2.00")
	}
	if b.Cmp(a) <= 0 {
		t.Errorf("2.00 should be > 1.00")
	}
	if a.Cmp(b) > 0 {
		t.Errorf("1.00 should be <= 2.00")
	}
	if a.Cmp(a) > 0 {
		t.Errorf("1.00 should be <= 1.00")
	}
	if b.Cmp(a) < 0 {
		t.Errorf("2.00 should be >= 1.00")
	}
	if a.Cmp(a) < 0 {
		t.Errorf("1.00 should be >= 1.00")
	}
	if a.Cmp(a) != 0 {
		t.Errorf("1.00 should == 1.00")
	}
}

func TestIsZero(t *testing.T) {
	if !Decimal(0).IsZero() {
		t.Errorf("0 should be zero")
	}
	if Parse("1.00").IsZero() {
		t.Errorf("1.00 should not be zero")
	}
}

func TestIsPositiveNegative(t *testing.T) {
	pos := Parse("1.00")
	neg := Parse("-1.00")
	zero := Decimal(0)
	if !pos.IsPositive() {
		t.Errorf("1.00 should be positive")
	}
	if pos.IsNegative() {
		t.Errorf("1.00 should not be negative")
	}
	if !neg.IsNegative() {
		t.Errorf("-1.00 should be negative")
	}
	if neg.IsPositive() {
		t.Errorf("-1.00 should not be positive")
	}
	if zero.IsPositive() || zero.IsNegative() {
		t.Errorf("0 should be neither positive nor negative")
	}
}

func TestFloat64(t *testing.T) {
	d := Parse("1.50")
	if got := d.Float64(); got != 1.5 {
		t.Errorf("Float64() = %f, want 1.5", got)
	}
}

func TestQuantizeTruncate(t *testing.T) {
	q := Parse("0.01")
	tests := []struct {
		input string
		want  string
	}{
		{"1.567", "1.56"},
		{"1.561", "1.56"},
		{"1.00", "1"},
		{"-1.567", "-1.56"},
		{"-1.561", "-1.56"},
	}
	for _, tt := range tests {
		d := Parse(tt.input)
		got := d.QuantizeTruncate(q).String()
		if got != tt.want {
			t.Errorf("QuantizeTruncate(%s) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestQuantizeFloor(t *testing.T) {
	q := Parse("0.01")
	tests := []struct {
		input string
		want  string
	}{
		{"1.567", "1.56"},
		{"1.561", "1.56"},
		{"1.00", "1"},
		{"-1.567", "-1.57"},
		{"-1.561", "-1.57"},
		{"-1.00", "-1"},
	}
	for _, tt := range tests {
		d := Parse(tt.input)
		got := d.QuantizeFloor(q).String()
		if got != tt.want {
			t.Errorf("QuantizeFloor(%s) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestQuantizeCeil(t *testing.T) {
	q := Parse("0.01")
	tests := []struct {
		input string
		want  string
	}{
		{"1.567", "1.57"},
		{"1.561", "1.57"},
		{"1.00", "1"},
		{"-1.567", "-1.56"},
		{"-1.561", "-1.56"},
		{"-1.00", "-1"},
	}
	for _, tt := range tests {
		d := Parse(tt.input)
		got := d.QuantizeCeil(q).String()
		if got != tt.want {
			t.Errorf("QuantizeCeil(%s) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestQuantizeAway(t *testing.T) {
	q := Parse("0.01")
	tests := []struct {
		input string
		want  string
	}{
		{"1.567", "1.57"},
		{"1.561", "1.57"},
		{"1.00", "1"},
		{"-1.567", "-1.57"},
		{"-1.561", "-1.57"},
		{"-1.00", "-1"},
	}
	for _, tt := range tests {
		d := Parse(tt.input)
		got := d.QuantizeAway(q).String()
		if got != tt.want {
			t.Errorf("QuantizeAway(%s) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestFromFloat64(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{1.5, "1.5"},
		{-1.5, "-1.5"},
		{123.456, "123.456"},
		{0.00000001, "0.00000001"}, // 1 satoshi
	}
	for _, tt := range tests {
		d := FromFloat64(tt.input)
		if got := d.String(); got != tt.want {
			t.Errorf("fromFloat64(%v) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestFromInt(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{-1, "-1"},
		{123, "123"},
		{-456, "-456"},
	}
	for _, tt := range tests {
		d := FromInt(tt.input)
		if got := d.String(); got != tt.want {
			t.Errorf("FromInt(%v) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestPrecision(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "1"},
		{1, "0.1"},
		{8, "0.00000001"},
		{9, "0.00000001"},
		{10, "0.00000001"},
		{11, "0.00000001"},
	}
	for _, tt := range tests {
		d := Precision(tt.input)
		if got := d.String(); got != tt.want {
			t.Errorf("Precision(%v) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func BenchmarkParse(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		Parse("12345.678901234")
	}
}

func BenchmarkString(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		a := randomNumbers[i&31]
		_ = a.String()
	}
}

func BenchmarkAdd(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		a := randomNumbers[(i+0)&31]
		c := randomNumbers[(i+1)&31]
		_ = a.Add(c)
	}
}

func BenchmarkMul(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		a := randomNumbers[(i+0)&31]
		c := randomNumbers[(i+1)&31]
		_ = a.Mul(c)
	}
}

func BenchmarkMulInt(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		a := randomNumbers[(i+0)&31]
		_ = a.MulInt(2)
	}
}

func BenchmarkDiv(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		a := randomNumbers[(i+0)&31]
		c := randomNumbers[(i+1)&31]
		_ = a.Div(c)
	}
}

func BenchmarkDivInt(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		a := randomNumbers[(i+0)&31]
		_ = a.DivInt(2)
	}
}
