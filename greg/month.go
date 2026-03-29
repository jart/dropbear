package greg

import "dropbear/clocky"

// DaysIn returns the number of days in a month.
func DaysIn(year int, month clocky.Month) int {
	switch month {
	case clocky.January, clocky.March, clocky.May, clocky.July, clocky.August, clocky.October, clocky.December:
		return 31
	case clocky.April, clocky.June, clocky.September, clocky.November:
		return 30
	case clocky.February:
		if IsLeapYear(year) {
			return 29
		}
		return 28
	}
	return 0
}
