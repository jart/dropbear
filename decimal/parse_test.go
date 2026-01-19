package decimal

import "testing"

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
