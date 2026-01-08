package sip

import (
	"dropbear/clocky"
	"testing"
)

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		input string
		want  string // expected time as RFC3339
	}{
		// Basic cases
		{`"2026-01-06T20:12:59.99839309Z"`, "2026-01-06T20:12:59.998393Z"},
		{`"2026-01-06T20:12:00Z"`, "2026-01-06T20:12:00Z"},
		{`"2024-01-15T14:30:00.123456789Z"`, "2024-01-15T14:30:00.123456Z"},

		// Unix epoch
		{`"1970-01-01T00:00:00Z"`, "1970-01-01T00:00:00Z"},
		{`"1970-01-01T00:00:01Z"`, "1970-01-01T00:00:01Z"},
		{`"1970-12-31T23:59:59Z"`, "1970-12-31T23:59:59Z"},

		// Leap years (divisible by 4)
		{`"2000-02-29T12:00:00Z"`, "2000-02-29T12:00:00Z"}, // century leap year (div by 400)
		{`"2004-02-29T00:00:00Z"`, "2004-02-29T00:00:00Z"},
		{`"2024-02-29T23:59:59Z"`, "2024-02-29T23:59:59Z"},

		// Century non-leap years (divisible by 100 but not 400)
		{`"1900-03-01T00:00:00Z"`, "1900-03-01T00:00:00Z"}, // 1900 was NOT a leap year
		{`"2100-03-01T00:00:00Z"`, "2100-03-01T00:00:00Z"}, // 2100 won't be a leap year

		// Year boundaries
		{`"1999-12-31T23:59:59Z"`, "1999-12-31T23:59:59Z"},
		{`"2000-01-01T00:00:00Z"`, "2000-01-01T00:00:00Z"},

		// Month boundaries
		{`"2023-01-31T12:00:00Z"`, "2023-01-31T12:00:00Z"},
		{`"2023-02-28T12:00:00Z"`, "2023-02-28T12:00:00Z"},
		{`"2023-03-31T12:00:00Z"`, "2023-03-31T12:00:00Z"},
		{`"2023-04-30T12:00:00Z"`, "2023-04-30T12:00:00Z"},
		{`"2023-12-31T12:00:00Z"`, "2023-12-31T12:00:00Z"},

		// Far future (make sure we don't overflow)
		{`"2099-12-31T23:59:59Z"`, "2099-12-31T23:59:59Z"},
	}
	for _, tt := range tests {
		data := []byte(tt.input)
		got, _, err := parseTimestamp(data, 0)
		if err != nil {
			t.Errorf("parseTimestamp(%s) error: %v", tt.input, err)
			continue
		}
		// Compare with clocky.ParseTime
		want, err := clocky.ParseTime(tt.want)
		if err != nil {
			t.Fatalf("clocky.ParseTime(%s) error: %v", tt.want, err)
		}
		if got != want {
			t.Errorf("parseTimestamp(%s) = %d, want %d (diff: %d µs)",
				tt.input, got, want, got-want)
		}
	}
}

var tsBenchData = []byte(`"2026-01-06T20:12:59.99839309Z"`)
var tsBenchDataShort = []byte(`"2026-01-06T20:12:00Z"`)

func BenchmarkParseTimestamp(b *testing.B) {
	for b.Loop() {
		parseTimestamp(tsBenchData, 0)
	}
}

func BenchmarkParseTimestampShort(b *testing.B) {
	for b.Loop() {
		parseTimestamp(tsBenchDataShort, 0)
	}
}

func BenchmarkClockyParseTime(b *testing.B) {
	s := "2026-01-06T20:12:59.99839309Z"
	for b.Loop() {
		clocky.ParseTime(s)
	}
}
