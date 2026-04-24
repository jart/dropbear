package cboe

import (
	"dropbear/clocky"
	"dropbear/greg"
)

// GetOpenTimeExtended returns the early hours open time for given date.
func GetOpenTimeExtended(year int, month clocky.Month, day int) clocky.Time {
	return clocky.Date(year, month, day, 4, 0, 0, 0, clocky.NYC)
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

// GetCloseTimeExtended returns the early hours close time for given date.
func GetCloseTimeExtended(year int, month clocky.Month, day int) clocky.Time {
	if IsEarlyCloseDay(year, month, day) {
		return clocky.Date(year, month, day, 17, 0, 0, 0, clocky.NYC)
	}
	return clocky.Date(year, month, day, 20, 0, 0, 0, clocky.NYC)
}

// IsTradingDay returns true if given date is a trading day.
func IsTradingDay(year int, month clocky.Month, day int) bool {
	wd := greg.Weekday(year, month, day)
	if wd == clocky.Saturday || wd == clocky.Sunday {
		return false
	}
	return !IsHoliday(year, month, day)
}

// IsMarketOpen returns true if the market is open for day trading.
func IsMarketOpen(dt clocky.Time) bool {
	year, month, day, hour, min, _ := dt.DateClock()
	if !IsTradingDay(year, month, day) {
		return false
	}
	hhmm := hour*100 + min
	if hhmm < 930 {
		return false
	}
	if IsEarlyCloseDay(year, month, day) {
		return hhmm < 1300
	}
	return hhmm < 1600
}

// IsMarketOpenExtended returns true if the market is open for day trading or extended hours.
func IsMarketOpenExtended(dt clocky.Time) bool {
	year, month, day, hour, _, _ := dt.DateClock()
	if !IsTradingDay(year, month, day) {
		return false
	}
	if hour < 4 {
		return false
	}
	if IsEarlyCloseDay(year, month, day) {
		return hour < 17
	}
	return hour < 20
}

// IsMarketOpenExtendedOvernight returns true if markets are open for day/extended/overnight trading.
func IsMarketOpenExtendedOvernight(dt clocky.Time) bool {
	year, month, day, hour, _, _ := dt.DateClock()
	if hour < 4 {
		return IsTradingDay(year, month, day)
	}
	if hour >= 20 {
		year2, month2, day2 := addDays(year, month, day, 1)
		return IsTradingDay(year2, month2, day2)
	}
	if !IsTradingDay(year, month, day) {
		return false
	}
	if IsEarlyCloseDay(year, month, day) {
		return hour < 17
	}
	return true
}

// IsExtended returns true if the market is open for extended hours only.
func IsExtended(dt clocky.Time) bool {
	year, month, day, hour, min, _ := dt.DateClock()
	if !IsTradingDay(year, month, day) {
		return false
	}
	hhmm := hour*100 + min
	if hhmm < 400 {
		return false
	}
	if hhmm < 930 {
		return true // pre-market
	}
	if IsEarlyCloseDay(year, month, day) {
		return hhmm >= 1300 && hour < 17
	}
	return hhmm >= 1600 && hour < 20
}

// IsOvernight returns true if the market is open for overnight trading only.
func IsOvernight(dt clocky.Time) bool {
	// The core offering is the Blue Ocean Session, which operates overnight
	// in the US from 8:00 PM ET- 4:00 AM ET (Sunday-Thursday).  It operates
	// only on those calendar days when the NYSE Trade Report Facility (TRF)
	// is open for reporting the following morning. Trades executed between
	// 8:00 PM ET and 12:00 AM ET will carry a trade date of the following
	// trade day. Settlement date will reflect the first business day
	// following the transaction (T+1). https://blueocean-tech.io/faq/
	year, month, day, hour, _, _ := dt.DateClock()
	if hour < 4 {
		return IsTradingDay(year, month, day)
	}
	if hour >= 20 {
		year2, month2, day2 := addDays(year, month, day, 1)
		return IsTradingDay(year2, month2, day2)
	}
	return false
}
