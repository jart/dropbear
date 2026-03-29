package cboe

import (
	"dropbear/clocky"
	"dropbear/greg"
	"fmt"
	"testing"
)

func TestIsHoliday_2026_CBOE(t *testing.T) {
	// From CBOE 2026 holiday calendar (the user's original data)
	holidays := []struct {
		name  string
		year  int
		month clocky.Month
		day   int
	}{
		{"New Year's Day", 2026, clocky.January, 1},
		{"MLK Day", 2026, clocky.January, 19},
		{"Presidents' Day", 2026, clocky.February, 16},
		{"Good Friday", 2026, clocky.April, 3},
		{"Memorial Day", 2026, clocky.May, 25},
		{"Juneteenth", 2026, clocky.June, 19},
		{"Independence Day Observed", 2026, clocky.July, 3}, // July 4 is Saturday
		{"Labor Day", 2026, clocky.September, 7},
		{"Thanksgiving", 2026, clocky.November, 26},
		{"Christmas", 2026, clocky.December, 25},
	}

	for _, h := range holidays {
		if !IsHoliday(h.year, h.month, h.day) {
			t.Errorf("%s (%d-%02d-%02d) should be a holiday", h.name, h.year, h.month, h.day)
		}
	}
}

func TestIsEarlyClose_2026_CBOE(t *testing.T) {
	// From CBOE 2026 early close calendar
	earlycloses := []struct {
		name  string
		year  int
		month clocky.Month
		day   int
	}{
		{"Day before Independence Day", 2026, clocky.July, 2}, // July 4 is Sat, so July 3 is holiday, July 2 early close
		{"Day after Thanksgiving", 2026, clocky.November, 27},
		{"Christmas Eve", 2026, clocky.December, 24},
	}

	for _, ec := range earlycloses {
		if !IsEarlyCloseDay(ec.year, ec.month, ec.day) {
			t.Errorf("%s (%d-%02d-%02d) should be an early close day", ec.name, ec.year, ec.month, ec.day)
		}
	}
}

func TestIsHoliday_MultiYear(t *testing.T) {
	// Test holidays across multiple years to catch edge cases
	tests := []struct {
		name  string
		year  int
		month clocky.Month
		day   int
	}{
		// 2020
		{"New Year's Day 2020", 2020, clocky.January, 1},
		{"MLK Day 2020", 2020, clocky.January, 20},
		{"Presidents' Day 2020", 2020, clocky.February, 17},
		{"Good Friday 2020", 2020, clocky.April, 10},
		{"Memorial Day 2020", 2020, clocky.May, 25},
		{"Independence Day Observed 2020", 2020, clocky.July, 3}, // July 4 is Saturday
		{"Labor Day 2020", 2020, clocky.September, 7},
		{"Thanksgiving 2020", 2020, clocky.November, 26},
		{"Christmas 2020", 2020, clocky.December, 25},

		// 2021
		{"New Year's Day 2021", 2021, clocky.January, 1},
		{"MLK Day 2021", 2021, clocky.January, 18},
		{"Presidents' Day 2021", 2021, clocky.February, 15},
		{"Good Friday 2021", 2021, clocky.April, 2},
		{"Memorial Day 2021", 2021, clocky.May, 31},
		{"Independence Day Observed 2021", 2021, clocky.July, 5}, // July 4 is Sunday
		{"Labor Day 2021", 2021, clocky.September, 6},
		{"Thanksgiving 2021", 2021, clocky.November, 25},
		{"Christmas Observed 2021", 2021, clocky.December, 24}, // Dec 25 is Saturday

		// 2022 (first year with Juneteenth)
		{"MLK Day 2022", 2022, clocky.January, 17},
		{"Presidents' Day 2022", 2022, clocky.February, 21},
		{"Good Friday 2022", 2022, clocky.April, 15},
		{"Memorial Day 2022", 2022, clocky.May, 30},
		{"Juneteenth Observed 2022", 2022, clocky.June, 20}, // June 19 is Sunday
		{"Independence Day 2022", 2022, clocky.July, 4},
		{"Labor Day 2022", 2022, clocky.September, 5},
		{"Thanksgiving 2022", 2022, clocky.November, 24},
		{"Christmas Observed 2022", 2022, clocky.December, 26}, // Dec 25 is Sunday

		// 2023
		{"New Year's Day Observed 2023", 2023, clocky.January, 2}, // Jan 1 is Sunday
		{"MLK Day 2023", 2023, clocky.January, 16},
		{"Presidents' Day 2023", 2023, clocky.February, 20},
		{"Good Friday 2023", 2023, clocky.April, 7},
		{"Memorial Day 2023", 2023, clocky.May, 29},
		{"Juneteenth 2023", 2023, clocky.June, 19},
		{"Independence Day 2023", 2023, clocky.July, 4},
		{"Labor Day 2023", 2023, clocky.September, 4},
		{"Thanksgiving 2023", 2023, clocky.November, 23},
		{"Christmas 2023", 2023, clocky.December, 25},

		// 2024
		{"New Year's Day 2024", 2024, clocky.January, 1},
		{"MLK Day 2024", 2024, clocky.January, 15},
		{"Presidents' Day 2024", 2024, clocky.February, 19},
		{"Good Friday 2024", 2024, clocky.March, 29},
		{"Memorial Day 2024", 2024, clocky.May, 27},
		{"Juneteenth 2024", 2024, clocky.June, 19},
		{"Independence Day 2024", 2024, clocky.July, 4},
		{"Labor Day 2024", 2024, clocky.September, 2},
		{"Thanksgiving 2024", 2024, clocky.November, 28},
		{"Christmas 2024", 2024, clocky.December, 25},

		// 2025
		{"New Year's Day 2025", 2025, clocky.January, 1},
		{"MLK Day 2025", 2025, clocky.January, 20},
		{"Presidents' Day 2025", 2025, clocky.February, 17},
		{"Good Friday 2025", 2025, clocky.April, 18},
		{"Memorial Day 2025", 2025, clocky.May, 26},
		{"Juneteenth 2025", 2025, clocky.June, 19},
		{"Independence Day 2025", 2025, clocky.July, 4},
		{"Labor Day 2025", 2025, clocky.September, 1},
		{"Thanksgiving 2025", 2025, clocky.November, 27},
		{"Christmas 2025", 2025, clocky.December, 25},
	}

	for _, tt := range tests {
		if !IsHoliday(tt.year, tt.month, tt.day) {
			t.Errorf("%s (%d-%02d-%02d) should be a holiday", tt.name, tt.year, tt.month, tt.day)
		}
	}
}

func TestIsHoliday_NotHoliday(t *testing.T) {
	// Test that regular days are not marked as holidays
	tests := []struct {
		year  int
		month clocky.Month
		day   int
	}{
		{2026, clocky.January, 2},  // Regular Friday
		{2026, clocky.January, 5},  // Regular Monday
		{2026, clocky.March, 15},   // Random Wednesday
		{2026, clocky.July, 15},    // Regular Wednesday
		{2026, clocky.October, 15}, // Regular Thursday
	}

	for _, tt := range tests {
		if IsHoliday(tt.year, tt.month, tt.day) {
			t.Errorf("%d-%02d-%02d should NOT be a holiday", tt.year, tt.month, tt.day)
		}
	}
}

func TestIsEarlyClose_MultiYear(t *testing.T) {
	tests := []struct {
		name  string
		year  int
		month clocky.Month
		day   int
		want  bool
	}{
		// July 3 early closes (depends on July 4 day of week)
		{"2020 July 4 Sat -> July 2 early close", 2020, clocky.July, 2, true},
		{"2021 July 4 Sun -> July 2 early close", 2021, clocky.July, 2, true},
		{"2022 July 4 Mon -> July 1 early close", 2022, clocky.July, 1, true},
		{"2023 July 3 regular early close", 2023, clocky.July, 3, true},
		{"2024 July 3 regular early close", 2024, clocky.July, 3, true},
		{"2025 July 3 regular early close", 2025, clocky.July, 3, true},
		{"2026 July 4 Sat -> July 2 early close", 2026, clocky.July, 2, true},
		{"2027 July 4 Sun -> July 2 early close", 2027, clocky.July, 2, true},

		// Day after Thanksgiving
		{"2020 day after Thanksgiving", 2020, clocky.November, 27, true},
		{"2021 day after Thanksgiving", 2021, clocky.November, 26, true},
		{"2022 day after Thanksgiving", 2022, clocky.November, 25, true},
		{"2023 day after Thanksgiving", 2023, clocky.November, 24, true},
		{"2024 day after Thanksgiving", 2024, clocky.November, 29, true},
		{"2025 day after Thanksgiving", 2025, clocky.November, 28, true},
		{"2026 day after Thanksgiving", 2026, clocky.November, 27, true},

		// Christmas Eve (depends on Christmas day of week)
		{"2020 Dec 24 regular", 2020, clocky.December, 24, true},
		{"2021 Dec 25 Sat -> Dec 23 early close", 2021, clocky.December, 23, true},
		{"2022 Dec 25 Sun -> Dec 23 early close", 2022, clocky.December, 23, true},
		{"2023 Dec 25 Mon -> Dec 22 early close", 2023, clocky.December, 22, true},
		{"2024 Dec 24 regular", 2024, clocky.December, 24, true},
		{"2025 Dec 24 regular", 2025, clocky.December, 24, true},
		{"2026 Dec 24 regular", 2026, clocky.December, 24, true},

		// Not early close days
		{"Random day not early close", 2026, clocky.March, 15, false},
		{"Thanksgiving itself not early close", 2026, clocky.November, 26, false},
	}

	for _, tt := range tests {
		got := IsEarlyCloseDay(tt.year, tt.month, tt.day)
		if got != tt.want {
			t.Errorf("%s: IsEarlyCloseDay(%d, %v, %d) = %v, want %v",
				tt.name, tt.year, tt.month, tt.day, got, tt.want)
		}
	}
}

func TestJTBeforeObservance(t *testing.T) {
	// JT was not observed by NYSE before 2022
	if IsHoliday(2021, clocky.June, 18) {
		t.Error("JT 2021 (observed June 18) should NOT be a holiday - NYSE didn't observe until 2022")
	}
	if IsHoliday(2020, clocky.June, 19) {
		t.Error("JT 2020 should NOT be a holiday - NYSE didn't observe until 2022")
	}
}

func TestHolidayCount(t *testing.T) {
	// Count holidays by iterating every day and verify we get the expected count
	// NYSE holidays: NYD, MLK, Presidents, Good Friday, Memorial, JT (2022+),
	//                Independence, Labor, Thanksgiving, Christmas
	// Note: When Jan 1 is Saturday, NYSE has no Friday observance (loses 1 holiday)
	// Note: Unscheduled closures add extra holidays (2018 Bush funeral)
	expectedHolidays := map[int]int{
		2016: 9,  // No JT, Jan 1 = Fri
		2017: 9,  // No JT, Jan 1 = Sun (observed Mon)
		2018: 10, // No JT, Jan 1 = Mon, +1 for Bush funeral (Dec 5)
		2019: 9,  // No JT, Jan 1 = Tue
		2020: 9,  // No JT, Jan 1 = Wed
		2021: 9,  // No JT, Jan 1 = Fri
		2022: 9,  // Has JT, but Jan 1 = Sat (no observance)
		2023: 10, // Has JT, Jan 1 = Sun (observed Mon)
		2024: 10, // Has JT, Jan 1 = Mon
		2025: 10, // Has JT, Jan 1 = Wed
		2026: 10, // Has JT, Jan 1 = Thu
		2027: 10, // Has JT, Jan 1 = Fri
		2028: 9,  // Has JT, but Jan 1 = Sat (no observance)
		2029: 10, // Has JT, Jan 1 = Mon
		2030: 10, // Has JT, Jan 1 = Tue
	}
	for year := 2016; year <= 2030; year++ {
		count := 0
		var holidays []string
		for month := clocky.January; month <= clocky.December; month++ {
			days := greg.DaysIn(year, month)
			for day := 1; day <= days; day++ {
				if IsHoliday(year, month, day) {
					count++
					holidays = append(holidays, fmt.Sprintf("%d-%02d-%02d", year, month, day))
				}
			}
		}
		expected := expectedHolidays[year]
		if count != expected {
			t.Errorf("Year %d: got %d holidays, want %d\nHolidays: %v", year, count, expected, holidays)
		}
	}
}

func TestEarlyCloseCount(t *testing.T) {
	// Count early close days - should be exactly 3 per year:
	// 1. Day before Independence Day
	// 2. Day after Thanksgiving
	// 3. Christmas Eve (adjusted)
	for year := 2016; year <= 2030; year++ {
		count := 0
		var earlycloses []string
		for month := clocky.January; month <= clocky.December; month++ {
			days := greg.DaysIn(year, month)
			for day := 1; day <= days; day++ {
				if IsEarlyCloseDay(year, month, day) {
					count++
					earlycloses = append(earlycloses, fmt.Sprintf("%d-%02d-%02d", year, month, day))
				}
			}
		}
		if count != 3 {
			t.Errorf("Year %d: got %d early close days, want 3\nEarly closes: %v", year, count, earlycloses)
		}
	}
}
