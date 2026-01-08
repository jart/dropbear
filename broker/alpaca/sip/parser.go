package sip

import (
	"dropbear/decimal"
)

// key2 packs two bytes into uint16 for switch dispatch
func key2(a, b byte) uint16 {
	return uint16(a) | uint16(b)<<8
}

func parseInt(data []byte, i int) (int64, int) {
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

func parseDecimal(data []byte, i int) (decimal.Decimal, int, error) {
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

func parseTradeCond(data []byte, i int) (TradeCond, int) {
	var result TradeCond
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

func parseQuoteCond(data []byte, i int) (QuoteCond, int) {
	var result QuoteCond
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
