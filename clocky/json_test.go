package clocky

import (
	"encoding/json"
	"testing"
)

func TestTimeUnmarshalJSON(t *testing.T) {
	// 2024-01-15T10:30:00Z = 1705314600 seconds = 1705314600000000 micros
	want := Time(1705314600_000000)

	tests := []struct {
		name  string
		input string
		want  Time
	}{
		// RFC3339 strings
		{"rfc3339", `"2024-01-15T10:30:00Z"`, want},
		{"rfc3339_micros", `"2024-01-15T10:30:00.123456Z"`, want + 123456},

		// Unix seconds (integer)
		{"unix_seconds", `1705314600`, want},

		// Unix seconds (float)
		{"unix_seconds_float", `1705314600.0`, want},
		{"unix_seconds_float_frac", `1705314600.123456`, want + 123456},
		{"unix_seconds_float_short", `1705314600.5`, want + 500000},

		// Unix milliseconds
		{"unix_millis", `1705314600000`, want},
		{"unix_millis_precise", `1705314600123`, want + 123000},

		// Unix microseconds
		{"unix_micros", `1705314600000000`, want},
		{"unix_micros_precise", `1705314600123456`, want + 123456},

		// Zero
		{"zero_int", `0`, 0},
		{"zero_string", `"1970-01-01T00:00:00Z"`, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Time
			if err := json.Unmarshal([]byte(tt.input), &got); err != nil {
				t.Fatalf("Unmarshal(%s) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Unmarshal(%s) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestTimeUnmarshalJSONErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		// Ambiguous timestamps before 1980
		{"old_seconds", `100000000`},           // 1973 as seconds
		{"old_millis", `100000000000`},         // 1973 as millis
		{"old_micros", `100000000000000`},      // 1973 as micros
		{"old_float", `100000000.0`},           // 1973 as float seconds
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Time
			err := json.Unmarshal([]byte(tt.input), &got)
			if err == nil {
				t.Errorf("Unmarshal(%s) should have failed, got %v", tt.input, got)
			}
		})
	}
}

func TestTimeMarshalJSON(t *testing.T) {
	tests := []struct {
		input Time
		want  string
	}{
		{Time(1705314600_000000), `"2024-01-15T10:30:00.000000Z"`},
		{Time(1705314600_123456), `"2024-01-15T10:30:00.123456Z"`},
		{Time(0), `"1970-01-01T00:00:00.000000Z"`},
	}

	for _, tt := range tests {
		got, err := tt.input.MarshalJSON()
		if err != nil {
			t.Errorf("MarshalJSON(%v) error: %v", tt.input, err)
			continue
		}
		if string(got) != tt.want {
			t.Errorf("MarshalJSON(%v) = %s, want %s", tt.input, got, tt.want)
		}
	}
}

func TestTimeMarshalJSONYearOverflow(t *testing.T) {
	// Year 10000 = 253402300800 seconds from epoch
	year10000 := Time(253_402_300_800_000_000)
	_, err := year10000.MarshalJSON()
	if err != ErrYearOverflow {
		t.Errorf("MarshalJSON(year 10000) should return ErrYearOverflow, got %v", err)
	}

	// Negative year (before year 0)
	negYear := Time(-62_167_219_200_000_001) // just before year 0
	_, err = negYear.MarshalJSON()
	if err != ErrYearOverflow {
		t.Errorf("MarshalJSON(negative year) should return ErrYearOverflow, got %v", err)
	}
}

func TestTimeJSONRoundTrip(t *testing.T) {
	type Event struct {
		Timestamp Time `json:"timestamp"`
	}

	original := Event{Timestamp: Time(1705314600_123456)}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Timestamp != original.Timestamp {
		t.Errorf("Timestamp = %v, want %v", decoded.Timestamp, original.Timestamp)
	}
}

func TestFromUnixAuto(t *testing.T) {
	tests := []struct {
		name    string
		input   int64
		want    Time
		wantErr bool
	}{
		// Seconds detection
		{"secs_2024", 1705314600, Time(1705314600_000000), false},
		{"secs_1980", 315_532_800, Time(315_532_800_000000), false}, // exactly 1980
		{"secs_2000", 946684800, Time(946684800_000000), false},

		// Millis detection
		{"millis_2024", 1705314600_000, Time(1705314600_000000), false},
		{"millis_1980", 315_532_800_000, Time(315_532_800_000000), false}, // exactly 1980

		// Micros detection
		{"micros_2024", 1705314600_000000, Time(1705314600_000000), false},
		{"micros_1980", 315_532_800_000_000, Time(315_532_800_000000), false}, // exactly 1980

		// Errors: before 1980 (outside epoch window)
		{"secs_1973", 100_000_000, 0, true},           // 1973 as seconds
		{"millis_1973", 100_000_000_000, 0, true},     // 1973 as millis
		{"micros_1973", 100_000_000_000_000, 0, true}, // 1973 as micros
		{"negative_far", -1705314600, 0, true},        // negative outside window

		// Epoch window: ±2 days around zero allowed
		{"zero", 0, 0, false},
		{"near_epoch_pos", 86400, Time(86400_000000), false},   // +1 day
		{"near_epoch_neg", -86400, Time(-86400_000000), false}, // -1 day
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fromUnixAuto(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("fromUnixAuto(%d) should have errored, got %d", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("fromUnixAuto(%d) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("fromUnixAuto(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestTimestampHeuristicEdgeCases(t *testing.T) {
	// Key dates in different units
	// 1970-01-01 00:00:00 UTC = 0
	// 1980-01-01 00:00:00 UTC = 315532800 seconds
	// 2000-01-01 00:00:00 UTC = 946684800 seconds

	const (
		epoch1980Secs   = 315_532_800
		epoch1980Millis = 315_532_800_000
		epoch1980Micros = 315_532_800_000_000
	)

	t.Run("1980_boundary_accepted", func(t *testing.T) {
		// 1980 is exactly at the boundary - should be accepted
		cases := []struct {
			name  string
			value int64
			want  Time
		}{
			{"1980_as_seconds", epoch1980Secs, Time(epoch1980Micros)},
			{"1980_as_millis", epoch1980Millis, Time(epoch1980Micros)},
			{"1980_as_micros", epoch1980Micros, Time(epoch1980Micros)},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, err := fromUnixAuto(tc.value)
				if err != nil {
					t.Fatalf("fromUnixAuto(%d) unexpected error: %v", tc.value, err)
				}
				if got != tc.want {
					t.Errorf("fromUnixAuto(%d) = %d, want %d", tc.value, got, tc.want)
				}
			})
		}
	})

	t.Run("pre_1980_rejected", func(t *testing.T) {
		// Values that would result in dates before 1980 are rejected
		cases := []struct {
			name  string
			value int64
		}{
			{"1973_as_seconds", 100_000_000},
			{"1973_as_millis", 100_000_000_000},
			{"1973_as_micros", 100_000_000_000_000},
			{"1979_as_seconds", 315_532_799}, // one second before 1980
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := fromUnixAuto(tc.value)
				if err == nil {
					t.Errorf("fromUnixAuto(%d) should have errored", tc.value)
				}
			})
		}
	})
}

func TestTimestampLimits(t *testing.T) {
	// Document the exact limits of each timestamp format

	t.Run("int64_seconds", func(t *testing.T) {
		// Heuristic: values < 1e11 are treated as seconds
		// Max: 99,999,999,999 seconds = 99999999999
		// = 99,999,999,999 / 86400 / 365.25 = 3168.8 years
		// = year 5138
		maxSecs := int64(99_999_999_999)
		got, err := fromUnixAuto(maxSecs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Logf("max seconds: %d -> %s", maxSecs, got.RFC3339())

		// One more and it becomes millis (which would be 1973, rejected)
		_, err = fromUnixAuto(100_000_000_000)
		if err == nil {
			t.Error("100e9 should error (interpreted as millis = 1973)")
		}
	})

	t.Run("int64_millis", func(t *testing.T) {
		// Heuristic: values >= 1e11 and < 1e14 are treated as millis
		// Min: 100,000,000,000 ms = 100,000,000 seconds = 1973 (rejected)
		// First valid: 315,532,800,000 ms = 1980-01-01
		// Max: 99,999,999,999,999 ms = 99,999,999,999 seconds = year 5138
		minValidMillis := int64(315_532_800_000) // 1980
		got, err := fromUnixAuto(minValidMillis)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Logf("min valid millis: %d -> %s", minValidMillis, got.RFC3339())

		maxMillis := int64(99_999_999_999_999)
		got, err = fromUnixAuto(maxMillis)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Logf("max millis: %d -> %s", maxMillis, got.RFC3339())
	})

	t.Run("int64_micros", func(t *testing.T) {
		// Heuristic: values >= 1e14 are treated as micros
		// Min: 100,000,000,000,000 µs = 100,000,000 seconds = 1973 (rejected)
		// First valid: 315,532,800,000,000 µs = 1980-01-01
		// Max: limited by int64 = 9,223,372,036,854,775,807 µs
		//    = 9,223,372,036,854 seconds = year 294,441
		minValidMicros := int64(315_532_800_000_000) // 1980
		got, err := fromUnixAuto(minValidMicros)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Logf("min valid micros: %d -> %s", minValidMicros, got.RFC3339())

		// Test a large value
		year3000Micros := int64(32_503_680_000_000_000) // ~year 3000
		got, err = fromUnixAuto(year3000Micros)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Logf("year 3000 micros: %d -> %s", year3000Micros, got.RFC3339())
	})

	t.Run("float_precision", func(t *testing.T) {
		// float64 has 53 bits of mantissa = 9,007,199,254,740,992
		// For µs precision (6 decimal places), we need integer + fraction
		// At 1e9 seconds (year 2001), we have ~53-30=23 bits for fraction = fine
		// At 1e10 seconds (year 2286), we have ~53-34=19 bits for fraction
		// Precision loss starts around 2^53 / 1e6 = 9,007,199,254 seconds = year 2255

		// Test current era - should be fine
		input := `1705314600.123456`
		var got Time
		err := got.UnmarshalJSON([]byte(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Logf("2024 float: %s -> %s (µs=%d)", input, got.RFC3339(), got%1_000_000)

		// Test year 2200 - still should be fine
		input2200 := `7258118400.123456` // ~2200
		err = got.UnmarshalJSON([]byte(input2200))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Logf("2200 float: %s -> %s (µs=%d)", input2200, got.RFC3339(), got%1_000_000)

		// Note: After ~year 2255, float64 loses µs precision
		// 9,007,199,254 seconds = 2^53 / 1e6 = max for exact µs
		t.Logf("float64 µs precision limit: ~year 2255 (9,007,199,254 seconds)")
	})
}

func TestRFC3339RequiresTimezone(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid: has timezone
		{"utc_z", `"2024-01-15T10:30:00Z"`, false},
		{"utc_offset", `"2024-01-15T10:30:00+00:00"`, false},
		{"positive_offset", `"2024-01-15T10:30:00+05:30"`, false},
		{"negative_offset", `"2024-01-15T10:30:00-08:00"`, false},
		{"with_micros", `"2024-01-15T10:30:00.123456Z"`, false},

		// Invalid: no timezone
		{"no_timezone", `"2024-01-15T10:30:00"`, true},
		{"no_tz_with_micros", `"2024-01-15T10:30:00.123456"`, true},
		{"date_only", `"2024-01-15"`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Time
			err := got.UnmarshalJSON([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Errorf("UnmarshalJSON(%s) should have failed, got %v", tt.input, got)
				}
			} else {
				if err != nil {
					t.Errorf("UnmarshalJSON(%s) unexpected error: %v", tt.input, err)
				}
			}
		})
	}
}

func BenchmarkTimeMarshalJSON(b *testing.B) {
	t := Time(1705314600_123456)
	b.ReportAllocs()
	for b.Loop() {
		t.MarshalJSON()
	}
}

func BenchmarkTimeUnmarshalJSON(b *testing.B) {
	data := []byte(`1705314600000`)
	b.ReportAllocs()
	for b.Loop() {
		var t Time
		t.UnmarshalJSON(data)
	}
}
