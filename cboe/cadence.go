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

func addDays(year int, month clocky.Month, day int, count int) (int, clocky.Month, int) {
	t := clocky.Date(year, month, day, 0, 0, 0, 0, clocky.NYC)
	t = t.Add(clocky.Duration(count) * clocky.Day)
	return t.Date()
}

func isThirdWeek(day int) bool {
	return (day-1)/7 == 2
}

var kOptionCadence = map[symbol.Symbol]OptionCadence{
	kSPXW:  OptionCadenceDaily,
	kRUTW:  OptionCadenceDaily,
	kXSP:   OptionCadenceDaily,
	kNDX:   OptionCadenceDaily,
	kSPY:   OptionCadenceDaily,
	kQQQ:   OptionCadenceDaily,
	kIWM:   OptionCadenceDaily,
	kGOOGL: OptionCadenceTweekly,
	kAAPL:  OptionCadenceTweekly,
	kMSFT:  OptionCadenceTweekly,
	kNVDA:  OptionCadenceTweekly,
	kTSLA:  OptionCadenceTweekly,
	kAVGO:  OptionCadenceTweekly,
	kMETA:  OptionCadenceTweekly,
	kSPEQX: OptionCadenceMonthly,
	kBTI:   OptionCadenceMonthly,
	kVIXW:  OptionCadenceVIXWeekly,
	kVIX:   OptionCadenceVIXMonthly,
}

const (
	kSPXW  = symbol.Symbol('S' | 'P'<<8 | 'X'<<16 | 'W'<<24)
	kRUTW  = symbol.Symbol('R' | 'U'<<8 | 'T'<<16 | 'W'<<24)
	kXSP   = symbol.Symbol('X' | 'S'<<8 | 'P'<<16)
	kNDX   = symbol.Symbol('N' | 'D'<<8 | 'X'<<16)
	kSPY   = symbol.Symbol('S' | 'P'<<8 | 'Y'<<16)
	kQQQ   = symbol.Symbol('Q' | 'Q'<<8 | 'Q'<<16)
	kIWM   = symbol.Symbol('I' | 'W'<<8 | 'M'<<16)
	kNVDA  = symbol.Symbol('N' | 'V'<<8 | 'D'<<16 | 'A'<<24)
	kTSLA  = symbol.Symbol('T' | 'S'<<8 | 'L'<<16 | 'A'<<24)
	kAVGO  = symbol.Symbol('A' | 'V'<<8 | 'G'<<16 | 'O'<<24)
	kMETA  = symbol.Symbol('M' | 'E'<<8 | 'T'<<16 | 'A'<<24)
	kMSFT  = symbol.Symbol('M' | 'S'<<8 | 'F'<<16 | 'T'<<24)
	kAAPL  = symbol.Symbol('A' | 'A'<<8 | 'P'<<16 | 'L'<<24)
	kGOOGL = symbol.Symbol('G' | 'O'<<8 | 'O'<<16 | 'G'<<24 | 'L'<<32)
	kBTI   = symbol.Symbol('B' | 'T'<<8 | 'I'<<16)
	kVIX   = symbol.Symbol('V' | 'I'<<8 | 'X'<<16)
	kVIXW  = symbol.Symbol('V' | 'I'<<8 | 'X'<<16 | 'W'<<24)
	kSPEQX = symbol.Symbol('S' | 'P'<<8 | 'E'<<16 | 'Q'<<24 | 'X'<<32)
)
