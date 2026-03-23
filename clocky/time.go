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
func (t Time) Second() int              { return t.goTime().Second() }
func (t Time) Nanosecond() int          { return t.goTime().Nanosecond() }
func (t Time) Hour() int                { return t.goTime().Hour() }
func (t Time) Day() int                 { return t.goTime().Day() }
func (t Time) Month() Month             { return Month(t.goTime().Month()) }
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

// Date creates a Time from date and time components in the specified location.
func Date(year int, month Month, day, hour, min, sec, nanos int, loc *time.Location) Time {
	t := time.Date(year, time.Month(month), day, hour, min, sec, nanos, loc)
	return Time(t.UnixNano())
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

func (d Time) Format(layout string) string {
	return d.goTime().Format(layout)
}
