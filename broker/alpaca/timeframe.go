package alpaca

import (
	"dropbear/clocky"
	"errors"
)

// Timeframe represents a bar aggregation period for historical data.
type Timeframe string

func (t Timeframe) String() string {
	return string(t)
}

var ErrUnsupportedTimeframe = errors.New("unsupported duration for bars timeframe")

// timeframeFromDuration converts a clocky.Duration to an Alpaca timeframe string.
// Returns an error if the duration doesn't align with supported timeframes:
//   - [1-59]T for minute aggregations
//   - [1-23]H for hour aggregations
//   - 1D for daily aggregations
//   - 1W for weekly aggregations
//   - [1,2,3,4,6,12]M for month aggregations
func timeframeFromDuration(d clocky.Duration) (Timeframe, error) {
	i := int64(d)
	if i == 0 {
		return "", ErrUnsupportedTimeframe
	}
	m := i / int64(clocky.Minute)
	if i%int64(clocky.Minute) != 0 {
		return "", ErrUnsupportedTimeframe
	}
	if m >= 1 && m <= 9 {
		var b [2]byte
		b[0] = '0' + byte(m)
		b[1] = 'T'
		return Timeframe(b[:]), nil
	}
	if m >= 10 && m <= 59 {
		var b [3]byte
		b[0] = '0' + byte(m/10)
		b[1] = '0' + byte(m%10)
		b[2] = 'T'
		return Timeframe(b[:]), nil
	}
	h := m / 60
	if m%60 != 0 {
		return "", ErrUnsupportedTimeframe
	}
	if h >= 1 && h <= 9 {
		var b [2]byte
		b[0] = '0' + byte(h)
		b[1] = 'H'
		return Timeframe(b[:]), nil
	}
	if h >= 10 && h <= 23 {
		var b [3]byte
		b[0] = '0' + byte(h/10)
		b[1] = '0' + byte(h%10)
		b[2] = 'H'
		return Timeframe(b[:]), nil
	}
	switch d {
	case clocky.Day:
		return "1D", nil
	case clocky.Week:
		return "1W", nil
	case clocky.Monthy:
		return "1M", nil
	case clocky.Monthy * 2:
		return "2M", nil
	case clocky.Monthy * 3:
		return "3M", nil
	case clocky.Monthy * 4:
		return "4M", nil
	case clocky.Monthy * 6:
		return "6M", nil
	case clocky.Monthy * 12:
		return "12M", nil
	default:
		return "", ErrUnsupportedTimeframe
	}
}
