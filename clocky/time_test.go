package clocky

import (
	"testing"
	"time"
)

func TestTimeIsZero(t *testing.T) {
	if !Time(0).IsZero() {
		t.Error("Time(0).IsZero() should be true")
	}
	if Time(1).IsZero() {
		t.Error("Time(1).IsZero() should be false")
	}
}

func MyTest(t *testing.T) {
	ts := MustParseTime("2026-03-11 10:16:19.241")
	// now test fields
	if ts.Year() != 2026 {
		t.Errorf("Year() = %d, want 2026", ts.Year())
	}
	if ts.Month() != March {
		t.Errorf("Month() = %d, want 3 (March)", ts.Month())
	}
	if ts.Day() != 11 {
		t.Errorf("Day() = %d, want 11", ts.Day())
	}
	if ts.Hour() != 10 {
		t.Errorf("Hour() = %d, want 10", ts.Hour())
	}
	if ts.Minute() != 16 {
		t.Errorf("Minute() = %d, want 16", ts.Minute())
	}
	if ts.Second() != 19 {
		t.Errorf("Second() = %d, want 19", ts.Second())
	}
	if ts.Nanosecond() != 241_000_000 {
		t.Errorf("Nanosecond() = %d, want 241000000", ts.Nanosecond())
	}
}

func TestTimeAdd(t *testing.T) {
	tests := []struct {
		name     string
		time     Time
		duration Duration
		expected Time
	}{
		{"AddSecond", Time(1_000_000_000), Second, Time(2_000_000_000)},
		{"AddMinute", Time(0), Minute, Time(60_000_000_000)},
		{"AddHour", Time(0), Hour, Time(3_600_000_000_000)},
		{"AddNegative", Time(1_000_000_000), -Second, Time(0)},
		{"AddZero", Time(1_000_000_000), 0, Time(1_000_000_000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.time.Add(tt.duration)
			if got != tt.expected {
				t.Errorf("Time(%d).Add(%d) = %d, want %d", tt.time, tt.duration, got, tt.expected)
			}
		})
	}
}

func TestTimeSub(t *testing.T) {
	tests := []struct {
		name     string
		t1       Time
		t2       Time
		expected Duration
	}{
		{"SubPositive", Time(2_000_000_000), Time(1_000_000_000), Duration(1_000_000_000)},
		{"SubNegative", Time(1_000_000_000), Time(2_000_000_000), Duration(-1_000_000_000)},
		{"SubZero", Time(1_000_000_000), Time(1_000_000_000), Duration(0)},
		{"SubFromZero", Time(0), Time(1_000_000_000), Duration(-1_000_000_000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.t1.Sub(tt.t2)
			if got != tt.expected {
				t.Errorf("Time(%d).Sub(%d) = %d, want %d", tt.t1, tt.t2, got, tt.expected)
			}
		})
	}
}

func TestTimeUnix(t *testing.T) {
	tests := []struct {
		name     string
		time     Time
		expected int64
	}{
		{"Zero", Time(0), 0},
		{"OneSecond", Time(1_000_000_000), 1},
		{"FractionalSecond", Time(1_500_000_000), 1}, // truncates
		{"LargeValue", Time(1_700_000_000_000_000_000), 1_700_000_000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.time.Unix()
			if got != tt.expected {
				t.Errorf("Time(%d).Unix() = %d, want %d", tt.time, got, tt.expected)
			}
		})
	}
}

func TestTimeQuantize(t *testing.T) {
	base := Time(3_661_500_000_000) // 1h 1m 1.5s in nanoseconds

	tests := []struct {
		name     string
		time     Time
		duration Duration
		expected Time
	}{
		{"QuantizeToSecond", base, Second, Time(3_661_000_000_000)},
		{"QuantizeToMinute", base, Minute, Time(3_660_000_000_000)},
		{"QuantizeToHour", base, Hour, Time(3_600_000_000_000)},
		{"QuantizeToMillisecond", base, Millisecond, Time(3_661_500_000_000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.time.Quantize(tt.duration)
			if got != tt.expected {
				t.Errorf("Time(%d).Quantize(%d) = %d, want %d", tt.time, tt.duration, got, tt.expected)
			}
		})
	}
}

func TestTimeBeforeAfter(t *testing.T) {
	t1 := Time(1_000_000_000)
	t2 := Time(2_000_000_000)

	if !t1.Before(t2) {
		t.Error("t1.Before(t2) should be true")
	}
	if t2.Before(t1) {
		t.Error("t2.Before(t1) should be false")
	}
	if t1.Before(t1) {
		t.Error("t1.Before(t1) should be false")
	}

	if !t2.After(t1) {
		t.Error("t2.After(t1) should be true")
	}
	if t1.After(t2) {
		t.Error("t1.After(t2) should be false")
	}
	if t1.After(t1) {
		t.Error("t1.After(t1) should be false")
	}
}

func TestTimeString(t *testing.T) {
	// Create a known time in UTC, then convert to Time
	utc := time.Date(2024, 6, 15, 12, 30, 45, 123456789, time.UTC)
	tm := Time(utc.UnixNano())

	// String() formats in local timezone, so we just check it's non-empty
	// and has the expected format structure
	s := tm.String()
	if len(s) != 29 { // "2006-01-02T15:04:05.000000000"
		t.Errorf("Time.String() = %q, expected length 29", s)
	}
}

func TestTimeRFC3339(t *testing.T) {
	// Create a known time in UTC
	utc := time.Date(2024, 6, 15, 12, 30, 45, 123456789, time.UTC)
	tm := Time(utc.UnixNano())

	got := tm.RFC3339()
	expected := "2024-06-15T12:30:45.123456789Z"
	if got != expected {
		t.Errorf("Time.RFC3339() = %q, want %q", got, expected)
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Empty", "", false},
		{"RFC3339", "2024-06-15T12:30:45Z", false},
		{"RFC3339Nano", "2024-06-15T12:30:45.123456789Z", false},
		{"RFC3339Offset", "2024-06-15T12:30:45-07:00", false},
		{"LocalDateTime", "2024-06-15T12:30:45", false},
		{"LocalDateTimeMinute", "2024-06-15T12:30", false},
		{"LocalDate", "2024-06-15", false},
		{"Invalid", "not-a-time", true},
		{"InvalidFormat", "15/06/2024", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if tt.input == "" && got != 0 {
				t.Errorf("ParseTime(\"\") = %d, want 0", got)
			}
		})
	}
}

func TestParseTimeRoundTrip(t *testing.T) {
	// Test that RFC3339 round-trips correctly
	inputs := []string{
		"2024-06-15T12:30:45.123456789Z",
		"2025-12-03T15:01:47.170451000Z",
	}
	for _, original := range inputs {
		t.Run(original, func(t *testing.T) {
			parsed, err := ParseTime(original)
			if err != nil {
				t.Fatalf("ParseTime(%q) error = %v", original, err)
			}
			// RFC3339 output should match
			got := parsed.RFC3339()
			if got != original {
				t.Errorf("Round trip failed: %q -> %d -> %q", original, parsed, got)
			}
		})
	}
}

func TestParseTimeValues(t *testing.T) {
	// Test specific parsed values
	tests := []struct {
		input    string
		expected int64 // Unix timestamp
	}{
		{"2024-01-01T00:00:00Z", 1704067200},
		{"1970-01-01T00:00:00Z", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseTime(tt.input)
			if err != nil {
				t.Fatalf("ParseTime(%q) error = %v", tt.input, err)
			}
			if got.Unix() != tt.expected {
				t.Errorf("ParseTime(%q).Unix() = %d, want %d", tt.input, got.Unix(), tt.expected)
			}
		})
	}
}

func BenchmarkParseTime(b *testing.B) {
	for b.Loop() {
		ParseTime("2025-12-03T15:01:47.170451000Z")
	}
}
