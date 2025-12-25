package clocky

import (
	"testing"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected Duration
		wantErr  bool
	}{
		// Empty and whitespace
		{"", 0, false},
		{"  ", 0, false},
		{"\t", 0, false},

		// Microseconds
		{"1us", 1 * Microsecond, false},
		{"100u", 100 * Microsecond, false},
		{"500us", 500 * Microsecond, false},
		{" 500us ", 500 * Microsecond, false},

		// Milliseconds
		{"1ms", 1 * Millisecond, false},
		{"100ms", 100 * Millisecond, false},
		{"1500ms", 1500 * Millisecond, false},

		// Seconds
		{"1", 1 * Second, false},
		{"1s", 1 * Second, false},
		{"30s", 30 * Second, false},
		{"90s", 90 * Second, false},

		// Minutes (lowercase m without s)
		{"1m", 1 * Minute, false},
		{"30m", 30 * Minute, false},
		{"90m", 90 * Minute, false},

		// Minutes (uppercase M)
		{"1M", 1 * Minute, false},
		{"5M", 5 * Minute, false},

		// Hours
		{"1h", 1 * Hour, false},
		{"24h", 24 * Hour, false},

		// Days
		{"1d", 1 * Day, false},
		{"7d", 7 * Day, false},
		{"30d", 30 * Day, false},

		// Weeks
		{"1w", 1 * Week, false},
		{"4w", 4 * Week, false},

		// Years
		{"1y", 1 * Year, false},
		{"2y", 2 * Year, false},

		// Negative values
		{"-1s", -1 * Second, false},
		{"-30m", -30 * Minute, false},
		{"-24h", -24 * Hour, false},
		{"-7d", -7 * Day, false},

		// Compound durations
		{"1h30m", 1*Hour + 30*Minute, false},
		{"2h45m", 2*Hour + 45*Minute, false},
		{"1d12h", 1*Day + 12*Hour, false},
		{"1h30m45s", 1*Hour + 30*Minute + 45*Second, false},
		{"1w2d3h", 1*Week + 2*Day + 3*Hour, false},
		{"1d-1h", 1*Day - 1*Hour, false},
		{"-1d-1h", -(1*Day - 1*Hour), false},

		// Invalid units
		{"1x", 1, true},
		{"5foo", 5, true},
		{"5m5foo", 5*Minute + 5, true},

		// Just a number (no unit)
		{"123", 123 * Second, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDurationString(t *testing.T) {
	tests := []struct {
		input    Duration
		expected string
	}{
		// Zero
		{0, "0"},

		// Single units
		{1 * Microsecond, "1us"},
		{1 * Millisecond, "1ms"},
		{1 * Second, "1s"},
		{1 * Minute, "1m"},
		{1 * Hour, "1h"},
		{1 * Day, "1d"},
		{1 * Week, "1w"},
		{1 * Year, "1y"},

		// Multiple of single units
		{500 * Microsecond, "500us"},
		{100 * Millisecond, "100ms"},
		{30 * Second, "30s"},
		{45 * Minute, "45m"},
		{24 * Hour, "1d"},
		{7 * Day, "1w"},

		// Compound durations
		{1*Hour + 30*Minute, "1h30m"},
		{2*Hour + 45*Minute, "2h45m"},
		{1*Day + 12*Hour, "1d12h"},
		{1*Hour + 30*Minute + 45*Second, "1h30m45s"},
		{1*Week + 2*Day + 3*Hour, "1w2d3h"},

		// Negative values
		{-1 * Second, "-1s"},
		{-30 * Minute, "-30m"},
		{-1*Hour - 30*Minute, "-1h30m"},

		// Complex mixed
		{1*Year + 2*Week + 3*Day + 4*Hour + 5*Minute + 6*Second + 7*Millisecond + 8*Microsecond,
			"1y2w3d4h5m6s7ms8us"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.input.String()
			if got != tt.expected {
				t.Errorf("Duration(%d).String() = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDurationConstants(t *testing.T) {
	// Verify the relationships between duration constants
	if Millisecond != 1000*Microsecond {
		t.Errorf("Millisecond = %d, want %d", Millisecond, 1000*Microsecond)
	}
	if Second != 1000*Millisecond {
		t.Errorf("Second = %d, want %d", Second, 1000*Millisecond)
	}
	if Minute != 60*Second {
		t.Errorf("Minute = %d, want %d", Minute, 60*Second)
	}
	if Hour != 60*Minute {
		t.Errorf("Hour = %d, want %d", Hour, 60*Minute)
	}
	if Day != 24*Hour {
		t.Errorf("Day = %d, want %d", Day, 24*Hour)
	}
	if Week != 7*Day {
		t.Errorf("Week = %d, want %d", Week, 7*Day)
	}
	if Year != 365*Day {
		t.Errorf("Year = %d, want %d", Year, 365*Day)
	}
}

func TestParseDurationRoundTrip(t *testing.T) {
	// Test that parsing the string representation gives back the original value
	durations := []Duration{
		0,
		1 * Microsecond,
		1 * Millisecond,
		1 * Second,
		1 * Minute,
		1 * Hour,
		1 * Day,
		1 * Week,
		1 * Year,
		1*Hour + 30*Minute,
		2*Day + 5*Hour + 30*Minute + 15*Second,
		-1 * Hour,
		-1 * Day,
		-2*Day - 3*Hour,
	}

	for _, d := range durations {
		t.Run(d.String(), func(t *testing.T) {
			str := d.String()
			parsed, err := ParseDuration(str)
			if err != nil {
				t.Errorf("ParseDuration(%q) returned error: %v", str, err)
				return
			}
			if parsed != d {
				t.Errorf("Round trip failed: %d -> %q -> %d", d, str, parsed)
			}
		})
	}
}

func TestParseDurationNegativeCompound(t *testing.T) {
	// The negative sign should apply to the entire compound duration
	// "-2d3h" should parse as -(2d + 3h)
	got, err := ParseDuration("-2d3h")
	if err != nil {
		t.Fatalf("ParseDuration returned error: %v", err)
	}
	expected := -2*Day - 3*Hour
	if got != expected {
		t.Errorf("ParseDuration(%q) = %v, want %v", "-2d3h", got, expected)
	}
}

func BenchmarkParseDuration_Simple(b *testing.B) {
	for b.Loop() {
		ParseDuration("30s")
	}
}

func BenchmarkParseDuration_Compound(b *testing.B) {
	for b.Loop() {
		ParseDuration("1h30m45s")
	}
}
