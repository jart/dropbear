package decimal

import (
	"math"
	"math/big"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"0", 0},
		{"1", 1_000_000},
		{"1.23", 1_230_000},
		{"123.45", 123_450_000},
		{"-123.45", -123_450_000},
		{"0.01", 10_000},
		{"0.1", 100_000},
		{".5", 500_000},
		{"0.0000001", 0},
		{"1.123456789", 1_123_457},
		{"1.23456789123456789", 1_234_568},
	}
	for _, tt := range tests {
		d := Parse(tt.input)
		if int64(d) != tt.want {
			t.Errorf("Parse(%q) = %d, want %d", tt.input, d, tt.want)
		}
	}
}

func TestParseWithExponent(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantStr string
	}{
		// cool syntax
		{"0", 0, "0"},
		{"-0", 0, "0"},
		{"1.", 1_000_000, "1"},
		{".314", 314_000, "0.314"},

		// too many puppies
		{".123456789", 123_457, "0.123457"},
		{".00000025", 0, "0"},
		{".00000025000001", 0, "0"},
		{".00000035", 0, "0"},
		{".00000045", 0, "0"},
		{".000000459", 0, "0"},
		{".000000444", 0, "0"},
		{".000000448", 0, "0"},

		// small negative exponents
		{"3.14e-4", 314, "0.000314"},
		{"3.6372083e-07", 0, "0"}, // truncated
		{"1e-9", 0, "0"},          // truncated
		{"1.5e-9", 0, "0"},        // truncated
		{"5e-10", 0, "0"},         // below precision

		// zero exponent
		{"1e0", 1_000_000, "1"},
		{"1.5e0", 1_500_000, "1.5"},
		{"123.456e0", 123_456_000, "123.456"},

		// positive exponents
		{"1e1", 10_000_000, "10"},
		{"1e2", 100_000_000, "100"},
		{"1.5e2", 150_000_000, "150"},
		{"3.14e3", 3_140_000_000, "3140"},

		// with explicit + sign
		{"+1e+2", 100_000_000, "100"},
		{"1e+2", 100_000_000, "100"},
		{"2.5e+1", 25_000_000, "25"},

		// uppercase E
		{"1E-4", 100, "0.0001"},
		{"1E+3", 1_000_000_000, "1000"},

		// negative mantissa with exponent
		{"-3.14e-4", -314, "-0.000314"},
		{"-1e2", -100_000_000, "-100"},
		{"-2.5e+1", -25_000_000, "-25"},

		// large exponents
		{"1e8", 100_000_000_000_000, "100000000"},
		{"1e-15", 0, "0"}, // below precision
		{"1e-19", 0, "0"}, // triggers large negative exp branch
		{"1e-25", 0, "0"}, // triggers multiple iterations of large exp branch
		{"0e19", 0, "0"},  // triggers large positive exp branch
		{"0e100", 0, "0"}, // triggers multiple iterations of large positive exp branch

		// thousands separators (banker's rounds .854775807 → .854776)
		{"9,223,372,036.854775807", 9_223_372_036_854_776, "9223372036.854776"},
		{"9'223'372'036.854'775'807", 9_223_372_036_854_776, "9223372036.854776"},
		{"9_223_372_036.854_775_807", 9_223_372_036_854_776, "9223372036.854776"},

		// too many decimal places
		{"3.1415926535", 3_141_593, "3.141593"}, // truncated

		// panic cases
		{"", 0, ""},
		{"_", 0, ""},
		{"1e2.5", 0, ""},
		// "9223372037", 0, ""}, // Now safe (9.2e17)
		{"92233720373483838", 0, ""},
	}

	for _, tt := range tests {
		if tt.wantStr == "" {
			t.Run(tt.input, func(t *testing.T) {
				defer func() {
					if recover() == nil {
						t.Errorf("Parse(%q) should have panicked", tt.input)
					}
				}()
				Parse(tt.input)
			})
		} else {
			d := Parse(tt.input)
			if int64(d) != tt.want {
				t.Errorf("Parse(%q) = %d, want %d", tt.input, d, tt.want)
			}
			if got := d.String(); got != tt.wantStr {
				t.Errorf("Parse(%q).String() = %q, want %q", tt.input, got, tt.wantStr)
			}
		}
	}
}

func TestParseLongDecimal(t *testing.T) {
	// This input has 16 fractional digits, more than the 6 decimal places supported.
	// The parser should round to 6 places: 2806.308220
	input := "2806.3082197200404453"
	d := Parse(input)
	// Internal representation: 2806.308220 * 1e6 = 2806308220
	want := int64(2_806_308_220)
	if int64(d) != want {
		t.Errorf("Parse(%q) = %d, want %d", input, d, want)
	}
	wantStr := "2806.30822"
	if got := d.String(); got != wantStr {
		t.Errorf("Parse(%q).String() = %q, want %q", input, got, wantStr)
	}
}

func TestParseNegativeExponentRounding(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantStr string
	}{
		// Positive mantissa with negative exponent triggering rem > half
		{"1.6e-6", 2, "0.000002"}, // 0.0000016 → rounds up to 0.000002
		{"6e-7", 1, "0.000001"},   // 0.0000006 → rounds up to 0.000001
		{"5.1e-6", 5, "0.000005"}, // 0.0000051 → truncates to 0.000005

		// Positive mantissa with rem == half && x%2 != 0 (banker's rounds odd up)
		{"1.5e-6", 2, "0.000002"}, // 0.0000015 with x=1 (odd) → rounds to 2
		{"3.5e-6", 4, "0.000004"}, // 0.0000035 with x=3 (odd) → rounds to 4

		// Negative mantissa with negative exponent (covers rem < 0 branch)
		{"-1.6e-6", -2, "-0.000002"}, // -0.0000016 → rounds to -0.000002
		{"-6e-7", -1, "-0.000001"},   // -0.0000006 → rounds to -0.000001
		{"-1.5e-6", -2, "-0.000002"}, // -0.0000015 with x=-1 (odd) → rounds to -2
		{"-3.5e-6", -4, "-0.000004"}, // -0.0000035 with x=-3 (odd) → rounds to -4
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d := Parse(tt.input)
			if int64(d) != tt.want {
				t.Errorf("Parse(%q) = %d, want %d", tt.input, d, tt.want)
			}
			if got := d.String(); got != tt.wantStr {
				t.Errorf("Parse(%q).String() = %q, want %q", tt.input, got, tt.wantStr)
			}
		})
	}
}

func TestMayParseErrors(t *testing.T) {
	tests := []struct {
		input   string
		wantErr error
	}{
		{"", ErrEmptyNumber},
		{"_", ErrMissingNumber},
		{"1e2.5", ErrBrokenNumber},  // trailing .5 after exponent
		{"123abc", ErrBrokenNumber}, // trailing garbage
		{"1.5xyz", ErrBrokenNumber}, // trailing garbage after decimal
		{"1e5abc", ErrBrokenNumber}, // trailing garbage after exponent
		{"92233720373483838", ErrIllegalNumber},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := ParseString(tt.input)
			if err != tt.wantErr {
				t.Errorf("ParseString(%q) error = %v, want %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func BenchmarkParse(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		Parse("12345.678901234")
	}
}

func BenchmarkParseInteger(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		Parse("123456789")
	}
}

func BenchmarkParseLongDecimal(b *testing.B) {
	input := "2806.3082197200404453"
	for i := 0; b.Loop(); i++ {
		_ = Parse(input)
	}
}

func FuzzParse(f *testing.F) {
	// Seed with some basic cases
	f.Add("0")
	f.Add("1")
	f.Add("-1")
	f.Add("1.234")
	f.Add("1.23456789")
	f.Add("1.5e-1")
	f.Add("100e5")
	f.Add("1.123_456,789'0")
	f.Add("9'223'372'036.854'775'807")
	f.Add("-9'223'372'036'854.775807")
	f.Add("-9'223'372'036'854.775808")

	f.Fuzz(func(t *testing.T, s string) {
		// 1. Try parsing with decimal.ParseBytes
		got, err := ParseString(s)
		if err != nil {
			// If our parser fails, check if it's a "reasonable" error
			// for obviously non-numeric input.
			return
		}

		// 2. Parse with math/big.Rat as the Oracle
		// We need to clean the string because big.Rat doesn't like _ , '
		cleanS := s
		cleanS = strings.ReplaceAll(cleanS, "_", "")
		cleanS = strings.ReplaceAll(cleanS, ",", "")
		cleanS = strings.ReplaceAll(cleanS, "'", "")

		rat := new(big.Rat)
		_, ok := rat.SetString(cleanS)
		if !ok {
			// If big.Rat can't parse it, but we did, we might have a bug
			// or we are just more permissive (like supporting empty exponents).
			return
		}

		// 3. Convert big.Rat to the expected Decimal (int64 with 6 places)
		// Calculation: RoundToEven(rat * 1,000,000)
		multiplier := new(big.Rat).SetInt64(Scale)
		rat.Mul(rat, multiplier)

		wantInt := roundToEven(rat)
		if got != Decimal(wantInt) {
			t.Errorf("For input %q:\n  got:  %v (%d)\n  want: %v (%d)", s, got, got, Decimal(wantInt), wantInt)
		}
	})
}

// roundToEven performs Banker's Rounding on a big.Rat to the nearest integer.
func roundToEven(r *big.Rat) int64 {
	num := r.Num()
	den := r.Denom()

	// Integer division: q = num / den, rem = num % den
	q := new(big.Int).Quo(num, den)
	rem := new(big.Int).Rem(num, den)

	// We need absolute values for the midpoint check
	absRem := new(big.Int).Abs(rem)
	absDen := new(big.Int).Abs(den)

	// Half = den / 2
	half := new(big.Int).Div(absDen, big.NewInt(2))
	isExactlyHalf := new(big.Int).Mul(absRem, big.NewInt(2)).Cmp(absDen) == 0

	// Banker's Rounding Logic:
	// 1. If rem > half: round away from zero
	// 2. If rem < half: truncate (already done by Quo)
	// 3. If rem == half: round to even
	if absRem.Cmp(half) > 0 || (isExactlyHalf && q.Bit(0) != 0) {
		if num.Sign() >= 0 {
			q.Add(q, big.NewInt(1))
		} else {
			q.Sub(q, big.NewInt(1))
		}
	}

	if !q.IsInt64() {
		// This happens on overflow, which the fuzzer should ignore
		// or we can catch separately.
		return 0
	}

	return q.Int64()
}

// Probe values, error conditions, and parse paths near the int64 boundary
// and around banker's rounding.
func TestParseBoundaries(t *testing.T) {
	tests := []struct {
		input string
		want  int64 // ignored if wantErr != nil
		err   error // nil = success
	}{
		// MinInt64 reachable through several syntactic paths
		{"-9223372036854.775808", math.MinInt64, nil},
		{"-9.223372036854775808e12", math.MinInt64, nil},
		{"-92233720368547.75808e-1", math.MinInt64, nil},
		{"-9223372036854775808e-6", math.MinInt64, nil},
		{"-9223372036854775.808e-3", math.MinInt64, nil},

		// MaxInt64 reachable through several syntactic paths
		{"9223372036854.775807", math.MaxInt64, nil},
		{"9.223372036854775807e12", math.MaxInt64, nil},
		{"9223372036854775807e-6", math.MaxInt64, nil},

		// One past the boundary -- must NOT silently wrap
		{"9223372036854.775808", 0, ErrToastedNumber}, // MaxInt64+1 positive
		{"-9223372036854.775809", 0, ErrToastedNumber},

		// Banker's rounding that pushes coef past the boundary
		// 9223372036854.7758075 -> coef=...807 (odd) + half -> ...808 = MaxInt64+1
		{"9223372036854.7758075", 0, ErrIllegalNumber},
		// 9223372036854.7758074 -> rem<half, stays at MaxInt64
		{"9223372036854.7758074", math.MaxInt64, nil},
		// -9223372036854.7758085 -> banker's: lost '5' is extraDigit,
		// coef=...808 (even), no round -> MinInt64
		{"-9223372036854.7758085", math.MinInt64, nil},

		// Banker's at zero
		{"0.0000005", 0, nil},                 // half, coef=0 even -> 0
		{"-0.0000005", 0, nil},                // half, coef=0 even -> 0
		{"0.0000015", 2, nil},                 // half, coef=1 odd -> 2
		{"-0.0000015", -2, nil},               // half, coef=-1 odd -> -2
		{"0.00000050000000000000001", 1, nil}, // just over half via tail
		{"-0.00000050000000000000001", -1, nil},
		{"0.00000049999999999999999", 0, nil}, // just under half

		// Extreme exponents at the divisorExp == 19 boundary
		{"5000000000000000000e-25", 0, nil}, // exact half (banker's: 0 even)
		{"5000000000000000001e-25", 1, nil}, // just over
		{"4999999999999999999e-25", 0, nil}, // just under
		{"-5000000000000000001e-25", -1, nil},
		{"9000000000000000000e-25", 1, nil}, // > half clearly

		// Very small via fractional accumulation hitting overflow
		{".0000007000000000000000000", 1, nil},
		{".00000050000000000000000001", 1, nil},
		{".00000049999999999999999999", 0, nil},

		// Zero with extreme exponents -- should never error
		{"0e1000000", 0, nil},
		{"0e-1000000", 0, nil},
		{"0.0e1000", 0, nil},

		// Long trailing zeros at the rounding cusp
		{"0.0000004999999999999999999999999999999", 0, nil},
		{"0.0000005000000000000000000000000000001", 1, nil},
		{"0.0000005000000000000000000000000000000", 0, nil}, // exact half + 0 even

		// Suffix exponent that needs lost digits back -> error
		{"99999999999999999.5e6", 0, ErrIllegalNumber},

		// Plain integer overflow
		{"99999999999999999999", 0, ErrIllegalNumber}, // 20 digits
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseString(tt.input)
			if tt.err != nil {
				if err != tt.err {
					t.Errorf("ParseString(%q): err = %v, want %v", tt.input, err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseString(%q): unexpected error %v", tt.input, err)
			}
			if int64(got) != tt.want {
				t.Errorf("ParseString(%q) = %d, want %d", tt.input, int64(got), tt.want)
			}
		})
	}
}

// Inputs the parser currently accepts leniently. Pinning the behavior
// so we notice if it changes.
func TestParseLenient(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1e", 1_000_000},     // 'e' with no digits -- exponent ignored
		{"1e+", 1_000_000},    // sign but no digits
		{"1e-", 1_000_000},    // sign but no digits
		{"1,", 1_000_000},     // trailing separator
		{"_1", 1_000_000},     // leading separator
		{"1__2", 12_000_000},  // double separator
		{"1.5_e0", 1_500_000}, // separator allowed before 'e'
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseString(tt.input)
			if err != nil {
				t.Errorf("ParseString(%q): unexpected error %v", tt.input, err)
				return
			}
			if int64(got) != tt.want {
				t.Errorf("ParseString(%q) = %d, want %d", tt.input, int64(got), tt.want)
			}
		})
	}
}

// KNOWN LIMITATION: an integer part whose value exceeds the signed 64-bit
// range always errors, even if a negative exponent would bring the final
// value back into range. To support these, we'd need to track
// extraDigit/hasMoreDigits during integer overflow and apply them through
// the shift logic.
func TestParseIntegerOverflowLimitation(t *testing.T) {
	// Mathematically these are well-defined and tiny, but rejected today.
	cases := []string{
		"9999999999999999999e-26",  // ≈ 1e-7, would round to 0
		"99999999999999999999e-13", // 9.99e6, well within range
		"-9999999999999999999e-13",
	}
	for _, in := range cases {
		_, err := ParseString(in)
		if err == nil {
			t.Logf("ParseString(%q) now succeeds -- consider promoting to TestParseBoundaries", in)
		}
	}
}
