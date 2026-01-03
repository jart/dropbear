package decimal

import (
	"fmt"
	"math"
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

// TestParseDegenerate tests edge cases in Parse that might cause issues.
func TestParseDegenerate(t *testing.T) {
	// These should all panic
	badInputs := []string{
		"",      // empty
		"+",     // just sign
		"-",     // just sign
		".",     // just decimal point
		"+.",    // sign and decimal point
		"-.",    // sign and decimal point
		"e5",    // exponent only
		"E5",    // exponent only uppercase
		"+e5",   // sign and exponent
		".e5",   // decimal and exponent only
		"abc",   // letters
		"1.2.3", // multiple decimal points
		"--1",   // double negative
		"++1",   // double positive
		"1 2",   // space in number
		" 1",    // leading space
		"1 ",    // trailing space
		"1a",    // trailing letter
		"1.2a",  // trailing letter after decimal
		"∞",     // unicode infinity
		"NaN",   // not a number
	}

	for _, input := range badInputs {
		t.Run(input, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Parse(%q) did not panic", input)
				}
			}()
			_ = Parse(input)
		})
	}
}

// TestParseEmptyExponent documents that empty exponents are treated as e0.
// This is intentional: "1e" parses as 1, "1e+" parses as 1, "1e-" parses as 1.
// Rationale: checking for exponent digits would add branches to the hot path
// for minimal benefit. If you write "1e" you get what you deserve.
func TestParseEmptyExponent(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"1e", "1"},
		{"1E", "1"},
		{"1e+", "1"},
		{"1e-", "1"},
		{"1.5e", "1.5"},
		{"123e", "123"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			result := Parse(tc.input)
			want := Parse(tc.want)
			if result != want {
				t.Errorf("Parse(%q) = %s, want %s", tc.input, result, want)
			}
		})
	}
}

// TestQuantizeZero tests what happens when quantizing with zero increment.
func TestQuantizeZero(t *testing.T) {
	d := Parse("1.234")

	methods := []struct {
		name string
		fn   func() Decimal
	}{
		{"QuantizeTruncate", func() Decimal { return d.QuantizeTruncate(Zero) }},
		{"QuantizeAway", func() Decimal { return d.QuantizeAway(Zero) }},
		{"QuantizeFloor", func() Decimal { return d.QuantizeFloor(Zero) }},
		{"QuantizeCeil", func() Decimal { return d.QuantizeCeil(Zero) }},
		{"QuantizeNearest", func() Decimal { return d.QuantizeNearest(Zero) }},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("%s(Zero) did not panic", m.name)
				}
			}()
			_ = m.fn()
		})
	}
}

// TestQuantizeNegative tests that negative quantum panics.
func TestQuantizeNegative(t *testing.T) {
	d := Parse("1.234")
	q := Parse("-0.01")

	methods := []struct {
		name string
		fn   func() Decimal
	}{
		{"QuantizeTruncate", func() Decimal { return d.QuantizeTruncate(q) }},
		{"QuantizeAway", func() Decimal { return d.QuantizeAway(q) }},
		{"QuantizeFloor", func() Decimal { return d.QuantizeFloor(q) }},
		{"QuantizeCeil", func() Decimal { return d.QuantizeCeil(q) }},
		{"QuantizeNearest", func() Decimal { return d.QuantizeNearest(q) }},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("%s with negative quantum did not panic", m.name)
				}
			}()
			_ = m.fn()
		})
	}
}

// TestQuantizeVerySmalllIncrement tests quantizing with increments smaller than our precision.
func TestQuantizeVerySmallIncrement(t *testing.T) {
	d := Parse("1.234")

	// 1 is the smallest possible non-zero Decimal (0.000000001)
	q := Decimal(1)
	result := d.QuantizeTruncate(q)
	if result != d {
		t.Errorf("Quantize to smallest unit changed value: %s -> %s", d, result)
	}
}

// TestQuantizeLargerThanValue tests quantizing when increment is larger than value.
func TestQuantizeLargerThanValue(t *testing.T) {
	d := Parse("0.5")
	q := One // 1.0

	t.Run("QuantizeTruncate", func(t *testing.T) {
		result := d.QuantizeTruncate(q)
		if result != Zero {
			t.Errorf("0.5.QuantizeTruncate(1) = %s, want 0", result)
		}
	})

	t.Run("QuantizeAway", func(t *testing.T) {
		result := d.QuantizeAway(q)
		if result != One {
			t.Errorf("0.5.QuantizeAway(1) = %s, want 1", result)
		}
	})

	t.Run("QuantizeNearest", func(t *testing.T) {
		result := d.QuantizeNearest(q)
		// 0.5 should round to 1 (half away from zero)
		if result != One {
			t.Errorf("0.5.QuantizeNearest(1) = %s, want 1", result)
		}
	})
}

// TestQuantizeNearestTieBreaking tests rounding behavior at exact midpoints.
func TestQuantizeNearestTieBreaking(t *testing.T) {
	q := Parse("0.1")

	tests := []struct {
		value string
		want  string
	}{
		{"0.05", "0.1"},   // positive half away from zero -> up
		{"-0.05", "-0.1"}, // negative half away from zero -> down
		{"0.15", "0.2"},
		{"-0.15", "-0.2"},
		{"0.25", "0.3"},
		{"-0.25", "-0.3"},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			d := Parse(tt.value)
			result := d.QuantizeNearest(q)
			want := Parse(tt.want)
			if result != want {
				t.Errorf("%s.QuantizeNearest(0.1) = %s, want %s", tt.value, result, tt.want)
			}
		})
	}
}

// TestDivIntByZero tests DivInt with zero divisor.
func TestDivIntByZero(t *testing.T) {
	d := One
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("DivInt(0) did not panic")
		}
	}()
	_ = d.DivInt(0)
}

// TestMulIntOverflowMoreCases tests additional multiplication overflow cases.
func TestMulIntOverflowMoreCases(t *testing.T) {
	tests := []struct {
		name string
		d    Decimal
		n    int
	}{
		{"MinInt", Min, 2},
		{"LargePos", Parse("50000000000"), 2},
		{"LargeNeg", Parse("-50000000000"), 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("%s.MulInt(%d) did not panic", tt.d, tt.n)
				}
			}()
			_ = tt.d.MulInt(tt.n)
		})
	}
}

// TestMinNeg tests that Min.Neg() panics (two's complement asymmetry).
func TestMinNeg(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Min.Neg() did not panic")
		}
	}()
	_ = Min.Neg()
}

// TestMinAbs tests that Min.Abs() panics (two's complement asymmetry).
func TestMinAbs(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Min.Abs() did not panic")
		}
	}()
	_ = Min.Abs()
}

// TestFromIntOverflow tests FromInt with values that would overflow.
func TestFromIntOverflow(t *testing.T) {
	tests := []struct {
		name string
		n    int
	}{
		{"MaxInt/Scale+1", math.MaxInt64/Scale + 1},
		{"MinInt/Scale-1", math.MinInt64/Scale - 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("FromInt(%d) did not panic", tt.n)
				}
			}()
			_ = FromInt(tt.n)
		})
	}
}

// TestFromFloat64NaN tests that NaN is properly rejected.
func TestFromFloat64NaN(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("FromFloat64(NaN) did not panic")
		}
	}()
	_ = FromFloat64(math.NaN())
}

// TestFromFloat64Inf tests that infinity is properly rejected.
func TestFromFloat64Inf(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("FromFloat64(+Inf) did not panic")
			}
		}()
		_ = FromFloat64(math.Inf(1))
	})
	t.Run("negative", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("FromFloat64(-Inf) did not panic")
			}
		}()
		_ = FromFloat64(math.Inf(-1))
	})
}

// TestFromFloat64RejectLargeValues tests that FromFloat64 rejects values
// where float64 lacks sufficient precision for our 8 decimal places.
// The limit is 2^53 / Scale ≈ 90 million.
func TestFromFloat64RejectLargeValues(t *testing.T) {
	largeValues := []float64{
		90072000, // just over maxSafeFloat (90,071,992)
		1e8,      // 100 million
		1e9,      // 1 billion
		9e9,      // 9 billion
		-90072000,
		-1e9,
	}

	for _, n := range largeValues {
		t.Run(fmt.Sprintf("%e", n), func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("FromFloat64(%e) did not panic", n)
				}
			}()
			_ = FromFloat64(n)
		})
	}
}

// TestFromFloat64AcceptSafeValues tests that FromFloat64 accepts values
// within the precision-safe range.
func TestFromFloat64AcceptSafeValues(t *testing.T) {
	safeValues := []float64{
		0,
		1,
		-1,
		1000000,  // 1 million
		90000000, // 90 million (just under limit)
		-90000000,
		0.123456789,
		1234.56789,
	}

	for _, n := range safeValues {
		t.Run(fmt.Sprintf("%v", n), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("FromFloat64(%v) panicked unexpectedly: %v", n, r)
				}
			}()
			_ = FromFloat64(n)
		})
	}
}

// TestFormatOutOfRange tests that Format panics for n outside [0, 8].
func TestFormatOutOfRange(t *testing.T) {
	d := Parse("1.23")

	badValues := []int{-1, 9, 10, 100}
	for _, n := range badValues {
		t.Run(fmt.Sprintf("Format(%d)", n), func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Format(%d) did not panic", n)
				}
			}()
			_ = d.Format(n)
		})
		t.Run(fmt.Sprintf("FormatThousand(%d)", n), func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("FormatThousand(%d) did not panic", n)
				}
			}()
			_ = d.FormatThousand(n)
		})
	}
}

// TestFromFloat64SafeRange tests that values within float64's exact integer
// range convert correctly.
func TestFromFloat64SafeRange(t *testing.T) {
	// Small values should convert exactly
	tests := []struct {
		f    float64
		want string
	}{
		{0.0, "0"},
		{1.0, "1"},
		{-1.0, "-1"},
		{0.123456789, "0.12345679"}, // Rounded to 8 places
		{1000.0, "1000"},
		{1000000.123456789, "1000000.12345679"}, // Rounded
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			d := FromFloat64(tt.f)
			want := Parse(tt.want)
			if d != want {
				t.Errorf("FromFloat64(%v) = %s, want %s", tt.f, d, want)
			}
		})
	}
}

// TestParseExtremeExponents tests Parse with very large exponents.
// BUG: Parse does not check for overflow during exponent multiplication.
// 1e10 is approximately the max safe value (1e10 * Scale = 1e18 ≈ MaxInt64).
func TestParseExtremeExponents(t *testing.T) {
	// These should overflow but currently produce garbage instead
	overflowCases := []string{
		"1e11", // 1e11 * Scale overflows
		"1e20",
		"1e50",
		"1e100",
	}

	for _, input := range overflowCases {
		t.Run(input, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Parse(%q) did not panic (should detect overflow)", input)
				}
			}()
			_ = Parse(input)
		})
	}

	// This should just become zero (underflow to zero is OK)
	t.Run("HugeNegativeExponent", func(t *testing.T) {
		result := Parse("1e-100")
		if result != Zero {
			t.Errorf("Parse(\"1e-100\") = %s, want 0", result)
		}
	})

	// Verify boundary: 1e10 should work (largest exponent that fits)
	t.Run("1e10_is_valid", func(t *testing.T) {
		result := Parse("1e10")
		want := Parse("10000000000")
		if result != want {
			t.Errorf("Parse(\"1e10\") = %s, want %s", result, want)
		}
	})

	// And 9e9 should also work (near max value)
	t.Run("9e9_is_valid", func(t *testing.T) {
		result := Parse("9e9")
		want := Parse("9000000000")
		if result != want {
			t.Errorf("Parse(\"9e9\") = %s, want %s", result, want)
		}
	})
}

// TestParseOverflowInMantissa tests overflow detection during mantissa parsing.
func TestParseOverflowInMantissa(t *testing.T) {
	// The max integer part is about 92 billion
	// Values larger should panic

	tests := []struct {
		input     string
		wantPanic bool
	}{
		{"9223372036.854775807", false}, // Max value with 9 places (truncated to 8) - safe
		{"92233720368.54775807", false}, // Max value with 8 places - safe
		{"9223372037", false},           // Now safe (9.2B)
		{"10000000000", false},          // Now safe (10B)
		{"92233720369", true},           // Just over max integer part
		{"99999999999", true},           // 100 billion
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			defer func() {
				r := recover()
				if tt.wantPanic && r == nil {
					t.Errorf("Parse(%q) did not panic", tt.input)
				}
				if !tt.wantPanic && r != nil {
					t.Errorf("Parse(%q) panicked unexpectedly: %v", tt.input, r)
				}
			}()
			_ = Parse(tt.input)
		})
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
