package clocky

import "time"

var localFormats = []string{
	"2006-01-02T15:04:05-0700", // e.g. "2024-01-15T10:30:00-0500" (schwab order history api)
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04",
	"2006-01-02",
}

// MustParseTime is like ParseTime but panics on error.
func MustParseTime(s string) Time {
	t, err := ParseTime(s)
	if err != nil {
		panic("invalid timestamp: " + s)
	}
	return t
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
