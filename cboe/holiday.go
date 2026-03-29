package cboe

import (
	"dropbear/clocky"
	"dropbear/greg"
)

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

// IsHoliday returns true if the given date is a US stock market holiday.
// This covers NYSE/NASDAQ holidays which are identical.
func IsHoliday(year int, month clocky.Month, day int) bool {
	// Check unscheduled closures first
	dateInt := year*10000 + int(month)*100 + day
	if unscheduledClosures[dateInt] {
		return true
	}

	wd := greg.Weekday(year, month, day)

	// New Year's Day: January 1 (observed Monday if Sunday, no observance if Saturday)
	if month == clocky.January {
		jan1Wd := greg.Weekday(year, clocky.January, 1)
		if day == 1 && jan1Wd != clocky.Saturday && jan1Wd != clocky.Sunday {
			return true
		}
		if day == 2 && jan1Wd == clocky.Sunday {
			return true
		}
	}

	// MLK Day: 3rd Monday of January
	if month == clocky.January && wd == clocky.Monday && greg.NthWeekday(year, clocky.January, clocky.Monday, 3) == day {
		return true
	}

	// Presidents' Day: 3rd Monday of February
	if month == clocky.February && wd == clocky.Monday && greg.NthWeekday(year, clocky.February, clocky.Monday, 3) == day {
		return true
	}

	// Good Friday: Friday before Easter Sunday
	if month == clocky.March || month == clocky.April {
		gfMonth, gfDay := greg.GoodFriday(year)
		if month == gfMonth && day == gfDay {
			return true
		}
	}

	// Memorial Day: Last Monday of May
	if month == clocky.May && wd == clocky.Monday && greg.LastWeekday(year, clocky.May, clocky.Monday) == day {
		return true
	}

	// JT: June 19 (observed Friday if Saturday, Monday if Sunday)
	// Note: NYSE observed starting 2022
	if year >= 2022 && month == clocky.June {
		jtWd := greg.Weekday(year, clocky.June, 19)
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
		jul4Wd := greg.Weekday(year, clocky.July, 4)
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
	if month == clocky.September && wd == clocky.Monday && greg.NthWeekday(year, clocky.September, clocky.Monday, 1) == day {
		return true
	}

	// Thanksgiving: 4th Thursday of November
	if month == clocky.November && wd == clocky.Thursday && greg.NthWeekday(year, clocky.November, clocky.Thursday, 4) == day {
		return true
	}

	// Christmas: December 25 (observed Friday if Saturday, Monday if Sunday)
	if month == clocky.December {
		xmasWd := greg.Weekday(year, clocky.December, 25)
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

// IsEarlyCloseDay returns true if the given date is a 1pm ET close day.
// Early close days: day before Independence Day, day after Thanksgiving, Christmas Eve.
func IsEarlyCloseDay(year int, month clocky.Month, day int) bool {
	wd := greg.Weekday(year, month, day)
	if wd == clocky.Saturday || wd == clocky.Sunday {
		return false
	}

	// Day before Independence Day (July 3rd, unless July 4 is Mon/Sat/Sun)
	if month == clocky.July {
		jul4Wd := greg.Weekday(year, clocky.July, 4)
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
		thanksgiving := greg.NthWeekday(year, clocky.November, clocky.Thursday, 4)
		if day == thanksgiving+1 {
			return true
		}
	}

	// Christmas Eve (December 24, adjusted for weekends)
	if month == clocky.December {
		xmasWd := greg.Weekday(year, clocky.December, 25)
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
