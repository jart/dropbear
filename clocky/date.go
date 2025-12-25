package clocky

import "time"

// Date returns Time for the given date components in the specified location.
func Date(year int, month time.Month, day, hour, min, sec, nsec int, loc *time.Location) Time {
	return Time(time.Date(year, month, day, hour, min, sec, nsec, loc).UnixMicro())
}
