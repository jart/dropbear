package sip

import "dropbear/clocky"

// parseTimestampFast parses RFC3339 timestamps from trusted Alpaca SIP data.
// Expects formats like:
//   - "2026-01-06T20:12:59.99839309Z" (with nanoseconds)
//   - "2026-01-06T20:12:00Z" (without fractional seconds)
//
// Returns nanoseconds since Unix epoch as clocky.Time.
func parseTimestamp(data []byte, i int) (clocky.Time, int, error) {
	if i >= len(data) || data[i] != '"' {
		return 0, i, nil
	}
	i++ // skip opening quote
	start := i

	// Find closing quote
	for i < len(data) && data[i] != '"' {
		i++
	}
	ts := data[start:i]
	if i < len(data) {
		i++ // skip closing quote
	}

	// Fast path: check minimum length for "YYYY-MM-DDTHH:MM:SSZ"
	if len(ts) < 20 {
		return 0, i, nil
	}

	// Parse date/time components directly from known offsets
	// Format: YYYY-MM-DDTHH:MM:SS[.nnnnnnnnn]Z
	//         0123456789012345678901234567890
	year := int(ts[0]-'0')*1000 + int(ts[1]-'0')*100 + int(ts[2]-'0')*10 + int(ts[3]-'0')
	month := int(ts[5]-'0')*10 + int(ts[6]-'0')
	day := int(ts[8]-'0')*10 + int(ts[9]-'0')
	hour := int(ts[11]-'0')*10 + int(ts[12]-'0')
	min := int(ts[14]-'0')*10 + int(ts[15]-'0')
	sec := int(ts[17]-'0')*10 + int(ts[18]-'0')

	// Parse fractional seconds if present
	var nsec int64
	if len(ts) > 20 && ts[19] == '.' {
		// Parse up to 9 digits for nanoseconds
		frac := ts[20 : len(ts)-1] // exclude trailing 'Z'
		var mult int64 = 100_000_000
		for j := 0; j < len(frac) && j < 9; j++ {
			nsec += int64(frac[j]-'0') * mult
			mult /= 10
		}
	}

	// Convert to Unix timestamp (days since epoch * 86400 + time of day)
	// Using the formula for days since Unix epoch (1970-01-01)
	days := daysFromDate(year, month, day)
	secs := int64(days)*86400 + int64(hour)*3600 + int64(min)*60 + int64(sec)

	// clocky.Time is nanoseconds since epoch
	return clocky.Time(secs*1_000_000_000 + nsec), i, nil
}
