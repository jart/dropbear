package clocky

import "time"

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

func (t Time) GoString() string {
	return "clocky.MustParseTime(\"" + t.RFC3339NYCNanos() + "\")"
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

// RFC3339NYCNanos returns formatted RFC3339 timestamp in US Eastern time with
// nanosecond precision. Like RFC3339NYC but includes the fractional seconds.
func (t Time) RFC3339NYCNanos() string {
	u := time.Unix(int64(t)/1_000_000_000, int64(t)%1_000_000_000).In(NYC)
	y, m, d := u.Date()
	h, M, s := u.Clock()
	ns := u.Nanosecond()
	_, offset := u.Zone()
	offsetHours := offset / 3600
	var buf [35]byte
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
	buf[29] = '-'
	buf[30] = '0'
	buf[31] = byte('0' + (-offsetHours)%10)
	buf[32] = ':'
	buf[33] = '0'
	buf[34] = '0'
	return string(buf[:])
}
