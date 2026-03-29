package greg

import "dropbear/clocky"

// EasterSunday returns the month and day of Easter Sunday for a given year.
// Uses the Anonymous Gregorian algorithm (Computus).
func EasterSunday(year int) (clocky.Month, int) {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := ((h + l - 7*m + 114) % 31) + 1
	return clocky.Month(month), day
}

// GoodFriday returns the month and day of Good Friday for a given year.
func GoodFriday(year int) (clocky.Month, int) {
	month, day := EasterSunday(year)
	day -= 2
	if day < 1 {
		month--
		day += DaysIn(year, month)
	}
	return month, day
}
