package decimal

import "testing"

func TestParseLongDecimal(t *testing.T) {
	// This input has 16 fractional digits, more than the 9 decimal places supported.
	// The parser should truncate to 9 places: 2806.308219720
	input := "2806.3082197200404453"
	d := Parse(input)

	// Internal representation: 2806.308219720 * 1e9 = 2806308219720
	want := int64(2806_308219720)
	if int64(d) != want {
		t.Errorf("Parse(%q) = %d, want %d", input, d, want)
	}

	wantStr := "2806.30821972"
	if got := d.String(); got != wantStr {
		t.Errorf("Parse(%q).String() = %q, want %q", input, got, wantStr)
	}
}

func TestParseWithExponent(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantStr string
	}{
		// Small negative exponents
		{"3.14e-4", 314000, "0.000314"},
		{"3.6372083e-07", 363, "0.000000363"},
		{"1e-9", 1, "0.000000001"},
		{"1.5e-9", 1, "0.000000001"}, // truncates to 1
		{"5e-10", 0, "0"},            // below precision

		// Zero exponent
		{"1e0", 1_000_000_000, "1"},
		{"1.5e0", 1_500_000_000, "1.5"},
		{"123.456e0", 123_456_000_000, "123.456"},

		// Positive exponents
		{"1e1", 10_000_000_000, "10"},
		{"1e2", 100_000_000_000, "100"},
		{"1.5e2", 150_000_000_000, "150"},
		{"3.14e3", 3_140_000_000_000, "3140"},

		// With explicit + sign
		{"1e+2", 100_000_000_000, "100"},
		{"2.5e+1", 25_000_000_000, "25"},

		// Uppercase E
		{"1E-4", 100_000, "0.0001"},
		{"1E+3", 1_000_000_000_000, "1000"},

		// Negative mantissa with exponent
		{"-3.14e-4", -314000, "-0.000314"},
		{"-1e2", -100_000_000_000, "-100"},
		{"-2.5e+1", -25_000_000_000, "-25"},

		// Large exponents (triggers shift >= len(pow10) branch)
		{"1e8", 100_000_000_000_000_000, "100000000"},
		{"1e-15", 0, "0"}, // below precision
		{"1e-19", 0, "0"}, // triggers large negative exp branch (exp >= 19)
		{"1e-25", 0, "0"}, // triggers multiple iterations of large exp branch
		{"0e19", 0, "0"},  // triggers large positive exp branch (exp >= 19)
		{"0e100", 0, "0"}, // triggers multiple iterations of large positive exp branch
		// TODO: overflow checking
	}

	for _, tt := range tests {
		d := Parse(tt.input)
		if int64(d) != tt.want {
			t.Errorf("Parse(%q) = %d, want %d", tt.input, d, tt.want)
		}
		if got := d.String(); got != tt.wantStr {
			t.Errorf("Parse(%q).String() = %q, want %q", tt.input, got, tt.wantStr)
		}
	}
}
