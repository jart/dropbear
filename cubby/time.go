package cubby

import (
	"dropbear/clocky"
)

// GetOpenTime returns the market open time for the given date as clocky.Time.
func GetOpenTime(dt clocky.Time) clocky.Time {
	year, m, day := dt.Date()
	return clocky.Date(year, clocky.Month(m), day, 6, 30, 0, 0)
}

// GetCloseTime returns the market close time for the given date.
func GetCloseTime(dt clocky.Time) clocky.Time {
	year, month, day := dt.Date()
	if isEarlyClose(year, month, day) {
		return clocky.Date(year, month, day, 10, 0, 0, 0)
	}
	return clocky.Date(year, month, day, 13, 0, 0, 0)
}

// IsTradingDay returns true if the given date is a trading day.
func IsTradingDay(dt clocky.Time) bool {
	year, m, day := dt.Date()
	month := clocky.Month(m)
	wd := dt.Weekday()
	if wd == clocky.Saturday || wd == clocky.Sunday {
		return false
	}
	return !isHoliday(year, month, day)
}

// Unscheduled market closures (disasters, national mourning, etc.)
// These cannot be predicted algorithmically.
var unscheduledClosures = map[int]bool{
	// September 11, 2001 terrorist attacks
	20010911: true,
	20010912: true,
	20010913: true,
	20010914: true,
	// Hurricane Sandy
	20121029: true,
	20121030: true,
	// National Day of Mourning for George H.W. Bush
	20181205: true,
}

// isWeekend returns true if the given date is a weekend.
func isWeekend(year int, month clocky.Month, day int) bool {
	wd := weekday(year, month, day)
	return wd == clocky.Saturday || wd == clocky.Sunday
}

// isHoliday returns true if the given date is a US stock market holiday.
// This covers NYSE/NASDAQ holidays which are identical.
func isHoliday(year int, month clocky.Month, day int) bool {
	// Check unscheduled closures first
	dateInt := year*10000 + int(month)*100 + day
	if unscheduledClosures[dateInt] {
		return true
	}

	wd := weekday(year, month, day)

	// New Year's Day: January 1 (observed Monday if Sunday, no observance if Saturday)
	if month == clocky.January {
		jan1Wd := weekday(year, clocky.January, 1)
		if day == 1 && jan1Wd != clocky.Saturday && jan1Wd != clocky.Sunday {
			return true
		}
		if day == 2 && jan1Wd == clocky.Sunday {
			return true
		}
	}

	// MLK Day: 3rd Monday of January
	if month == clocky.January && wd == clocky.Monday && nthWeekday(year, clocky.January, clocky.Monday, 3) == day {
		return true
	}

	// Presidents' Day: 3rd Monday of February
	if month == clocky.February && wd == clocky.Monday && nthWeekday(year, clocky.February, clocky.Monday, 3) == day {
		return true
	}

	// Good Friday: Friday before Easter Sunday
	if month == clocky.March || month == clocky.April {
		gfMonth, gfDay := goodFriday(year)
		if month == gfMonth && day == gfDay {
			return true
		}
	}

	// Memorial Day: Last Monday of May
	if month == clocky.May && wd == clocky.Monday && lastWeekday(year, clocky.May, clocky.Monday) == day {
		return true
	}

	// JT: June 19 (observed Friday if Saturday, Monday if Sunday)
	// Note: NYSE observed starting 2022
	if year >= 2022 && month == clocky.June {
		jtWd := weekday(year, clocky.June, 19)
		observed := 19
		switch jtWd {
		case clocky.Saturday:
			observed = 18
		case clocky.Sunday:
			observed = 20
		}
		if day == observed {
			return true
		}
	}

	// Independence Day: July 4 (observed Friday if Saturday, Monday if Sunday)
	if month == clocky.July {
		jul4Wd := weekday(year, clocky.July, 4)
		observed := 4
		switch jul4Wd {
		case clocky.Saturday:
			observed = 3
		case clocky.Sunday:
			observed = 5
		}
		if day == observed {
			return true
		}
	}

	// Labor Day: 1st Monday of September
	if month == clocky.September && wd == clocky.Monday && nthWeekday(year, clocky.September, clocky.Monday, 1) == day {
		return true
	}

	// Thanksgiving: 4th Thursday of November
	if month == clocky.November && wd == clocky.Thursday && nthWeekday(year, clocky.November, clocky.Thursday, 4) == day {
		return true
	}

	// Christmas: December 25 (observed Friday if Saturday, Monday if Sunday)
	if month == clocky.December {
		xmasWd := weekday(year, clocky.December, 25)
		observed := 25
		switch xmasWd {
		case clocky.Saturday:
			observed = 24
		case clocky.Sunday:
			observed = 26
		}
		if day == observed {
			return true
		}
	}

	return false
}

// isEarlyClose returns true if the given date is a 1pm ET close day.
// Early close days: day before Independence Day, day after Thanksgiving, Christmas Eve.
func isEarlyClose(year int, month clocky.Month, day int) bool {
	wd := weekday(year, month, day)
	if wd == clocky.Saturday || wd == clocky.Sunday {
		return false
	}

	// Day before Independence Day (July 3rd, unless July 4 is Mon/Sat/Sun)
	if month == clocky.July {
		jul4Wd := weekday(year, clocky.July, 4)
		switch jul4Wd {
		case clocky.Monday:
			// July 4 is Monday, July 3 is Sunday - early close on Friday July 1
			if day == 1 && wd == clocky.Friday {
				return true
			}
		case clocky.Saturday:
			// July 4 is Saturday (observed Fri Jul 3) - early close on Thursday July 2
			if day == 2 && wd == clocky.Thursday {
				return true
			}
		case clocky.Sunday:
			// July 4 is Sunday (observed Mon Jul 5) - early close on Friday July 2
			if day == 2 && wd == clocky.Friday {
				return true
			}
		default:
			// Normal case: early close on July 3
			if day == 3 {
				return true
			}
		}
	}

	// Day after Thanksgiving (4th Friday of November)
	if month == clocky.November && wd == clocky.Friday {
		thanksgiving := nthWeekday(year, clocky.November, clocky.Thursday, 4)
		if day == thanksgiving+1 {
			return true
		}
	}

	// Christmas Eve (December 24, adjusted for weekends)
	if month == clocky.December {
		xmasWd := weekday(year, clocky.December, 25)
		switch xmasWd {
		case clocky.Monday:
			// Christmas is Monday, Dec 24 is Sunday - early close on Friday Dec 22
			if day == 22 && wd == clocky.Friday {
				return true
			}
		case clocky.Saturday:
			// Christmas is Saturday (observed Fri Dec 24) - early close on Thursday Dec 23
			if day == 23 && wd == clocky.Thursday {
				return true
			}
		case clocky.Sunday:
			// Christmas is Sunday (observed Mon Dec 26) - Dec 24 is Saturday, early close on Friday Dec 23
			if day == 23 && wd == clocky.Friday {
				return true
			}
		default:
			// Normal case: early close on Dec 24
			if day == 24 {
				return true
			}
		}
	}

	return false
}

// weekday returns the day of the week for a given date.
// Uses Zeller's congruence for the Gregorian calendar.
func weekday(year int, month clocky.Month, day int) clocky.Weekday {
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

// nthWeekday returns the day of month for the nth occurrence of a weekday.
func nthWeekday(year int, month clocky.Month, targetWd clocky.Weekday, n int) int {
	firstWd := weekday(year, month, 1)
	daysUntil := (int(targetWd) - int(firstWd) + 7) % 7
	return 1 + daysUntil + (n-1)*7
}

// lastWeekday returns the day of month for the last occurrence of a weekday.
func lastWeekday(year int, month clocky.Month, targetWd clocky.Weekday) int {
	daysInMonth := daysIn(year, month)
	lastWd := weekday(year, month, daysInMonth)
	daysBack := (int(lastWd) - int(targetWd) + 7) % 7
	return daysInMonth - daysBack
}

// daysIn returns the number of days in a month.
func daysIn(year int, month clocky.Month) int {
	switch month {
	case clocky.January, clocky.March, clocky.May, clocky.July, clocky.August, clocky.October, clocky.December:
		return 31
	case clocky.April, clocky.June, clocky.September, clocky.November:
		return 30
	case clocky.February:
		if isLeapYear(year) {
			return 29
		}
		return 28
	}
	return 0
}

// isLeapYear returns true if year is a leap year.
func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// goodFriday returns the month and day of Good Friday for a given year.
func goodFriday(year int) (clocky.Month, int) {
	month, day := easterSunday(year)
	day -= 2
	if day < 1 {
		month--
		day += daysIn(year, month)
	}
	return month, day
}

// easterSunday returns the month and day of Easter Sunday for a given year.
// Uses the Anonymous Gregorian algorithm (Computus).
func easterSunday(year int) (clocky.Month, int) {
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
