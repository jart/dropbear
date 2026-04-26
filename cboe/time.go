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

type Session uint8

const (
	SessionClosed Session = iota
	SessionDay
	SessionExtended
	SessionOvernight
)

// GetSession returns the trading session for given time.
// The fast path is optimized for regular trading hours (9:30–16:00)
// since that's when the SIP feed is most active and latency matters.
func GetSession(dt clocky.Time) Session {
	year, month, day, hour, min, _ := dt.DateClock()
	hhmm := hour*100 + min
	// fast path: regular trading hours
	if hhmm >= 930 && hhmm < 1600 {
		if IsTradingDay(year, month, day) {
			if hhmm >= 1300 && IsEarlyCloseDay(year, month, day) {
				return SessionExtended
			}
			return SessionDay
		}
		return SessionClosed
	}
	if hhmm < 400 {
		if IsTradingDay(year, month, day) {
			return SessionOvernight
		}
		return SessionClosed
	}
	if hhmm < 930 {
		if IsTradingDay(year, month, day) {
			return SessionExtended
		}
		return SessionClosed
	}
	// hhmm >= 1600
	if IsTradingDay(year, month, day) {
		closeHour := 20
		if IsEarlyCloseDay(year, month, day) {
			closeHour = 17
		}
		if hour < closeHour {
			return SessionExtended
		}
	}
	if hour >= 20 {
		year2, month2, day2 := addDays(year, month, day, 1)
		if IsTradingDay(year2, month2, day2) {
			return SessionOvernight
		}
	}
	return SessionClosed
}
