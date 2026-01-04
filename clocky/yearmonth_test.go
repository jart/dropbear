package clocky

import "testing"

func TestParseYearMonth(t *testing.T) {
	tests := []struct {
		input   string
		want    YearMonth
		wantErr bool
	}{
		{"2016-01", 201601, false},
		{"2016-12", 201612, false},
		{"1999-06", 199906, false},
		{"2025-09", 202509, false},
		{"2000-01", 200001, false},
		{"2023-07", 202307, false},

		// invalid format
		{"", 0, true},
		{"2016", 0, true},
		{"201601", 0, true},
		{"2016/01", 0, true},
		{"16-01", 0, true},
		{"2016-1", 0, true},
		{"abcd-ef", 0, true},

		// invalid month
		{"2016-00", 0, true},
		{"2016-13", 0, true},

		// invalid year
		{"1499-01", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseYearMonth(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseYearMonth(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseYearMonth(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestYearMonthString(t *testing.T) {
	tests := []struct {
		ym   YearMonth
		want string
	}{
		{201601, "2016-01"},
		{201612, "2016-12"},
		{199906, "1999-06"},
		{202509, "2025-09"},
		{200001, "2000-01"},
		{200012, "2000-12"},
		{202307, "2023-07"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.ym.String(); got != tt.want {
				t.Errorf("YearMonth(%d).String() = %q, want %q", tt.ym, got, tt.want)
			}
		})
	}
}

func TestYearMonthRoundTrip(t *testing.T) {
	tests := []string{
		"2016-01",
		"2016-12",
		"1999-06",
		"2025-09",
		"2000-01",
	}
	for _, s := range tests {
		t.Run(s, func(t *testing.T) {
			ym, err := ParseYearMonth(s)
			if err != nil {
				t.Fatalf("ParseYearMonth(%q) = %v", s, err)
			}
			if got := ym.String(); got != s {
				t.Errorf("round-trip: ParseYearMonth(%q).String() = %q", s, got)
			}
		})
	}
}

func TestYearMonthIncrement(t *testing.T) {
	tests := []struct {
		ym   YearMonth
		want YearMonth
	}{
		{201601, 201602},
		{201611, 201612},
		{201612, 201701},
		{199912, 200001},
	}
	for _, tt := range tests {
		t.Run(tt.ym.String(), func(t *testing.T) {
			if got := tt.ym.Increment(); got != tt.want {
				t.Errorf("YearMonth(%d).Increment() = %d, want %d", tt.ym, got, tt.want)
			}
		})
	}
}

func TestYearMonthYear(t *testing.T) {
	tests := []struct {
		ym   YearMonth
		want int
	}{
		{202509, 2025},
		{201601, 2016},
		{199912, 1999},
	}
	for _, tt := range tests {
		if got := tt.ym.Year(); got != tt.want {
			t.Errorf("YearMonth(%d).Year() = %d, want %d", tt.ym, got, tt.want)
		}
	}
}

func TestYearMonthMonth(t *testing.T) {
	tests := []struct {
		ym   YearMonth
		want int
	}{
		{202509, 9},
		{201601, 1},
		{199912, 12},
	}
	for _, tt := range tests {
		if got := tt.ym.Month(); got != tt.want {
			t.Errorf("YearMonth(%d).Month() = %d, want %d", tt.ym, got, tt.want)
		}
	}
}
