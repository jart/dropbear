package clocky

import (
	"sync/atomic"
	"time"
)

// Time represents an instant in time, stored as UNIX UTC nanoseconds.
type Time int64

const (
	MaxTime = Time(1<<63 - 1)
	MinTime = -MaxTime - 1
)

// Weekday represents a day of the week.
type Weekday int

const (
	Sunday Weekday = iota
	Monday
	Tuesday
	Wednesday
	Thursday
	Friday
	Saturday
)

// A Month specifies a month of the year (January = 1, ...).
type Month int

const (
	January Month = 1 + iota
	February
	March
	April
	May
	June
	July
	August
	September
	October
	November
	December
)

func (t Time) IsZero() bool             { return t == 0 }
func (t Time) Add(d Duration) Time      { return Time(int64(t) + int64(d)) }
func (t Time) Sub(u Time) Duration      { return Duration(int64(t) - int64(u)) }
func (t Time) Unix() int64              { return int64(t) / 1_000_000_000 }
func (t Time) UnixNano() int64          { return int64(t) }
func (t Time) Quantize(d Duration) Time { return Time((int64(t) / int64(d)) * int64(d)) }
func (t Time) Before(u Time) bool       { return t < u }
func (t Time) After(u Time) bool        { return t > u }
func (t Time) Minute() int              { return t.goTime().Minute() }
func (t Time) Hour() int                { return t.goTime().Hour() }
func (t Time) Day() int                 { return t.goTime().Day() }
func (t Time) Month() int               { return int(t.goTime().Month()) }
func (t Time) Year() int                { return t.goTime().Year() }
func (t Time) Weekday() Weekday         { return Weekday(t.goTime().Weekday()) }

func (t Time) goTime() time.Time {
	return time.Unix(int64(t)/1_000_000_000, int64(t)%1_000_000_000).In(TZ)
}

func (t Time) goTimeUTC() time.Time {
	return time.Unix(int64(t)/1_000_000_000, int64(t)%1_000_000_000).UTC()
}

// Date returns the date.
func (t Time) Date() (year int, month Month, day int) {
	y, m, d := t.goTime().Date()
	return y, Month(m), d
}

// Clock returns the time.
func (t Time) Clock() (hour, min, sec int) {
	return t.goTime().Clock()
}

// DateInt returns the date in YYYYMMDD format.
func (t Time) DateInt() int {
	y, m, d := t.Date()
	return y*10000 + int(m)*100 + d
}

// ClockInt returns the time in HHMMSS format.
func (t Time) ClockInt() int {
	hour, min, sec := t.Clock()
	return hour*1_00_00 + min*1_00 + sec
}

// String returns formatted local nanosecond timestamp.
// This is intended for displaying the time to us humans.
func (t Time) String() string {
	var buf [29]byte
	u := t.goTime()
	y, m, d := u.Date()
	h, M, s := u.Clock()
	ns := int64(t) % 1_000_000_000
	buf[0] = byte('0' + (y/1000)%10)
	buf[1] = byte('0' + (y/100)%10)
	buf[2] = byte('0' + (y/10)%10)
	buf[3] = byte('0' + (y % 10))
	buf[4] = '-'
	buf[5] = byte('0' + int(m)/10)
	buf[6] = byte('0' + int(m)%10)
	buf[7] = '-'
	buf[8] = byte('0' + d/10)
	buf[9] = byte('0' + d%10)
	buf[10] = 'T'
	buf[11] = byte('0' + h/10)
	buf[12] = byte('0' + h%10)
	buf[13] = ':'
	buf[14] = byte('0' + M/10)
	buf[15] = byte('0' + M%10)
	buf[16] = ':'
	buf[17] = byte('0' + s/10)
	buf[18] = byte('0' + s%10)
	buf[19] = '.'
	buf[20] = byte('0' + (ns/100000000)%10)
	buf[21] = byte('0' + (ns/10000000)%10)
	buf[22] = byte('0' + (ns/1000000)%10)
	buf[23] = byte('0' + (ns/100000)%10)
	buf[24] = byte('0' + (ns/10000)%10)
	buf[25] = byte('0' + (ns/1000)%10)
	buf[26] = byte('0' + (ns/100)%10)
	buf[27] = byte('0' + (ns/10)%10)
	buf[28] = byte('0' + (ns % 10))
	return string(buf[:])
}

// GoString returns Go syntax for the time, e.g. clocky.MustParseTime("2026-01-06T20:12:33.059744Z").
func (t Time) GoString() string {
	return "clocky.MustParseTime(\"" + t.RFC3339() + "\")"
}

// RFC3339 returns formatted RFC3339 UTC timestamp with nanosecond precision.
// This is intended for Internet protocols that need it.
func (t Time) RFC3339() string {
	var buf [30]byte
	u := t.goTimeUTC()
	y, m, d := u.Date()
	h, M, s := u.Clock()
	ns := int64(t) % 1_000_000_000
	buf[0] = byte('0' + (y/1000)%10)
	buf[1] = byte('0' + (y/100)%10)
	buf[2] = byte('0' + (y/10)%10)
	buf[3] = byte('0' + (y % 10))
	buf[4] = '-'
	buf[5] = byte('0' + int(m)/10)
	buf[6] = byte('0' + int(m)%10)
	buf[7] = '-'
	buf[8] = byte('0' + d/10)
	buf[9] = byte('0' + d%10)
	buf[10] = 'T'
	buf[11] = byte('0' + h/10)
	buf[12] = byte('0' + h%10)
	buf[13] = ':'
	buf[14] = byte('0' + M/10)
	buf[15] = byte('0' + M%10)
	buf[16] = ':'
	buf[17] = byte('0' + s/10)
	buf[18] = byte('0' + s%10)
	buf[19] = '.'
	buf[20] = byte('0' + (ns/100000000)%10)
	buf[21] = byte('0' + (ns/10000000)%10)
	buf[22] = byte('0' + (ns/1000000)%10)
	buf[23] = byte('0' + (ns/100000)%10)
	buf[24] = byte('0' + (ns/10000)%10)
	buf[25] = byte('0' + (ns/1000)%10)
	buf[26] = byte('0' + (ns/100)%10)
	buf[27] = byte('0' + (ns/10)%10)
	buf[28] = byte('0' + (ns % 10))
	buf[29] = 'Z'
	return string(buf[:])
}

// RFC3339NYC returns formatted RFC3339 timestamp in US Eastern time.
// This is intended for displaying timestamps to traders since US equity
// markets operate on Eastern time. The offset will be -05:00 (EST) or
// -04:00 (EDT) depending on daylight saving time.
func (t Time) RFC3339NYC() string {
	u := time.Unix(int64(t)/1_000_000_000, int64(t)%1_000_000_000).In(NYC)
	y, m, d := u.Date()
	h, M, s := u.Clock()
	_, offset := u.Zone()
	offsetHours := offset / 3600
	var buf [25]byte
	buf[0] = byte('0' + (y/1000)%10)
	buf[1] = byte('0' + (y/100)%10)
	buf[2] = byte('0' + (y/10)%10)
	buf[3] = byte('0' + (y % 10))
	buf[4] = '-'
	buf[5] = byte('0' + int(m)/10)
	buf[6] = byte('0' + int(m)%10)
	buf[7] = '-'
	buf[8] = byte('0' + d/10)
	buf[9] = byte('0' + d%10)
	buf[10] = 'T'
	buf[11] = byte('0' + h/10)
	buf[12] = byte('0' + h%10)
	buf[13] = ':'
	buf[14] = byte('0' + M/10)
	buf[15] = byte('0' + M%10)
	buf[16] = ':'
	buf[17] = byte('0' + s/10)
	buf[18] = byte('0' + s%10)
	buf[19] = '-'
	buf[20] = '0'
	buf[21] = byte('0' + (-offsetHours)%10)
	buf[22] = ':'
	buf[23] = '0'
	buf[24] = '0'
	return string(buf[:])
}

var localFormats = []string{
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02",
}

// Date creates a Time from date and time components in California.
func Date(year int, month Month, day, hour, min, sec, nanos int) Time {
	t := time.Date(year, time.Month(month), day, hour, min, sec, nanos, TZ)
	return Time(t.UnixNano())
}

// Parse turns string into Time.
func ParseTime(s string) (Time, error) {
	if s == "" {
		return 0, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return Time(t.UnixNano()), nil
	}
	t, err = time.Parse(time.RFC3339, s)
	if err == nil {
		return Time(t.UnixNano()), nil
	}
	for _, format := range localFormats {
		if t, err := time.ParseInLocation(format, s, TZ); err == nil {
			return Time(t.UnixNano()), nil
		}
	}
	return 0, err
}

// MustParseTime is like ParseTime but panics on error.
func MustParseTime(s string) Time {
	t, err := ParseTime(s)
	if err != nil {
		panic("invalid timestamp: " + s)
	}
	return t
}

// Store atomically stores v into d.
func (d *Time) Store(v Time) {
	atomic.StoreInt64((*int64)(d), int64(v))
}

// Load atomically loads and returns the value of d.
func (d *Time) Load() Time {
	return Time(atomic.LoadInt64((*int64)(d)))
}

// AtomicAdd atomically adds v to d.
func (d *Time) AtomicAdd(v Duration) Time {
	return Time(atomic.AddInt64((*int64)(d), int64(v)))
}

// Swap atomically replaces v into d.
func (d *Time) Swap(v Time) Time {
	return Time(atomic.SwapInt64((*int64)(d), int64(v)))
}

// CompareAndSwap executes the compare-and-swap operation for d.
func (d *Time) CompareAndSwap(old, new Time) bool {
	return atomic.CompareAndSwapInt64((*int64)(d), int64(old), int64(new))
}
