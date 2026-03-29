package greg

import (
	"dropbear/clocky"
)

// Weekday returns the day of the week for a given date.
// Uses Zeller's congruence for the Gregorian calendar.
func Weekday(year int, month clocky.Month, day int) clocky.Weekday {
	m := int(month)
	y := year
	if m < 3 {
		m += 12
		y--
	}
	q := day
	k := y % 100
	j := y / 100
	h := (q + (13*(m+1))/5 + k + k/4 + j/4 - 2*j) % 7
	return clocky.Weekday((h + 6) % 7)
}

// NthWeekday returns the day of month for the nth occurrence of a weekday.
func NthWeekday(year int, month clocky.Month, targetWd clocky.Weekday, n int) int {
	firstWd := Weekday(year, month, 1)
	daysUntil := (int(targetWd) - int(firstWd) + 7) % 7
	return 1 + daysUntil + (n-1)*7
}

// LastWeekday returns the day of month for the last occurrence of a weekday.
func LastWeekday(year int, month clocky.Month, targetWd clocky.Weekday) int {
	daysInMonth := DaysIn(year, month)
	lastWd := Weekday(year, month, daysInMonth)
	daysBack := (int(lastWd) - int(targetWd) + 7) % 7
	return daysInMonth - daysBack
}

// IsWeekend returns true if the given date is a weekend.
func IsWeekend(year int, month clocky.Month, day int) bool {
	wd := Weekday(year, month, day)
	return wd == clocky.Saturday || wd == clocky.Sunday
}
