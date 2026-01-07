package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
)

// InternSymbol returns the interned symbol string from the Assets map.
// This avoids allocation by using Go's map[string([]byte)] optimization.
func InternSymbol(b []byte) string {
	if asset, ok := Assets[string(b)]; ok {
		return asset.Symbol
	}
	return string(b)
}

// ParseSIPTradeFast parses a SIP trade message with minimal allocations.
// Uses symbol interning and avoids json.Unmarshal overhead.
func ParseSIPTradeFast(data []byte) (SIPTrade, error) {
	var t SIPTrade
	var err error
	i := 0
	for i < len(data) {
		// Find field name
		for i < len(data) && data[i] != '"' {
			i++
		}
		if i >= len(data) {
			break
		}
		i++ // skip opening quote
		keyStart := i
		for i < len(data) && data[i] != '"' {
			i++
		}
		if i >= len(data) {
			break
		}
		keyLen := i - keyStart
		keyByte := data[keyStart]
		i++ // skip closing quote

		// Skip to value
		for i < len(data) && data[i] != ':' {
			i++
		}
		i++ // skip colon
		for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
			i++
		}

		// All trade fields are 1-char keys
		if keyLen != 1 {
			i = skipValue(data, i)
			continue
		}

		switch keyByte {
		case 'T': // Type
			if i < len(data) && data[i] == '"' {
				i++
				if i < len(data) {
					t.Type = SIPMessageType(data[i])
					i++
				}
				if i < len(data) && data[i] == '"' {
					i++
				}
			}
		case 'S': // Symbol
			if i < len(data) && data[i] == '"' {
				i++
				start := i
				for i < len(data) && data[i] != '"' {
					i++
				}
				t.Symbol = InternSymbol(data[start:i])
				i++
			}
		case 'i': // TradeID
			t.TradeID, i = parseIntFast(data, i)
		case 'x': // Exchange
			if i < len(data) && data[i] == '"' {
				i++
				if i < len(data) {
					t.Exchange = SIPExchange(data[i])
					i++
				}
				if i < len(data) && data[i] == '"' {
					i++
				}
			}
		case 'p': // Price
			t.Price, i, err = parseDecimalFast(data, i)
			if err != nil {
				return t, err
			}
		case 's': // Size
			t.Size, i = parseIntFast(data, i)
		case 'c': // Conditions
			t.Conditions, i = parseTradeCondFast(data, i)
		case 't': // Timestamp
			t.Timestamp, i, err = parseTimestampFast(data, i)
			if err != nil {
				return t, err
			}
		case 'z': // Tape
			if i < len(data) && data[i] == '"' {
				i++
				if i < len(data) {
					t.Tape = SIPTape(data[i])
					i++
				}
				if i < len(data) && data[i] == '"' {
					i++
				}
			}
		default:
			i = skipValue(data, i)
		}
	}
	return t, nil
}

// key2 packs two bytes into uint16 for switch dispatch
func key2(a, b byte) uint16 {
	return uint16(a) | uint16(b)<<8
}

// ParseSIPQuoteFast parses a SIP quote message with minimal allocations.
func ParseSIPQuoteFast(data []byte) (SIPQuote, error) {
	var q SIPQuote
	var err error
	i := 0
	for i < len(data) {
		// Find field name
		for i < len(data) && data[i] != '"' {
			i++
		}
		if i >= len(data) {
			break
		}
		i++ // skip opening quote
		keyStart := i
		for i < len(data) && data[i] != '"' {
			i++
		}
		if i >= len(data) {
			break
		}
		keyLen := i - keyStart
		i++ // skip closing quote

		// Skip to value
		for i < len(data) && data[i] != ':' {
			i++
		}
		i++ // skip colon
		for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
			i++
		}

		switch keyLen {
		case 1:
			switch data[keyStart] {
			case 'T': // Type
				if i < len(data) && data[i] == '"' {
					i++
					if i < len(data) {
						q.Type = SIPMessageType(data[i])
						i++
					}
					if i < len(data) && data[i] == '"' {
						i++
					}
				}
			case 'S': // Symbol
				if i < len(data) && data[i] == '"' {
					i++
					start := i
					for i < len(data) && data[i] != '"' {
						i++
					}
					q.Symbol = InternSymbol(data[start:i])
					i++
				}
			case 'c': // Conditions
				q.Conditions, i = parseQuoteCondFast(data, i)
			case 't': // Timestamp
				q.Timestamp, i, err = parseTimestampFast(data, i)
				if err != nil {
					return q, err
				}
			case 'z': // Tape
				if i < len(data) && data[i] == '"' {
					i++
					if i < len(data) {
						q.Tape = SIPTape(data[i])
						i++
					}
					if i < len(data) && data[i] == '"' {
						i++
					}
				}
			default:
				i = skipValue(data, i)
			}
		case 2:
			switch key2(data[keyStart], data[keyStart+1]) {
			case key2('a', 'x'): // AskExchange
				if i < len(data) && data[i] == '"' {
					i++
					if i < len(data) {
						q.AskExchange = SIPExchange(data[i])
						i++
					}
					if i < len(data) && data[i] == '"' {
						i++
					}
				}
			case key2('a', 'p'): // AskPrice
				q.AskPrice, i, err = parseDecimalFast(data, i)
				if err != nil {
					return q, err
				}
			case key2('a', 's'): // AskSize
				q.AskSize, i = parseIntFast(data, i)
			case key2('b', 'x'): // BidExchange
				if i < len(data) && data[i] == '"' {
					i++
					if i < len(data) {
						q.BidExchange = SIPExchange(data[i])
						i++
					}
					if i < len(data) && data[i] == '"' {
						i++
					}
				}
			case key2('b', 'p'): // BidPrice
				q.BidPrice, i, err = parseDecimalFast(data, i)
				if err != nil {
					return q, err
				}
			case key2('b', 's'): // BidSize
				q.BidSize, i = parseIntFast(data, i)
			default:
				i = skipValue(data, i)
			}
		default:
			i = skipValue(data, i)
		}
	}
	return q, nil
}

// ParseSIPBarFast parses a SIP bar message with minimal allocations.
func ParseSIPBarFast(data []byte) (SIPBar, error) {
	var b SIPBar
	var err error
	i := 0
	for i < len(data) {
		// Find field name
		for i < len(data) && data[i] != '"' {
			i++
		}
		if i >= len(data) {
			break
		}
		i++ // skip opening quote
		keyStart := i
		for i < len(data) && data[i] != '"' {
			i++
		}
		if i >= len(data) {
			break
		}
		keyLen := i - keyStart
		i++ // skip closing quote

		// Skip to value
		for i < len(data) && data[i] != ':' {
			i++
		}
		i++ // skip colon
		for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
			i++
		}
		switch keyLen {
		case 1:
			switch data[keyStart] {
			case 'T': // Type
				if i < len(data) && data[i] == '"' {
					i++
					if i < len(data) {
						b.Type = SIPMessageType(data[i])
						i++
					}
					if i < len(data) && data[i] == '"' {
						i++
					}
				}
			case 'S': // Symbol
				if i < len(data) && data[i] == '"' {
					i++
					start := i
					for i < len(data) && data[i] != '"' {
						i++
					}
					b.Symbol = InternSymbol(data[start:i])
					i++
				}
			case 'o': // Open
				b.Open, i, err = parseDecimalFast(data, i)
				if err != nil {
					return b, err
				}
			case 'h': // High
				b.High, i, err = parseDecimalFast(data, i)
				if err != nil {
					return b, err
				}
			case 'l': // Low
				b.Low, i, err = parseDecimalFast(data, i)
				if err != nil {
					return b, err
				}
			case 'c': // Close
				b.Close, i, err = parseDecimalFast(data, i)
				if err != nil {
					return b, err
				}
			case 'v': // Volume
				b.Volume, i = parseIntFast(data, i)
			case 'n': // NumTrades
				b.NumTrades, i = parseIntFast(data, i)
			case 't': // Timestamp
				b.Timestamp, i, err = parseTimestampFast(data, i)
				if err != nil {
					return b, err
				}
			default:
				i = skipValue(data, i)
			}
		case 2:
			if data[keyStart] == 'v' && data[keyStart+1] == 'w' { // VWAP
				b.VWAP, i, err = parseDecimalFast(data, i)
				if err != nil {
					return b, err
				}
			} else {
				i = skipValue(data, i)
			}
		default:
			i = skipValue(data, i)
		}
	}
	return b, nil
}

func parseIntFast(data []byte, i int) (int64, int) {
	var n int64
	neg := false
	if i < len(data) && data[i] == '-' {
		neg = true
		i++
	}
	for i < len(data) && data[i] >= '0' && data[i] <= '9' {
		n = n*10 + int64(data[i]-'0')
		i++
	}
	if neg {
		n = -n
	}
	return n, i
}

func parseDecimalFast(data []byte, i int) (decimal.Decimal, int, error) {
	// Skip opening quote if present
	quoted := false
	if i < len(data) && data[i] == '"' {
		quoted = true
		i++
	}
	start := i
	for i < len(data) && (data[i] == '-' || data[i] == '.' || (data[i] >= '0' && data[i] <= '9')) {
		i++
	}
	d, err := decimal.ParseBytes(data[start:i])
	if err != nil {
		return d, i, err
	}
	if quoted && i < len(data) && data[i] == '"' {
		i++
	}
	return d, i, nil
}

// parseTimestampFast parses RFC3339 timestamps from trusted Alpaca SIP data.
// Expects formats like:
//   - "2026-01-06T20:12:59.99839309Z" (with nanoseconds)
//   - "2026-01-06T20:12:00Z" (without fractional seconds)
//
// Returns microseconds since Unix epoch as clocky.Time.
func parseTimestampFast(data []byte, i int) (clocky.Time, int, error) {
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
	var usec int64
	if len(ts) > 20 && ts[19] == '.' {
		// Parse up to 6 digits for microseconds
		frac := ts[20 : len(ts)-1] // exclude trailing 'Z'
		var mult int64 = 100000
		for j := 0; j < len(frac) && j < 6; j++ {
			usec += int64(frac[j]-'0') * mult
			mult /= 10
		}
	}

	// Convert to Unix timestamp (days since epoch * 86400 + time of day)
	// Using the formula for days since Unix epoch (1970-01-01)
	days := daysFromDate(year, month, day)
	secs := int64(days)*86400 + int64(hour)*3600 + int64(min)*60 + int64(sec)

	// clocky.Time is microseconds since epoch
	return clocky.Time(secs*1000000 + usec), i, nil
}

// daysFromDate returns days since Unix epoch (1970-01-01) for a given date.
// Uses the formula from Howard Hinnant's date algorithms.
func daysFromDate(year, month, day int) int64 {
	// Shift March to month 0 to simplify leap year handling
	if month <= 2 {
		year--
		month += 12
	}
	month -= 3

	// Era and year-of-era
	era := year / 400
	yoe := year - era*400
	// Day-of-year
	doy := (153*month+2)/5 + day - 1
	// Day-of-era
	doe := yoe*365 + yoe/4 - yoe/100 + doy

	// Days since epoch (Unix epoch is 1970-01-01 = day 719468 from year 0)
	return int64(era)*146097 + int64(doe) - 719468
}

func parseTradeCondFast(data []byte, i int) (SIPTradeCond, int) {
	var result SIPTradeCond
	if i >= len(data) || data[i] != '[' {
		return result, i
	}
	i++ // skip '['
	for i < len(data) {
		for i < len(data) && jsonArraySkip[data[i]] {
			i++
		}
		if i >= len(data) || data[i] == ']' {
			i++
			break
		}
		if data[i] == '"' {
			i++
			if i < len(data) {
				result |= tradeCondFromChar[data[i]]
				i++
			}
			if i < len(data) && data[i] == '"' {
				i++
			}
		}
	}
	return result, i
}

func parseQuoteCondFast(data []byte, i int) (SIPQuoteCond, int) {
	var result SIPQuoteCond
	if i >= len(data) || data[i] != '[' {
		return result, i
	}
	i++ // skip '['
	for i < len(data) {
		for i < len(data) && jsonArraySkip[data[i]] {
			i++
		}
		if i >= len(data) || data[i] == ']' {
			i++
			break
		}
		if data[i] == '"' {
			i++
			if i < len(data) {
				result |= quoteCondFromChar[data[i]]
				i++
			}
			if i < len(data) && data[i] == '"' {
				i++
			}
		}
	}
	return result, i
}

func skipValue(data []byte, i int) int {
	if i >= len(data) {
		return i
	}
	switch data[i] {
	case '"': // string
		i++
		for i < len(data) && data[i] != '"' {
			if data[i] == '\\' {
				i++
			}
			i++
		}
		if i < len(data) {
			i++
		}
	case '[': // array
		depth := 1
		i++
		for i < len(data) && depth > 0 {
			switch data[i] {
			case '[':
				depth++
			case ']':
				depth--
			case '"':
				i++
				for i < len(data) && data[i] != '"' {
					if data[i] == '\\' {
						i++
					}
					i++
				}
			}
			i++
		}
	case '{': // object
		depth := 1
		i++
		for i < len(data) && depth > 0 {
			switch data[i] {
			case '{':
				depth++
			case '}':
				depth--
			case '"':
				i++
				for i < len(data) && data[i] != '"' {
					if data[i] == '\\' {
						i++
					}
					i++
				}
			}
			i++
		}
	default: // number, bool, null
		for i < len(data) && data[i] != ',' && data[i] != '}' && data[i] != ']' {
			i++
		}
	}
	return i
}

// ParseSIPStatusFast parses a SIP status message with minimal allocations.
func ParseSIPStatusFast(data []byte) (SIPStatus, error) {
	var s SIPStatus
	var err error
	i := 0
	for i < len(data) {
		// Find field name
		for i < len(data) && data[i] != '"' {
			i++
		}
		if i >= len(data) {
			break
		}
		i++ // skip opening quote
		keyStart := i
		for i < len(data) && data[i] != '"' {
			i++
		}
		if i >= len(data) {
			break
		}
		keyLen := i - keyStart
		i++ // skip closing quote

		// Skip to value
		for i < len(data) && data[i] != ':' {
			i++
		}
		i++ // skip colon
		for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
			i++
		}

		switch keyLen {
		case 1:
			switch data[keyStart] {
			case 'T': // Type
				if i < len(data) && data[i] == '"' {
					i++
					if i < len(data) {
						s.Type = SIPMessageType(data[i])
						i++
					}
					if i < len(data) && data[i] == '"' {
						i++
					}
				}
			case 'S': // Symbol
				if i < len(data) && data[i] == '"' {
					i++
					start := i
					for i < len(data) && data[i] != '"' {
						i++
					}
					s.Symbol = InternSymbol(data[start:i])
					i++
				}
			case 't': // Timestamp
				s.Timestamp, i, err = parseTimestampFast(data, i)
				if err != nil {
					return s, err
				}
			default:
				i = skipValue(data, i)
			}
		case 2:
			switch key2(data[keyStart], data[keyStart+1]) {
			case key2('s', 'c'): // Status Code
				if i < len(data) && data[i] == '"' {
					i++
					if i < len(data) {
						s.Code = SIPStatusCode(data[i])
						i++
					}
					if i < len(data) && data[i] == '"' {
						i++
					}
				}
			case key2('s', 'm'): // Status Message
				if i < len(data) && data[i] == '"' {
					i++
					start := i
					for i < len(data) && data[i] != '"' {
						if data[i] == '\\' {
							i++
						}
						i++
					}
					s.Message = string(data[start:i])
					i++
				}
			case key2('r', 'c'): // Reason Code
				if i < len(data) && data[i] == '"' {
					i++
					start := i
					for i < len(data) && data[i] != '"' {
						i++
					}
					s.Reason, err = ParseReasonCode(data[start:i])
					if err != nil {
						return s, err
					}
					i++
				}
			default:
				i = skipValue(data, i)
			}
		default:
			i = skipValue(data, i)
		}
	}
	return s, nil
}
