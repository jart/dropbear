package cboe

import (
	"dropbear/clocky"
	"dropbear/greg"
	"testing"
)

// IsMarketOpen returns true if the market is open for day trading.
func IsMarketOpen(dt clocky.Time) bool {
	return GetSession(dt) == SessionDay
}

// IsMarketOpenExtended returns true if the market is open for day trading or extended hours.
func IsMarketOpenExtended(dt clocky.Time) bool {
	s := GetSession(dt)
	return s == SessionDay || s == SessionExtended
}

// IsMarketOpenExtendedOvernight returns true if markets are open for day/extended/overnight trading.
func IsMarketOpenExtendedOvernight(dt clocky.Time) bool {
	return GetSession(dt) != SessionClosed
}

// IsExtended returns true if the market is open for extended hours only.
func IsExtended(dt clocky.Time) bool {
	return GetSession(dt) == SessionExtended
}

// IsOvernight returns true if the market is open for overnight trading only.
func IsOvernight(dt clocky.Time) bool {
	return GetSession(dt) == SessionOvernight
}

func TestGetCloseTime(t *testing.T) {
	tests := []struct {
		name      string
		year      int
		month     clocky.Month
		day       int
		wantHour  int
		wantEarly bool
	}{
		{"Day after Thanksgiving", 2026, clocky.November, 27, 13, true},
		{"Christmas Eve", 2026, clocky.December, 24, 13, true},
		{"Day before July 4 (Sat)", 2026, clocky.July, 2, 13, true},
		{"Thanksgiving itself", 2026, clocky.November, 26, 16, false},
		{"Random day", 2026, clocky.March, 15, 16, false},
	}
	for _, tt := range tests {
		got := GetCloseTime(tt.year, tt.month, tt.day)
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
				if IsTradingDay(year, month, day) {
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

func TestIsOvernight(t *testing.T) {
	tests := []struct {
		name string
		dt   clocky.Time
		want bool
	}{
		{"Thursday 3:00 AM", clocky.Date(2026, clocky.April, 23, 3, 0, 0, 0, clocky.NYC), true},
		{"Thursday 4:00 AM", clocky.Date(2026, clocky.April, 23, 4, 0, 0, 0, clocky.NYC), false},
		{"Thursday 10:00 PM", clocky.Date(2026, clocky.April, 23, 22, 0, 0, 0, clocky.NYC), true},
		{"Friday 10:00 PM", clocky.Date(2026, clocky.April, 24, 22, 0, 0, 0, clocky.NYC), false},
		// 2025: July 4 is Friday (holiday), July 3 (Thursday) is early close
		// extended closes at 5 PM, overnight never starts because July 4 is holiday
		{"Wednesday July 2nd 2025 8:00 PM", clocky.Date(2025, clocky.July, 2, 20, 0, 0, 0, clocky.NYC), true},  // next day is trading day
		{"Thursday July 3rd 2025 3:00 AM", clocky.Date(2025, clocky.July, 3, 3, 0, 0, 0, clocky.NYC), true},    // today is trading day
		{"Thursday July 3rd 2025 5:00 PM", clocky.Date(2025, clocky.July, 3, 17, 0, 0, 0, clocky.NYC), false},  // gap after early extended close
		{"Thursday July 3rd 2025 7:00 PM", clocky.Date(2025, clocky.July, 3, 19, 0, 0, 0, clocky.NYC), false},  // gap
		{"Thursday July 3rd 2025 8:00 PM", clocky.Date(2025, clocky.July, 3, 20, 0, 0, 0, clocky.NYC), false},  // next day is holiday
		{"Thursday July 3rd 2025 10:00 PM", clocky.Date(2025, clocky.July, 3, 22, 0, 0, 0, clocky.NYC), false}, // next day is holiday
		{"Friday July 4th 2025 3:00 AM", clocky.Date(2025, clocky.July, 4, 3, 0, 0, 0, clocky.NYC), false},     // holiday
		// 2026: July 4 is Saturday, observed Friday July 3 (holiday), July 2 (Thursday) is early close
		{"Tuesday July 1st 2026 8:00 PM", clocky.Date(2026, clocky.July, 1, 20, 0, 0, 0, clocky.NYC), true},   // next day is trading day
		{"Thursday July 2nd 2026 3:00 AM", clocky.Date(2026, clocky.July, 2, 3, 0, 0, 0, clocky.NYC), true},   // today is trading day
		{"Thursday July 2nd 2026 5:00 PM", clocky.Date(2026, clocky.July, 2, 17, 0, 0, 0, clocky.NYC), false}, // gap after early extended close
		{"Thursday July 2nd 2026 7:00 PM", clocky.Date(2026, clocky.July, 2, 19, 0, 0, 0, clocky.NYC), false}, // gap
		{"Thursday July 2nd 2026 8:00 PM", clocky.Date(2026, clocky.July, 2, 20, 0, 0, 0, clocky.NYC), false}, // next day is observed holiday
		{"Friday July 3rd 2026 3:00 AM", clocky.Date(2026, clocky.July, 3, 3, 0, 0, 0, clocky.NYC), false},    // observed holiday
		{"Friday July 3rd 2026 8:00 PM", clocky.Date(2026, clocky.July, 3, 20, 0, 0, 0, clocky.NYC), false},   // next day is Saturday
		{"Saturday July 4th 2026 2:30 AM", clocky.Date(2026, clocky.July, 4, 2, 30, 0, 0, clocky.NYC), false},
	}
	for _, tt := range tests {
		if got := IsOvernight(tt.dt); got != tt.want {
			t.Errorf("%s: IsOvernight() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsMarketOpen(t *testing.T) {
	tests := []struct {
		name string
		dt   clocky.Time
		want bool
	}{
		{"Thursday 3:59 AM", clocky.Date(2026, clocky.April, 23, 3, 59, 0, 0, clocky.NYC), false},
		{"Thursday 4:00 AM", clocky.Date(2026, clocky.April, 23, 4, 0, 0, 0, clocky.NYC), false},
		{"Thursday 4:01 AM", clocky.Date(2026, clocky.April, 23, 4, 1, 0, 0, clocky.NYC), false},
		{"Thursday 9:29 AM", clocky.Date(2026, clocky.April, 23, 9, 29, 0, 0, clocky.NYC), false},
		{"Thursday 9:30 AM", clocky.Date(2026, clocky.April, 23, 9, 30, 0, 0, clocky.NYC), true},
		{"Thursday 3:59 PM", clocky.Date(2026, clocky.April, 23, 15, 59, 0, 0, clocky.NYC), true},
		{"Thursday 4:00 PM", clocky.Date(2026, clocky.April, 23, 16, 0, 0, 0, clocky.NYC), false},
		{"Thursday 4:30 PM", clocky.Date(2026, clocky.April, 23, 16, 30, 0, 0, clocky.NYC), false},
		{"Thursday 7:00 PM", clocky.Date(2026, clocky.April, 23, 19, 0, 0, 0, clocky.NYC), false},
		{"Thursday 8:00 PM", clocky.Date(2026, clocky.April, 23, 20, 0, 0, 0, clocky.NYC), false},
		{"Saturday 10:00 AM", clocky.Date(2026, clocky.April, 25, 10, 0, 0, 0, clocky.NYC), false},
		// 2025: July 4 is Friday (holiday), July 3 (Thursday) is early close
		{"Thursday July 3rd 2025 9:30 AM", clocky.Date(2025, clocky.July, 3, 9, 30, 0, 0, clocky.NYC), true},
		{"Thursday July 3rd 2025 12:00 PM", clocky.Date(2025, clocky.July, 3, 12, 0, 0, 0, clocky.NYC), true},
		{"Thursday July 3rd 2025 12:59 PM", clocky.Date(2025, clocky.July, 3, 12, 59, 0, 0, clocky.NYC), true},
		{"Thursday July 3rd 2025 1:00 PM", clocky.Date(2025, clocky.July, 3, 13, 0, 0, 0, clocky.NYC), false}, // early close
		{"Thursday July 3rd 2025 4:00 PM", clocky.Date(2025, clocky.July, 3, 16, 0, 0, 0, clocky.NYC), false},
		{"Friday July 4th 2025 9:30 AM", clocky.Date(2025, clocky.July, 4, 9, 30, 0, 0, clocky.NYC), false},  // holiday
		{"Friday July 4th 2025 12:00 PM", clocky.Date(2025, clocky.July, 4, 12, 0, 0, 0, clocky.NYC), false}, // holiday
		// 2026: July 4 is Saturday, observed Friday July 3 (holiday), July 2 (Thursday) is early close
		{"Thursday July 2nd 2026 12:00 PM", clocky.Date(2026, clocky.July, 2, 12, 0, 0, 0, clocky.NYC), true},
		{"Thursday July 2nd 2026 12:59 PM", clocky.Date(2026, clocky.July, 2, 12, 59, 0, 0, clocky.NYC), true},
		{"Thursday July 2nd 2026 1:00 PM", clocky.Date(2026, clocky.July, 2, 13, 0, 0, 0, clocky.NYC), false}, // early close
		{"Thursday July 2nd 2026 4:59 PM", clocky.Date(2026, clocky.July, 2, 16, 59, 0, 0, clocky.NYC), false},
		{"Friday July 3rd 2026 12:00 PM", clocky.Date(2026, clocky.July, 3, 12, 0, 0, 0, clocky.NYC), false}, // observed holiday
		{"Friday July 3rd 2026 1:00 PM", clocky.Date(2026, clocky.July, 3, 13, 0, 0, 0, clocky.NYC), false},  // observed holiday
		{"Saturday July 4th 2026 9:30 AM", clocky.Date(2026, clocky.July, 4, 9, 30, 0, 0, clocky.NYC), false},
	}
	for _, tt := range tests {
		if got := IsMarketOpen(tt.dt); got != tt.want {
			t.Errorf("%s: IsMarketOpen() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsMarketOpenExtended(t *testing.T) {
	tests := []struct {
		name string
		dt   clocky.Time
		want bool
	}{
		{"Thursday 3:59 AM", clocky.Date(2026, clocky.April, 23, 3, 59, 0, 0, clocky.NYC), false},
		{"Thursday 4:00 AM", clocky.Date(2026, clocky.April, 23, 4, 0, 0, 0, clocky.NYC), true},
		{"Thursday 4:01 AM", clocky.Date(2026, clocky.April, 23, 4, 1, 0, 0, clocky.NYC), true},
		{"Thursday 9:30 AM", clocky.Date(2026, clocky.April, 23, 9, 30, 0, 0, clocky.NYC), true},
		{"Thursday 4:00 PM", clocky.Date(2026, clocky.April, 23, 16, 0, 0, 0, clocky.NYC), true},
		{"Thursday 4:30 PM", clocky.Date(2026, clocky.April, 23, 16, 30, 0, 0, clocky.NYC), true},
		{"Thursday 7:00 PM", clocky.Date(2026, clocky.April, 23, 19, 0, 0, 0, clocky.NYC), true},
		{"Thursday 7:59 PM", clocky.Date(2026, clocky.April, 23, 19, 59, 0, 0, clocky.NYC), true},
		{"Thursday 8:00 PM", clocky.Date(2026, clocky.April, 23, 20, 0, 0, 0, clocky.NYC), false},
		{"Saturday 10:00 AM", clocky.Date(2026, clocky.April, 25, 10, 0, 0, 0, clocky.NYC), false},
		// 2025: July 4 is Friday (holiday), July 3 (Thursday) is early close
		// extended closes at 5 PM instead of 8 PM, leaving a gap before overnight (which never starts)
		{"Thursday July 3rd 2025 4:00 AM", clocky.Date(2025, clocky.July, 3, 4, 0, 0, 0, clocky.NYC), true},
		{"Thursday July 3rd 2025 12:00 PM", clocky.Date(2025, clocky.July, 3, 12, 0, 0, 0, clocky.NYC), true},
		{"Thursday July 3rd 2025 1:00 PM", clocky.Date(2025, clocky.July, 3, 13, 0, 0, 0, clocky.NYC), true},  // market closed but extended open
		{"Thursday July 3rd 2025 4:59 PM", clocky.Date(2025, clocky.July, 3, 16, 59, 0, 0, clocky.NYC), true}, // still extended
		{"Thursday July 3rd 2025 5:00 PM", clocky.Date(2025, clocky.July, 3, 17, 0, 0, 0, clocky.NYC), false}, // early extended close
		{"Thursday July 3rd 2025 7:00 PM", clocky.Date(2025, clocky.July, 3, 19, 0, 0, 0, clocky.NYC), false}, // gap
		{"Friday July 4th 2025 4:00 AM", clocky.Date(2025, clocky.July, 4, 4, 0, 0, 0, clocky.NYC), false},    // holiday
		{"Friday July 4th 2025 12:00 PM", clocky.Date(2025, clocky.July, 4, 12, 0, 0, 0, clocky.NYC), false},  // holiday
		// 2026: July 4 is Saturday, observed Friday July 3 (holiday), July 2 (Thursday) is early close
		{"Thursday July 2nd 2026 4:00 AM", clocky.Date(2026, clocky.July, 2, 4, 0, 0, 0, clocky.NYC), true},
		{"Thursday July 2nd 2026 12:00 PM", clocky.Date(2026, clocky.July, 2, 12, 0, 0, 0, clocky.NYC), true},
		{"Thursday July 2nd 2026 1:00 PM", clocky.Date(2026, clocky.July, 2, 13, 0, 0, 0, clocky.NYC), true},  // market closed but extended open
		{"Thursday July 2nd 2026 4:59 PM", clocky.Date(2026, clocky.July, 2, 16, 59, 0, 0, clocky.NYC), true}, // still extended
		{"Thursday July 2nd 2026 5:00 PM", clocky.Date(2026, clocky.July, 2, 17, 0, 0, 0, clocky.NYC), false}, // early extended close
		{"Thursday July 2nd 2026 7:00 PM", clocky.Date(2026, clocky.July, 2, 19, 0, 0, 0, clocky.NYC), false}, // gap
		{"Friday July 3rd 2026 4:00 AM", clocky.Date(2026, clocky.July, 3, 4, 0, 0, 0, clocky.NYC), false},    // observed holiday
		{"Friday July 3rd 2026 12:00 PM", clocky.Date(2026, clocky.July, 3, 12, 0, 0, 0, clocky.NYC), false},  // observed holiday
		{"Saturday July 4th 2026 9:30 AM", clocky.Date(2026, clocky.July, 4, 9, 30, 0, 0, clocky.NYC), false},
	}
	for _, tt := range tests {
		if got := IsMarketOpenExtended(tt.dt); got != tt.want {
			t.Errorf("%s: IsMarketOpenExtended() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsExtended(t *testing.T) {
	tests := []struct {
		name string
		dt   clocky.Time
		want bool
	}{
		{"Thursday 3:59 AM", clocky.Date(2026, clocky.April, 23, 3, 59, 0, 0, clocky.NYC), false},
		{"Thursday 4:00 AM", clocky.Date(2026, clocky.April, 23, 4, 0, 0, 0, clocky.NYC), true},
		{"Thursday 4:01 AM", clocky.Date(2026, clocky.April, 23, 4, 1, 0, 0, clocky.NYC), true},
		{"Thursday 9:30 AM", clocky.Date(2026, clocky.April, 23, 9, 30, 0, 0, clocky.NYC), false},
		{"Thursday 3:59 PM", clocky.Date(2026, clocky.April, 23, 15, 59, 0, 0, clocky.NYC), false},
		{"Thursday 4:00 PM", clocky.Date(2026, clocky.April, 23, 16, 0, 0, 0, clocky.NYC), true},
		{"Thursday 4:01 PM", clocky.Date(2026, clocky.April, 23, 16, 1, 0, 0, clocky.NYC), true},
		{"Thursday 7:00 PM", clocky.Date(2026, clocky.April, 23, 19, 0, 0, 0, clocky.NYC), true},
		{"Thursday 8:00 PM", clocky.Date(2026, clocky.April, 23, 20, 0, 0, 0, clocky.NYC), false},
		// 2025: July 4 is Friday (holiday), July 3 (Thursday) is early close
		{"Thursday July 3rd 2025 4:00 AM", clocky.Date(2025, clocky.July, 3, 4, 0, 0, 0, clocky.NYC), true},     // pre-market extended
		{"Thursday July 3rd 2025 9:30 AM", clocky.Date(2025, clocky.July, 3, 9, 30, 0, 0, clocky.NYC), false},   // market open, not just extended
		{"Thursday July 3rd 2025 12:59 PM", clocky.Date(2025, clocky.July, 3, 12, 59, 0, 0, clocky.NYC), false}, // market open
		{"Thursday July 3rd 2025 1:00 PM", clocky.Date(2025, clocky.July, 3, 13, 0, 0, 0, clocky.NYC), true},    // market closed, extended only
		{"Thursday July 3rd 2025 4:59 PM", clocky.Date(2025, clocky.July, 3, 16, 59, 0, 0, clocky.NYC), true},   // still extended
		{"Thursday July 3rd 2025 5:00 PM", clocky.Date(2025, clocky.July, 3, 17, 0, 0, 0, clocky.NYC), false},   // early extended close
		{"Friday July 4th 2025 9:30 AM", clocky.Date(2025, clocky.July, 4, 9, 30, 0, 0, clocky.NYC), false},     // holiday
		// 2026: July 4 is Saturday, observed Friday July 3 (holiday), July 2 (Thursday) is early close
		{"Thursday July 2nd 2026 4:00 AM", clocky.Date(2026, clocky.July, 2, 4, 0, 0, 0, clocky.NYC), true},     // pre-market extended
		{"Thursday July 2nd 2026 9:30 AM", clocky.Date(2026, clocky.July, 2, 9, 30, 0, 0, clocky.NYC), false},   // market open
		{"Thursday July 2nd 2026 12:59 PM", clocky.Date(2026, clocky.July, 2, 12, 59, 0, 0, clocky.NYC), false}, // market open
		{"Thursday July 2nd 2026 1:00 PM", clocky.Date(2026, clocky.July, 2, 13, 0, 0, 0, clocky.NYC), true},    // market closed, extended only
		{"Thursday July 2nd 2026 4:59 PM", clocky.Date(2026, clocky.July, 2, 16, 59, 0, 0, clocky.NYC), true},   // still extended
		{"Thursday July 2nd 2026 5:00 PM", clocky.Date(2026, clocky.July, 2, 17, 0, 0, 0, clocky.NYC), false},   // early extended close
		{"Friday July 3rd 2026 9:30 AM", clocky.Date(2026, clocky.July, 3, 9, 30, 0, 0, clocky.NYC), false},     // observed holiday
		{"Saturday July 4th 2026 9:30 AM", clocky.Date(2026, clocky.July, 4, 9, 30, 0, 0, clocky.NYC), false},
	}
	for _, tt := range tests {
		if got := IsExtended(tt.dt); got != tt.want {
			t.Errorf("%s: IsExtended() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsMarketOpenExtendedOvernight(t *testing.T) {
	tests := []struct {
		name string
		dt   clocky.Time
		want bool
	}{
		// normal week
		{"Thursday 3:00 AM", clocky.Date(2026, clocky.April, 23, 3, 0, 0, 0, clocky.NYC), true},    // overnight
		{"Thursday 3:59 AM", clocky.Date(2026, clocky.April, 23, 3, 59, 0, 0, clocky.NYC), true},   // overnight
		{"Thursday 4:00 AM", clocky.Date(2026, clocky.April, 23, 4, 0, 0, 0, clocky.NYC), true},    // extended
		{"Thursday 9:30 AM", clocky.Date(2026, clocky.April, 23, 9, 30, 0, 0, clocky.NYC), true},   // market
		{"Thursday 3:59 PM", clocky.Date(2026, clocky.April, 23, 15, 59, 0, 0, clocky.NYC), true},  // market
		{"Thursday 4:00 PM", clocky.Date(2026, clocky.April, 23, 16, 0, 0, 0, clocky.NYC), true},   // extended
		{"Thursday 7:59 PM", clocky.Date(2026, clocky.April, 23, 19, 59, 0, 0, clocky.NYC), true},  // extended
		{"Thursday 8:00 PM", clocky.Date(2026, clocky.April, 23, 20, 0, 0, 0, clocky.NYC), true},   // overnight
		{"Thursday 11:59 PM", clocky.Date(2026, clocky.April, 23, 23, 59, 0, 0, clocky.NYC), true}, // overnight
		{"Friday 3:59 AM", clocky.Date(2026, clocky.April, 24, 3, 59, 0, 0, clocky.NYC), true},     // overnight
		{"Friday 4:00 AM", clocky.Date(2026, clocky.April, 24, 4, 0, 0, 0, clocky.NYC), true},      // extended
		{"Friday 7:59 PM", clocky.Date(2026, clocky.April, 24, 19, 59, 0, 0, clocky.NYC), true},    // extended
		{"Friday 8:00 PM", clocky.Date(2026, clocky.April, 24, 20, 0, 0, 0, clocky.NYC), false},    // weekend, no overnight
		{"Saturday 12:00 PM", clocky.Date(2026, clocky.April, 25, 12, 0, 0, 0, clocky.NYC), false}, // weekend
		{"Sunday 7:59 PM", clocky.Date(2026, clocky.April, 26, 19, 59, 0, 0, clocky.NYC), false},   // weekend
		{"Sunday 8:00 PM", clocky.Date(2026, clocky.April, 26, 20, 0, 0, 0, clocky.NYC), true},     // overnight resumes
		// 2025: July 4 is Friday (holiday), July 3 (Thursday) is early close
		// the dead zone: 5 PM Thursday through Sunday 8 PM
		{"Wednesday July 2nd 2025 8:00 PM", clocky.Date(2025, clocky.July, 2, 20, 0, 0, 0, clocky.NYC), true},  // overnight into early close day
		{"Thursday July 3rd 2025 3:00 AM", clocky.Date(2025, clocky.July, 3, 3, 0, 0, 0, clocky.NYC), true},    // overnight
		{"Thursday July 3rd 2025 4:00 AM", clocky.Date(2025, clocky.July, 3, 4, 0, 0, 0, clocky.NYC), true},    // extended
		{"Thursday July 3rd 2025 12:00 PM", clocky.Date(2025, clocky.July, 3, 12, 0, 0, 0, clocky.NYC), true},  // market
		{"Thursday July 3rd 2025 1:00 PM", clocky.Date(2025, clocky.July, 3, 13, 0, 0, 0, clocky.NYC), true},   // extended (market early closed)
		{"Thursday July 3rd 2025 4:59 PM", clocky.Date(2025, clocky.July, 3, 16, 59, 0, 0, clocky.NYC), true},  // extended
		{"Thursday July 3rd 2025 5:00 PM", clocky.Date(2025, clocky.July, 3, 17, 0, 0, 0, clocky.NYC), false},  // dead zone starts
		{"Thursday July 3rd 2025 7:00 PM", clocky.Date(2025, clocky.July, 3, 19, 0, 0, 0, clocky.NYC), false},  // dead zone
		{"Thursday July 3rd 2025 8:00 PM", clocky.Date(2025, clocky.July, 3, 20, 0, 0, 0, clocky.NYC), false},  // no overnight (holiday tomorrow)
		{"Thursday July 3rd 2025 10:00 PM", clocky.Date(2025, clocky.July, 3, 22, 0, 0, 0, clocky.NYC), false}, // dead zone
		{"Friday July 4th 2025 3:00 AM", clocky.Date(2025, clocky.July, 4, 3, 0, 0, 0, clocky.NYC), false},     // holiday
		{"Friday July 4th 2025 12:00 PM", clocky.Date(2025, clocky.July, 4, 12, 0, 0, 0, clocky.NYC), false},   // holiday
		{"Friday July 4th 2025 8:00 PM", clocky.Date(2025, clocky.July, 4, 20, 0, 0, 0, clocky.NYC), false},    // no overnight (weekend)
		{"Saturday July 5th 2025 12:00 PM", clocky.Date(2025, clocky.July, 5, 12, 0, 0, 0, clocky.NYC), false}, // weekend
		{"Sunday July 6th 2025 8:00 PM", clocky.Date(2025, clocky.July, 6, 20, 0, 0, 0, clocky.NYC), true},     // overnight resumes
		// 2026: July 4 is Saturday, observed Friday July 3 (holiday), July 2 (Thursday) is early close
		{"Wednesday July 1st 2026 8:00 PM", clocky.Date(2026, clocky.July, 1, 20, 0, 0, 0, clocky.NYC), true}, // overnight into early close day
		{"Thursday July 2nd 2026 3:00 AM", clocky.Date(2026, clocky.July, 2, 3, 0, 0, 0, clocky.NYC), true},   // overnight
		{"Thursday July 2nd 2026 4:00 AM", clocky.Date(2026, clocky.July, 2, 4, 0, 0, 0, clocky.NYC), true},   // extended
		{"Thursday July 2nd 2026 12:00 PM", clocky.Date(2026, clocky.July, 2, 12, 0, 0, 0, clocky.NYC), true}, // market
		{"Thursday July 2nd 2026 1:00 PM", clocky.Date(2026, clocky.July, 2, 13, 0, 0, 0, clocky.NYC), true},  // extended (market early closed)
		{"Thursday July 2nd 2026 4:59 PM", clocky.Date(2026, clocky.July, 2, 16, 59, 0, 0, clocky.NYC), true}, // extended
		{"Thursday July 2nd 2026 5:00 PM", clocky.Date(2026, clocky.July, 2, 17, 0, 0, 0, clocky.NYC), false}, // dead zone starts
		{"Thursday July 2nd 2026 8:00 PM", clocky.Date(2026, clocky.July, 2, 20, 0, 0, 0, clocky.NYC), false}, // no overnight (observed holiday tomorrow)
		{"Friday July 3rd 2026 12:00 PM", clocky.Date(2026, clocky.July, 3, 12, 0, 0, 0, clocky.NYC), false},  // observed holiday
		{"Friday July 3rd 2026 8:00 PM", clocky.Date(2026, clocky.July, 3, 20, 0, 0, 0, clocky.NYC), false},   // no overnight (weekend)
		{"Sunday July 5th 2026 8:00 PM", clocky.Date(2026, clocky.July, 5, 20, 0, 0, 0, clocky.NYC), true},    // overnight resumes
	}
	for _, tt := range tests {
		if got := IsMarketOpenExtendedOvernight(tt.dt); got != tt.want {
			t.Errorf("%s: IsMarketOpenExtendedOvernight() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestGetSession(t *testing.T) {
	tests := []struct {
		name string
		dt   clocky.Time
		want Session
	}{
		// normal Thursday
		{"Thu 3:00 AM", clocky.Date(2026, clocky.April, 23, 3, 0, 0, 0, clocky.NYC), SessionOvernight},
		{"Thu 3:59 AM", clocky.Date(2026, clocky.April, 23, 3, 59, 0, 0, clocky.NYC), SessionOvernight},
		{"Thu 4:00 AM", clocky.Date(2026, clocky.April, 23, 4, 0, 0, 0, clocky.NYC), SessionExtended},
		{"Thu 9:29 AM", clocky.Date(2026, clocky.April, 23, 9, 29, 0, 0, clocky.NYC), SessionExtended},
		{"Thu 9:30 AM", clocky.Date(2026, clocky.April, 23, 9, 30, 0, 0, clocky.NYC), SessionDay},
		{"Thu 3:59 PM", clocky.Date(2026, clocky.April, 23, 15, 59, 0, 0, clocky.NYC), SessionDay},
		{"Thu 4:00 PM", clocky.Date(2026, clocky.April, 23, 16, 0, 0, 0, clocky.NYC), SessionExtended},
		{"Thu 7:59 PM", clocky.Date(2026, clocky.April, 23, 19, 59, 0, 0, clocky.NYC), SessionExtended},
		{"Thu 8:00 PM", clocky.Date(2026, clocky.April, 23, 20, 0, 0, 0, clocky.NYC), SessionOvernight},
		{"Thu 11:59 PM", clocky.Date(2026, clocky.April, 23, 23, 59, 0, 0, clocky.NYC), SessionOvernight},
		// Friday into weekend
		{"Fri 3:59 PM", clocky.Date(2026, clocky.April, 24, 15, 59, 0, 0, clocky.NYC), SessionDay},
		{"Fri 4:00 PM", clocky.Date(2026, clocky.April, 24, 16, 0, 0, 0, clocky.NYC), SessionExtended},
		{"Fri 7:59 PM", clocky.Date(2026, clocky.April, 24, 19, 59, 0, 0, clocky.NYC), SessionExtended},
		{"Fri 8:00 PM", clocky.Date(2026, clocky.April, 24, 20, 0, 0, 0, clocky.NYC), SessionClosed},
		{"Sat 12:00 PM", clocky.Date(2026, clocky.April, 25, 12, 0, 0, 0, clocky.NYC), SessionClosed},
		{"Sun 7:59 PM", clocky.Date(2026, clocky.April, 26, 19, 59, 0, 0, clocky.NYC), SessionClosed},
		{"Sun 8:00 PM", clocky.Date(2026, clocky.April, 26, 20, 0, 0, 0, clocky.NYC), SessionOvernight},
		// early close: 2025 July 3 (Thu), July 4 (Fri) is holiday
		{"Jul3 2025 9:30 AM", clocky.Date(2025, clocky.July, 3, 9, 30, 0, 0, clocky.NYC), SessionDay},
		{"Jul3 2025 12:59 PM", clocky.Date(2025, clocky.July, 3, 12, 59, 0, 0, clocky.NYC), SessionDay},
		{"Jul3 2025 1:00 PM", clocky.Date(2025, clocky.July, 3, 13, 0, 0, 0, clocky.NYC), SessionExtended},
		{"Jul3 2025 4:59 PM", clocky.Date(2025, clocky.July, 3, 16, 59, 0, 0, clocky.NYC), SessionExtended},
		{"Jul3 2025 5:00 PM", clocky.Date(2025, clocky.July, 3, 17, 0, 0, 0, clocky.NYC), SessionClosed},
		{"Jul3 2025 8:00 PM", clocky.Date(2025, clocky.July, 3, 20, 0, 0, 0, clocky.NYC), SessionClosed},
		{"Jul4 2025 3:00 AM", clocky.Date(2025, clocky.July, 4, 3, 0, 0, 0, clocky.NYC), SessionClosed},
		{"Jul4 2025 12:00 PM", clocky.Date(2025, clocky.July, 4, 12, 0, 0, 0, clocky.NYC), SessionClosed},
		// early close: 2026 July 2 (Thu), July 3 (Fri) is observed holiday
		{"Jul2 2026 12:59 PM", clocky.Date(2026, clocky.July, 2, 12, 59, 0, 0, clocky.NYC), SessionDay},
		{"Jul2 2026 1:00 PM", clocky.Date(2026, clocky.July, 2, 13, 0, 0, 0, clocky.NYC), SessionExtended},
		{"Jul2 2026 5:00 PM", clocky.Date(2026, clocky.July, 2, 17, 0, 0, 0, clocky.NYC), SessionClosed},
		{"Jul2 2026 8:00 PM", clocky.Date(2026, clocky.July, 2, 20, 0, 0, 0, clocky.NYC), SessionClosed},
		{"Jul3 2026 12:00 PM", clocky.Date(2026, clocky.July, 3, 12, 0, 0, 0, clocky.NYC), SessionClosed},
	}
	for _, tt := range tests {
		if got := GetSession(tt.dt); got != tt.want {
			t.Errorf("%s: GetSession() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestGetSessionCrossValidate checks GetSession against the individual Is* functions
// for every minute across several interesting weeks.
func TestGetSessionCrossValidate(t *testing.T) {
	// check every minute from Sunday April 19 2026 through Saturday April 25 2026
	// plus the July 4 holiday weeks for 2025 and 2026
	starts := []clocky.Time{
		clocky.Date(2026, clocky.April, 19, 0, 0, 0, 0, clocky.NYC),
		clocky.Date(2025, clocky.July, 1, 0, 0, 0, 0, clocky.NYC),
		clocky.Date(2026, clocky.July, 1, 0, 0, 0, 0, clocky.NYC),
	}
	for _, start := range starts {
		for i := range 7 * 24 * 60 {
			dt := start.Add(clocky.Duration(i) * clocky.Minute)
			got := GetSession(dt)
			isDay := IsMarketOpen(dt)
			isExt := IsExtended(dt)
			isOvn := IsOvernight(dt)
			var want Session
			switch {
			case isDay:
				want = SessionDay
			case isExt:
				want = SessionExtended
			case isOvn:
				want = SessionOvernight
			default:
				want = SessionClosed
			}
			if got != want {
				t.Errorf("%s: GetSession()=%v, want %v (day=%v ext=%v ovn=%v)",
					dt, got, want, isDay, isExt, isOvn)
			}
		}
	}
}

func BenchmarkGetSession(b *testing.B) {
	dt := clocky.Date(2026, clocky.April, 23, 12, 0, 0, 0, clocky.NYC)
	for b.Loop() {
		GetSession(dt)
	}
}

func BenchmarkGetOpenTime(b *testing.B) {
	for b.Loop() {
		GetOpenTime(2026, clocky.April, 23)
	}
}

func BenchmarkGetCloseTime(b *testing.B) {
	for b.Loop() {
		GetCloseTime(2026, clocky.April, 23)
	}
}

func BenchmarkGetCloseTimeEarlyClose(b *testing.B) {
	for b.Loop() {
		GetCloseTime(2025, clocky.July, 3)
	}
}

func BenchmarkGetOpenTimeExtended(b *testing.B) {
	for b.Loop() {
		GetOpenTimeExtended(2026, clocky.April, 23)
	}
}

func BenchmarkGetCloseTimeExtended(b *testing.B) {
	for b.Loop() {
		GetCloseTimeExtended(2026, clocky.April, 23)
	}
}

func BenchmarkIsTradingDay(b *testing.B) {
	for b.Loop() {
		IsTradingDay(2026, clocky.April, 23)
	}
}

func BenchmarkIsTradingDayHoliday(b *testing.B) {
	for b.Loop() {
		IsTradingDay(2026, clocky.July, 4)
	}
}

func BenchmarkIsHoliday(b *testing.B) {
	for b.Loop() {
		IsHoliday(2026, clocky.July, 4)
	}
}

func BenchmarkIsEarlyCloseDay(b *testing.B) {
	for b.Loop() {
		IsEarlyCloseDay(2025, clocky.July, 3)
	}
}

func BenchmarkIsMarketOpen(b *testing.B) {
	dt := clocky.Date(2026, clocky.April, 23, 12, 0, 0, 0, clocky.NYC)
	for b.Loop() {
		IsMarketOpen(dt)
	}
}

func BenchmarkIsMarketOpenExtended(b *testing.B) {
	dt := clocky.Date(2026, clocky.April, 23, 17, 0, 0, 0, clocky.NYC)
	for b.Loop() {
		IsMarketOpenExtended(dt)
	}
}

func BenchmarkIsMarketOpenExtendedOvernight(b *testing.B) {
	dt := clocky.Date(2026, clocky.April, 23, 22, 0, 0, 0, clocky.NYC)
	for b.Loop() {
		IsMarketOpenExtendedOvernight(dt)
	}
}

func BenchmarkIsExtended(b *testing.B) {
	dt := clocky.Date(2026, clocky.April, 23, 17, 0, 0, 0, clocky.NYC)
	for b.Loop() {
		IsExtended(dt)
	}
}

func BenchmarkIsOvernight(b *testing.B) {
	dt := clocky.Date(2026, clocky.April, 23, 22, 0, 0, 0, clocky.NYC)
	for b.Loop() {
		IsOvernight(dt)
	}
}
