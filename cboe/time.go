package cboe

import (
	"dropbear/clocky"
	"dropbear/greg"
)

// GetOpenTime returns the market open time for the given date as clocky.Time.
func GetOpenTime(dt clocky.Time) clocky.Time {
	year, month, day := dt.Date()
	return getOpenTime(year, month, day)
}

// GetCloseTime returns the market close time for the given date.
func GetCloseTime(dt clocky.Time) clocky.Time {
	year, month, day := dt.Date()
	return getCloseTime(year, month, day)
}

// IsTradingDay returns true if the given date is a trading day.
func IsTradingDay(dt clocky.Time) bool {
	year, month, day := dt.Date()
	return isTradingDay(year, month, day)
}

// IsMarketOpen returns true if the market is open at the given time.
func IsMarketOpen(dt clocky.Time) bool {
	year, month, day := dt.Date()
	openTime := getOpenTime(year, month, day)
	closeTime := getCloseTime(year, month, day)
	return !dt.Before(openTime) && dt.Before(closeTime) && isTradingDay(year, month, day)
}

func getOpenTime(year int, month clocky.Month, day int) clocky.Time {
	return clocky.Date(year, month, day, 9, 30, 0, 0, clocky.NYC)
}

func getCloseTime(year int, month clocky.Month, day int) clocky.Time {
	if IsEarlyCloseDay(year, month, day) {
		return clocky.Date(year, month, day, 13, 0, 0, 0, clocky.NYC)
	}
	return clocky.Date(year, month, day, 16, 0, 0, 0, clocky.NYC)
}

func isTradingDay(year int, month clocky.Month, day int) bool {
	wd := greg.Weekday(year, month, day)
	if wd == clocky.Saturday || wd == clocky.Sunday {
		return false
	}
	return !IsHoliday(year, month, day)
}
