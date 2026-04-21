// cadence predicts when options chains are available
// https://www.cboe.com/available_weeklys/
// https://cdn.cboe.com/resources/options/Cboe2026OPTIONSCalendar.pdf
package cboe

import (
	"dropbear/clocky"
	"dropbear/greg"
	"dropbear/symbol"
)

// OptionCadence defines how frequently options chains are available.
type OptionCadence byte

const (
	OptionCadenceWeekly     OptionCadence = iota // every friday
	OptionCadenceTweekly                         // every monday, wednesday, and friday
	OptionCadenceMonthly                         // third friday of month
	OptionCadenceDaily                           // every trading day
	OptionCadenceVIXWeekly                       // every wednesday except the third wednesday of month
	OptionCadenceVIXMonthly                      // third wednesday of month
)

// GetOptionCadence returns how frequently options chains are available for a given symbol.
func GetOptionCadence(sym symbol.Symbol) OptionCadence {
	return kOptionCadence[sym]
}

// HasOptionChain returns true if an options chain is available for the
// given symbol and date. When a regular expiration day falls on a market
// holiday, the chain moves to the previous trading day (e.g. Good Friday
// chains move to Thursday). This implementation assumes stocks at least
// have weekly cadence and hasn't hard-coded many monthly ones.
func HasOptionChain(sym symbol.Symbol, year int, month clocky.Month, day int) bool {
	cadence := GetOptionCadence(sym)
	switch cadence {
	case OptionCadenceDaily:
		return !IsHoliday(year, month, day)
	case OptionCadenceTweekly:
		switch greg.Weekday(year, month, day) {
		case clocky.Monday:
			return !IsHoliday(year, month, day)
		case clocky.Tuesday:
			// monday was a holiday, tuesday gets the monday chain
			y, m, d := addDays(year, month, day, -1)
			return IsHoliday(y, m, d)
		case clocky.Wednesday:
			return !IsHoliday(year, month, day)
		case clocky.Thursday:
			// friday is a holiday, thursday gets the friday chain; OR
			// wednesday was a holiday, thursday gets the wednesday chain
			y1, m1, d1 := addDays(year, month, day, +1)
			y2, m2, d2 := addDays(year, month, day, -1)
			return IsHoliday(y1, m1, d1) || IsHoliday(y2, m2, d2)
		case clocky.Friday:
			return !IsHoliday(year, month, day)
		default:
			return false
		}
	case OptionCadenceWeekly:
		switch greg.Weekday(year, month, day) {
		case clocky.Friday:
			return !IsHoliday(year, month, day)
		case clocky.Thursday:
			// if friday is a holiday, thursday gets the chain
			y, m, d := addDays(year, month, day, 1)
			return IsHoliday(y, m, d)
		default:
			return false
		}
	case OptionCadenceMonthly:
		switch greg.Weekday(year, month, day) {
		case clocky.Friday:
			return isThirdWeek(day) && !IsHoliday(year, month, day)
		case clocky.Thursday:
			// third friday is a holiday (e.g. Good Friday), thursday gets the chain
			y, m, d := addDays(year, month, day, 1)
			return isThirdWeek(d) && IsHoliday(y, m, d)
		default:
			return false
		}
	case OptionCadenceVIXMonthly:
		switch greg.Weekday(year, month, day) {
		case clocky.Wednesday:
			return isThirdWeek(day) && !IsHoliday(year, month, day)
		case clocky.Tuesday:
			y, m, d := addDays(year, month, day, 1)
			return isThirdWeek(d) && IsHoliday(y, m, d)
		default:
			return false
		}
	case OptionCadenceVIXWeekly:
		switch greg.Weekday(year, month, day) {
		case clocky.Wednesday:
			// vixw is available every wednesday except the third wednesday of the month
			return !isThirdWeek(day) && !IsHoliday(year, month, day)
		case clocky.Tuesday:
			y, m, d := addDays(year, month, day, 1)
			return !isThirdWeek(d) && IsHoliday(y, m, d)
		default:
			return false
		}
	default:
		return false
	}
}

func GetNextOptionChain(sym symbol.Symbol, year int, month clocky.Month, day int) (int, clocky.Month, int) {
	for {
		year, month, day = addDays(year, month, day, 1)
		if HasOptionChain(sym, year, month, day) {
			return year, month, day
		}
	}
}

func addDays(year int, month clocky.Month, day int, count int) (int, clocky.Month, int) {
	t := clocky.Date(year, month, day, 0, 0, 0, 0, clocky.NYC)
	t = t.Add(clocky.Duration(count) * clocky.Day)
	return t.Date()
}

func isThirdWeek(day int) bool {
	return (day-1)/7 == 2
}

var kOptionCadence = map[symbol.Symbol]OptionCadence{
	symbol.SPXW:  OptionCadenceDaily,
	symbol.RUTW:  OptionCadenceDaily,
	symbol.XSP:   OptionCadenceDaily,
	symbol.NDX:   OptionCadenceDaily,
	symbol.SPY:   OptionCadenceDaily,
	symbol.QQQ:   OptionCadenceDaily,
	symbol.IWM:   OptionCadenceDaily,
	symbol.GOOGL: OptionCadenceTweekly,
	symbol.AAPL:  OptionCadenceTweekly,
	symbol.MSFT:  OptionCadenceTweekly,
	symbol.AMZN:  OptionCadenceTweekly,
	symbol.NVDA:  OptionCadenceTweekly,
	symbol.TSLA:  OptionCadenceTweekly,
	symbol.AVGO:  OptionCadenceTweekly,
	symbol.META:  OptionCadenceTweekly,
	symbol.IBIT:  OptionCadenceTweekly,
	symbol.TLT:   OptionCadenceTweekly,
	symbol.GLD:   OptionCadenceTweekly,
	symbol.SLV:   OptionCadenceTweekly,
	symbol.SPEQX: OptionCadenceMonthly,
	symbol.BTI:   OptionCadenceMonthly,
	symbol.VIXW:  OptionCadenceVIXWeekly,
	symbol.VIX:   OptionCadenceVIXMonthly,
}
