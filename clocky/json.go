package clocky

import (
	"errors"
	"slices"
	"time"
)

var (
	ErrTimestampTooOld = errors.New("timestamp before 1980 is ambiguous (use RFC3339 string for older dates)")
	ErrMissingTimezone = errors.New("timestamp string requires timezone (Z or +/-offset)")
	ErrYearOverflow    = errors.New("year outside 0000-9999 range")
)

// UnmarshalJSON implements json.Unmarshaler.
// Accepts JSON strings (RFC3339 with timezone), numbers (unix seconds/millis/micros), or null.
// Float numbers like 1705314600.123 are treated as seconds with fractional precision.
// String timestamps MUST include timezone (Z or +/-HH:MM) to avoid ambiguity.
// Null values are unmarshaled as zero (Unix epoch).
func (t *Time) UnmarshalJSON(data []byte) error {
	if len(data) == 4 && data[0] == 'n' && data[1] == 'u' && data[2] == 'l' && data[3] == 'l' {
		*t = 0
		return nil
	}
	if data[0] == '"' {
		return t.unmarshalRFC3339(data)
	}
	if hasDot(data) {
		return t.unmarshalFloat(data)
	}
	return t.unmarshalInteger(data)
}

// MarshalJSON implements json.Marshaler.
// Outputs RFC3339 with nanosecond precision.
func (t Time) MarshalJSON() ([]byte, error) {
	var buf [32]byte
	buf[0] = '"'
	u := time.Unix(int64(t)/1_000_000_000, int64(t)%1_000_000_000).UTC()
	y, m, d := u.Date()
	if y < 0 || y > 9999 {
		return nil, ErrYearOverflow
	}
	h, M, s := u.Clock()
	ns := int64(t) % 1_000_000_000
	buf[1] = byte('0' + (y/1000)%10)
	buf[2] = byte('0' + (y/100)%10)
	buf[3] = byte('0' + (y/10)%10)
	buf[4] = byte('0' + (y % 10))
	buf[5] = '-'
	buf[6] = byte('0' + int(m)/10)
	buf[7] = byte('0' + int(m)%10)
	buf[8] = '-'
	buf[9] = byte('0' + d/10)
	buf[10] = byte('0' + d%10)
	buf[11] = 'T'
	buf[12] = byte('0' + h/10)
	buf[13] = byte('0' + h%10)
	buf[14] = ':'
	buf[15] = byte('0' + M/10)
	buf[16] = byte('0' + M%10)
	buf[17] = ':'
	buf[18] = byte('0' + s/10)
	buf[19] = byte('0' + s%10)
	buf[20] = '.'
	buf[21] = byte('0' + (ns/100000000)%10)
	buf[22] = byte('0' + (ns/10000000)%10)
	buf[23] = byte('0' + (ns/1000000)%10)
	buf[24] = byte('0' + (ns/100000)%10)
	buf[25] = byte('0' + (ns/10000)%10)
	buf[26] = byte('0' + (ns/1000)%10)
	buf[27] = byte('0' + (ns/100)%10)
	buf[28] = byte('0' + (ns/10)%10)
	buf[29] = byte('0' + (ns % 10))
	buf[30] = 'Z'
	buf[31] = '"'
	return buf[:], nil
}

const (
	// minTimestamp is 1980-01-01 00:00:00 UTC in nanoseconds.
	// Timestamps before this are rejected because the seconds/millis/micros/nanos
	// heuristic becomes unreliable near the Unix epoch.
	minTimestamp = 315_532_800_000_000_000

	// epochWindow allows timestamps within ±2 days of epoch (for zero/null values).
	// This is in the final ns result, so ±172800 seconds input (treated as seconds).
	epochWindow = 2 * 24 * 60 * 60 * 1_000_000_000 // 172,800,000,000,000 ns
)

func (t *Time) unmarshalRFC3339(data []byte) error {
	data = data[1 : len(data)-1]
	parsed, err := ParseTime(string(data))
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

func (t *Time) unmarshalFloat(data []byte) error {
	neg := false
	i := 0
	if data[0] == '-' {
		neg = true
		i++
	}
	var intPart int64
	for ; i < len(data) && data[i] != '.'; i++ {
		intPart = intPart*10 + int64(data[i]-'0')
	}
	i++ // skip dot
	// Parse up to 9 fractional digits as nanoseconds
	var fracNanos int64
	mul := int64(100_000_000)
	for ; i < len(data) && mul > 0; i++ {
		fracNanos += int64(data[i]-'0') * mul
		mul /= 10
	}
	if neg {
		intPart = -intPart
		fracNanos = -fracNanos
	}
	result := Time(intPart*1_000_000_000 + fracNanos)
	if err := validateTimestamp(result); err != nil {
		return err
	}
	*t = result
	return nil
}

func (t *Time) unmarshalInteger(data []byte) error {
	neg := false
	i := 0
	if data[0] == '-' {
		neg = true
		i++
	}
	var v int64
	for ; i < len(data); i++ {
		v = v*10 + int64(data[i]-'0')
	}
	if neg {
		v = -v
	}
	result, err := fromUnixAuto(v)
	if err != nil {
		return err
	}
	*t = result
	return nil
}

// fromUnixAuto guesses if v is seconds, milliseconds, microseconds, or nanoseconds.
func fromUnixAuto(v int64) (Time, error) {
	abs := v
	if abs < 0 {
		abs = -abs
	}
	var result Time
	switch {
	case abs < 1e11: // seconds
		result = Time(v * 1_000_000_000)
	case abs < 1e14: // milliseconds
		result = Time(v * 1_000_000)
	case abs < 1e17: // microseconds
		result = Time(v * 1_000)
	default: // nanoseconds
		result = Time(v)
	}
	if err := validateTimestamp(result); err != nil {
		return 0, err
	}
	return result, nil
}

// validateTimestamp returns error if timestamp is before 1980,
// unless it's within ±2 days of epoch (for zero/null values).
func validateTimestamp(result Time) error {
	if result >= -epochWindow && result <= epochWindow {
		return nil
	}
	if result < minTimestamp {
		return ErrTimestampTooOld
	}
	return nil
}

func hasDot(data []byte) bool {
	return slices.Contains(data, '.')
}

// hasTimezone checks if a timestamp string has a timezone indicator (Z or +/-offset).
func hasTimezone(data []byte) bool {
	n := len(data)
	if n == 0 {
		return false
	}
	if data[n-1] == 'Z' {
		return true
	}
	if data[n-1] < '0' || data[n-1] > '9' {
		return false
	}
	// Look for +/- before offset digits (+HH:MM is 6 chars, +HHMM is 5 chars)
	for i := n - 5; i >= 0 && i >= n-6; i-- {
		if data[i] == '+' || data[i] == '-' {
			valid := true
			for j := i + 1; j < n; j++ {
				c := data[j]
				if c != ':' && (c < '0' || c > '9') {
					valid = false
					break
				}
			}
			if valid {
				return true
			}
		}
	}
	return false
}
