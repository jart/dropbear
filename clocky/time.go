package clocky

import (
	"encoding/binary"
	"sync/atomic"
	"time"
)

// Time represents an instant in time, stored as UNIX UTC microseconds.
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

func (t Time) IsZero() bool             { return t == 0 }
func (t Time) Add(d Duration) Time      { return Time(int64(t) + int64(d)) }
func (t Time) Sub(u Time) Duration      { return Duration(int64(t) - int64(u)) }
func (t Time) Unix() int64              { return int64(t) / 1_000_000 }
func (t Time) Quantize(d Duration) Time { return Time((int64(t) / int64(d)) * int64(d)) }
func (t Time) Before(u Time) bool       { return t < u }
func (t Time) After(u Time) bool        { return t > u }
func (t Time) Minute() int              { return time.UnixMicro(int64(t)).In(TZ).Minute() }
func (t Time) Hour() int                { return time.UnixMicro(int64(t)).In(TZ).Hour() }
func (t Time) Day() int                 { return int(time.UnixMicro(int64(t)).In(TZ).Day()) }
func (t Time) Month() int               { return int(time.UnixMicro(int64(t)).In(TZ).Month()) }
func (t Time) Year() int                { return time.UnixMicro(int64(t)).In(TZ).Year() }
func (t Time) Weekday() Weekday         { return Weekday(time.UnixMicro(int64(t)).In(TZ).Weekday()) }

// Date returns the date.
func (t Time) Date() (year int, month time.Month, day int) {
	return time.UnixMicro(int64(t)).In(TZ).Date()
}

// Clock returns the time.
func (t Time) Clock() (hour, min, sec int) {
	return time.UnixMicro(int64(t)).In(TZ).Clock()
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

// YearMonth returns the year and month in YYYY-MM format.
func (t Time) YearMonthString() string {
	var buf [7]byte
	y, m, _ := t.Date()
	buf[0] = byte('0' + (y/1000)%10)
	buf[1] = byte('0' + (y/100)%10)
	buf[2] = byte('0' + (y/10)%10)
	buf[3] = byte('0' + (y % 10))
	buf[4] = '-'
	buf[5] = byte('0' + int(m)/10)
	buf[6] = byte('0' + int(m)%10)
	return string(buf[:])
}

// String returns formatted local microsecond timestamp.
// This is intended for displaying the time to us humans.
func (t Time) String() string {
	var buf [26]byte
	u := time.UnixMicro(int64(t)).In(TZ)
	y, m, d := u.Date()
	h, M, s := u.Clock()
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
	buf[20] = byte('0' + (t/100000)%10)
	buf[21] = byte('0' + (t/10000)%10)
	buf[22] = byte('0' + (t/1000)%10)
	buf[23] = byte('0' + (t/100)%10)
	buf[24] = byte('0' + (t/10)%10)
	buf[25] = byte('0' + (t % 10))
	return string(buf[:])
}

// String returns formatted RFC3339 UTC timestamp.
// This is intended for Internet protocols that need it.
func (t Time) RFC3339() string {
	var buf [27]byte
	u := time.UnixMicro(int64(t)).UTC()
	y, m, d := u.Date()
	h, M, s := u.Clock()
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
	buf[20] = byte('0' + (t/100000)%10)
	buf[21] = byte('0' + (t/10000)%10)
	buf[22] = byte('0' + (t/1000)%10)
	buf[23] = byte('0' + (t/100)%10)
	buf[24] = byte('0' + (t/10)%10)
	buf[25] = byte('0' + (t % 10))
	buf[26] = 'Z'
	return string(buf[:])
}

var localFormats = []string{
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02",
}

// Parse turns string into Time.
func ParseTime(s string) (Time, error) {
	if s == "" {
		return 0, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return Time(t.UnixMicro()), nil
	}
	for _, format := range localFormats {
		if t, err := time.ParseInLocation(format, s, TZ); err == nil {
			return Time(t.UnixMicro()), nil
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

func (d Time) Encode(b []byte) []byte {
	return binary.LittleEndian.AppendUint64(b, uint64(d))
}

func (d *Time) Decode(b []byte) []byte {
	*d = Time(int64(binary.LittleEndian.Uint64(b)))
	return b[8:]
}
