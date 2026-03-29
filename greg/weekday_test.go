package greg

import (
	"dropbear/clocky"
	"testing"
)

func TestWeekday(t *testing.T) {
	tests := []struct {
		year  int
		month clocky.Month
		day   int
		want  clocky.Weekday
	}{
		{2026, clocky.January, 1, clocky.Thursday},
		{2026, clocky.January, 19, clocky.Monday}, // MLK
		{2026, clocky.July, 4, clocky.Saturday},
		{2026, clocky.December, 25, clocky.Friday},
		{2024, clocky.February, 29, clocky.Thursday}, // Leap day
	}
	for _, tt := range tests {
		got := Weekday(tt.year, tt.month, tt.day)
		if got != tt.want {
			t.Errorf("weekday(%d, %v, %d) = %d, want %d",
				tt.year, tt.month, tt.day, got, tt.want)
		}
	}
}

func TestNthWeekday(t *testing.T) {
	tests := []struct {
		year    int
		month   clocky.Month
		weekday clocky.Weekday
		n       int
		want    int
	}{
		// MLK Day: 3rd Monday of January
		{2026, clocky.January, clocky.Monday, 3, 19},
		{2025, clocky.January, clocky.Monday, 3, 20},
		{2024, clocky.January, clocky.Monday, 3, 15},

		// Presidents' Day: 3rd Monday of February
		{2026, clocky.February, clocky.Monday, 3, 16},
		{2025, clocky.February, clocky.Monday, 3, 17},

		// Labor Day: 1st Monday of September
		{2026, clocky.September, clocky.Monday, 1, 7},
		{2025, clocky.September, clocky.Monday, 1, 1},

		// Thanksgiving: 4th Thursday of November
		{2026, clocky.November, clocky.Thursday, 4, 26},
		{2025, clocky.November, clocky.Thursday, 4, 27},
	}
	for _, tt := range tests {
		got := NthWeekday(tt.year, tt.month, tt.weekday, tt.n)
		if got != tt.want {
			t.Errorf("NthWeekday(%d, %v, %d, %d) = %d, want %d",
				tt.year, tt.month, tt.weekday, tt.n, got, tt.want)
		}
	}
}

func TestLastWeekday(t *testing.T) {
	tests := []struct {
		year    int
		month   clocky.Month
		weekday clocky.Weekday
		want    int
	}{
		// Memorial Day: Last Monday of May
		{2026, clocky.May, clocky.Monday, 25},
		{2025, clocky.May, clocky.Monday, 26},
		{2024, clocky.May, clocky.Monday, 27},
		{2023, clocky.May, clocky.Monday, 29},
	}
	for _, tt := range tests {
		got := LastWeekday(tt.year, tt.month, tt.weekday)
		if got != tt.want {
			t.Errorf("LastWeekday(%d, %v, %d) = %d, want %d",
				tt.year, tt.month, tt.weekday, got, tt.want)
		}
	}
}
