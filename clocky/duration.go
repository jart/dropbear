package clocky

import (
	"dropbear/decimal"
	"fmt"
)

// Duration represents a length of time in microseconds.
type Duration int64

const (
	Microsecond Duration = 1
	Millisecond Duration = 1000 * Microsecond
	Second      Duration = 1000 * Millisecond
	Minute      Duration = 60 * Second
	Hour        Duration = 60 * Minute
	Day         Duration = 24 * Hour
	Week        Duration = 7 * Day
	Month       Duration = 30 * Day
	Year        Duration = 365 * Day
)

func (d Duration) Milliseconds() int64    { return int64(d) / int64(Millisecond) }
func (d Duration) Hours() decimal.Decimal { return d.Div(Hour) }
func (d Duration) Days() decimal.Decimal  { return d.Div(Day) }

func (d Duration) Div(u Duration) decimal.Decimal {
	num := int64(d) / int64(u)
	den := int64(d) % int64(u)
	fix := int64(decimal.Scale)
	return decimal.Decimal(num*fix + den*fix/int64(u))
}

const (
	minDuration Duration = -1 << 63
	maxDuration Duration = 1<<63 - 1
)

// Round returns the result of rounding d to the nearest multiple of m.
// The rounding behavior for halfway values is to round away from zero.
// If the result exceeds the maximum (or minimum)
// value that can be stored in a Duration, Round returns the maximum (or minimum) duration.
// If m <= 0, Round returns d unchanged.
func (d Duration) Round(m Duration) Duration {
	if m <= 0 {
		return d
	}
	r := d % m
	if d < 0 {
		r = -r
		if uint64(r)+uint64(r) < uint64(m) {
			return d + r
		}
		if d1 := d - m + r; d1 < d {
			return d1
		}
		return minDuration // overflow
	}
	if uint64(r)+uint64(r) < uint64(m) {
		return d - r
	}
	if d1 := d + m - r; d1 > d {
		return d1
	}
	return maxDuration // overflow
}

// MustParseDuration is like ParseDuration but panics on error.
func MustParseDuration(s string) Duration {
	d, err := ParseDuration(s)
	if err != nil {
		panic("invalid duration: " + s)
	}
	return d
}

// ParseDuration parses a duration string like "1500ms", "2h45m", or "-3d".
// When no unit is specified, seconds are assumed.
func ParseDuration(str string) (Duration, error) {

	// skip spaces
	i := 0
	for i < len(str) && (str[i] == ' ' || str[i] == '\t') {
		i++
	}

	// return zero if empty
	if i == len(str) {
		return 0, nil
	}

	// parse sign
	s := Duration(1)
	if len(str) > 0 && str[0] == '-' {
		s = -1
		i++
	}

	// parse integer part
	x := Duration(0)
	for i < len(str) && str[i] >= '0' && str[i] <= '9' {
		x *= 10
		x += Duration(str[i] - '0')
		i++
	}

	// parse time unit
	if i < len(str) {
		switch str[i] {
		case 'u':
			x *= Microsecond
			i++
			if i < len(str) && str[i] == 's' {
				i++
			}
		case 'm':
			if i+1 < len(str) && str[i+1] == 's' {
				x *= Millisecond
				i += 2
			} else {
				x *= Minute
				i++
			}
		case 's':
			x *= Second
			i++
		case 'M':
			x *= Minute
			i++
		case 'h':
			x *= Hour
			i++
		case 'd':
			x *= Day
			i++
		case 'w':
			x *= Week
			i++
		case 'y':
			x *= Year
			i++
		default:
			return x * s, fmt.Errorf("invalid duration: %s", str)
		}
	} else {
		x *= Second
	}

	// recurse for more parts
	if i < len(str) {
		y, err := ParseDuration(str[i:])
		x += y
		if err != nil {
			return x * s, err
		}
	}

	return x * s, nil
}

type unit struct {
	suffix string
	value  Duration
}

var units = []unit{
	{"y", Year},
	{"w", Week},
	{"d", Day},
	{"h", Hour},
	{"m", Minute},
	{"s", Second},
	{"ms", Millisecond},
	{"us", Microsecond},
}

// String returns a string representation of the duration like "2h45m" or "-3d".
func (d Duration) String() string {
	if d == 0 {
		return "0"
	}
	neg := false
	if d < 0 {
		neg = true
		d = -d
	}
	str := ""
	for _, unit := range units {
		if d >= unit.value {
			qty := d / unit.value
			d -= qty * unit.value
			str += fmt.Sprintf("%d%s", qty, unit.suffix)
		}
	}
	if neg {
		str = "-" + str
	}
	return str
}
