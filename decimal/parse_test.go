package decimal

import (
	"log"
	"testing"
)

func TestParseWithExponent(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantStr string
	}{
		// cool syntax
		{"0", 0, "0"},
		{"-0", 0, "0"},
		{"1.", 100_000_000, "1"},
		{".314", 31_400_000, "0.314"},

		// small negative exponents
		{"3.14e-4", 31_400, "0.000314"},
		{"3.6372083e-07", 36, "0.00000036"}, // truncated
		{"1e-9", 0, "0"},                    // truncated
		{"1.5e-9", 0, "0"},                  // truncated
		{"5e-10", 0, "0"},                   // below precision

		// zero exponent
		{"1e0", 100_000_000, "1"},
		{"1.5e0", 150_000_000, "1.5"},
		{"123.456e0", 12_345_600_000, "123.456"},

		// positive exponents
		{"1e1", 1_000_000_000, "10"},
		{"1e2", 10_000_000_000, "100"},
		{"1.5e2", 15_000_000_000, "150"},
		{"3.14e3", 314_000_000_000, "3140"},

		// with explicit + sign
		{"+1e+2", 10_000_000_000, "100"},
		{"1e+2", 10_000_000_000, "100"},
		{"2.5e+1", 2_500_000_000, "25"},

		// uppercase E
		{"1E-4", 10_000, "0.0001"},
		{"1E+3", 100_000_000_000, "1000"},

		// negative mantissa with exponent
		{"-3.14e-4", -31_400, "-0.000314"},
		{"-1e2", -10_000_000_000, "-100"},
		{"-2.5e+1", -2_500_000_000, "-25"},

		// large exponents
		{"1e8", 10_000_000_000_000_000, "100000000"},
		{"1e-15", 0, "0"}, // below precision
		{"1e-19", 0, "0"}, // triggers large negative exp branch
		{"1e-25", 0, "0"}, // triggers multiple iterations of large exp branch
		{"0e19", 0, "0"},  // triggers large positive exp branch
		{"0e100", 0, "0"}, // triggers multiple iterations of large positive exp branch

		// thousands separators
		{"9,223,372,036.854775807", 9_223_372_036_854_775_80, "9223372036.8547758"}, // truncated
		{"9'223'372'036.854'775'807", 9_223_372_036_854_775_80, "9223372036.8547758"},
		{"9_223_372_036.854_775_807", 9_223_372_036_854_775_80, "9223372036.8547758"},

		// too many decimal places
		{"3.1415926535", 3_141_592_65, "3.14159265"}, // truncated

		// panic cases
		{"", 0, ""},
		{"_", 0, ""},
		{"1e2.5", 0, ""},
		// "9223372037", 0, ""}, // Now safe (9.2e17)
		{"92233720373483838", 0, ""},
	}

	for _, tt := range tests {
		if tt.wantStr == "" {
			assertPanics(t, tt.input, func() {
				d := Parse(tt.input)
				log.Printf("Parse(%q) = %d", tt.input, d)
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
	// This input has 16 fractional digits, more than the 8 decimal places supported.
	// The parser should truncate to 8 places: 2806.30821972
	input := "2806.3082197200404453"
	d := Parse(input)
	// Internal representation: 2806.30821972 * 1e8 = 280630821972
	want := int64(280_630_821_972)
	if int64(d) != want {
		t.Errorf("Parse(%q) = %d, want %d", input, d, want)
	}
	wantStr := "2806.30821972"
	if got := d.String(); got != wantStr {
		t.Errorf("Parse(%q).String() = %q, want %q", input, got, wantStr)
	}
}

func BenchmarkParseLongDecimal(b *testing.B) {
	input := "2806.3082197200404453"
	for i := 0; b.Loop(); i++ {
		_ = Parse(input)
	}
}
