package cboe

import (
	"dropbear/clocky"
	"dropbear/greg"
	"testing"
)

func TestGetCloseTime(t *testing.T) {
	tests := []struct {
		name      string
		date      clocky.Time
		wantHour  int
		wantEarly bool
	}{
		{"Day after Thanksgiving", clocky.Date(2026, clocky.November, 27, 13, 0, 0, 0, clocky.NYC), 13, true},
		{"Christmas Eve", clocky.Date(2026, clocky.December, 24, 13, 0, 0, 0, clocky.NYC), 13, true},
		{"Day before July 4 (Sat)", clocky.Date(2026, clocky.July, 2, 13, 0, 0, 0, clocky.NYC), 13, true},
		{"Thanksgiving itself", clocky.Date(2026, clocky.November, 26, 13, 0, 0, 0, clocky.NYC), 16, false},
		{"Random day", clocky.Date(2026, clocky.March, 15, 13, 0, 0, 0, clocky.NYC), 16, false},
	}
	for _, tt := range tests {
		got := GetCloseTime(tt.date)
		if got.Hour() != tt.wantHour {
			t.Errorf("%s: GetCloseTime() hour = %d, want %d", tt.name, got.Hour(), tt.wantHour)
		}
	}
}

func TestTradingDayCount(t *testing.T) {
	// Count trading days per year - should be ~250-253
	// Formula: 365 (or 366) - weekends (~104) - holidays (~9-10) = ~251-252
	for year := 2016; year <= 2030; year++ {
		count := 0
		for month := clocky.January; month <= clocky.December; month++ {
			days := greg.DaysIn(year, month)
			for day := 1; day <= days; day++ {
				dt := clocky.Date(year, month, day, 12, 0, 0, 0, clocky.NYC)
				if IsTradingDay(dt) {
					count++
				}
			}
		}
		// Sanity check: should be between 250 and 253
		if count < 250 || count > 253 {
			t.Errorf("Year %d: got %d trading days, expected 250-253", year, count)
		}
		t.Logf("Year %d: %d trading days", year, count)
	}
}

// Benchmark the holiday check
func BenchmarkIsHoliday(b *testing.B) {
	for b.Loop() {
		IsHoliday(2026, clocky.July, 4)
	}
}
