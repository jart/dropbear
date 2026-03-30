package cboe

import (
	"dropbear/clocky"
	"dropbear/greg"
)

// IsMarketOpen returns true if the market is open at the given time.
func IsMarketOpen(dt clocky.Time) bool {
	year, month, day := dt.Date()
	openTime := GetOpenTime(year, month, day)
	closeTime := GetCloseTime(year, month, day)
	return !dt.Before(openTime) && dt.Before(closeTime) && IsTradingDay(year, month, day)
}

// GetOpenTime returns the market open time for given date.
func GetOpenTime(year int, month clocky.Month, day int) clocky.Time {
	return clocky.Date(year, month, day, 9, 30, 0, 0, clocky.NYC)
}

// GetCloseTime returns the market close time for given date.
func GetCloseTime(year int, month clocky.Month, day int) clocky.Time {
	if IsEarlyCloseDay(year, month, day) {
		return clocky.Date(year, month, day, 13, 0, 0, 0, clocky.NYC)
	}
	return clocky.Date(year, month, day, 16, 0, 0, 0, clocky.NYC)
}

// IsTradingDay returns true if given date is a trading day.
func IsTradingDay(year int, month clocky.Month, day int) bool {
	wd := greg.Weekday(year, month, day)
	if wd == clocky.Saturday || wd == clocky.Sunday {
		return false
	}
	return !IsHoliday(year, month, day)
}
