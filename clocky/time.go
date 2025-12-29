package clocky

import (
	"encoding/binary"
	"io"
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
func (t Time) Month() int               { return int(time.UnixMicro(int64(t)).In(TZ).Month()) }
func (t Time) Year() int                { return time.UnixMicro(int64(t)).In(TZ).Year() }
func (t Time) Weekday() Weekday         { return Weekday(time.UnixMicro(int64(t)).In(TZ).Weekday()) }
func (t Time) YearMonth() string        { return time.UnixMicro(int64(t)).In(TZ).Format("2006-01") }

// String returns formatted local microsecond timestamp.
// This is intended for displaying the time to us humans.
func (t Time) String() string {
	return time.UnixMicro(int64(t)).In(TZ).Format("2006-01-02T15:04:05.000000")
}

// String returns formatted RFC3339 UTC timestamp.
// This is intended for Internet protocols that need it.
func (t Time) RFC3339() string {
	return time.UnixMicro(int64(t)).UTC().Format(time.RFC3339Nano)
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

// Exchange atomically replaces v into d.
func (d *Time) Exchange(v Time) Time {
	return Time(atomic.SwapInt64((*int64)(d), int64(v)))
}

func (d Time) Encode(b []byte) []byte {
	return binary.LittleEndian.AppendUint64(b, uint64(d))
}

func (d *Time) Decode(b []byte) []byte {
	*d = Time(int64(binary.LittleEndian.Uint64(b)))
	return b[8:]
}

func (d *Time) Deserialize(reader io.Reader) error {
	var b [8]byte
	_, err := io.ReadFull(reader, b[:])
	if err != nil {
		return err
	}
	d.Decode(b[:])
	return nil
}
