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
	for i := 0; i < len(data); {
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
		key := data[keyStart:i]
		i++ // skip closing quote

		// Skip to value
		for i < len(data) && data[i] != ':' {
			i++
		}
		i++ // skip colon
		for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
			i++
		}

		// Parse value based on key
		switch {
		case len(key) == 1 && key[0] == 'T': // Type
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
		case len(key) == 1 && key[0] == 'S': // Symbol
			if i < len(data) && data[i] == '"' {
				i++
				start := i
				for i < len(data) && data[i] != '"' {
					i++
				}
				t.Symbol = InternSymbol(data[start:i])
				i++ // skip closing quote
			}
		case len(key) == 1 && key[0] == 'i': // TradeID
			t.TradeID, i = parseIntFast(data, i)
		case len(key) == 1 && key[0] == 'x': // Exchange
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
		case len(key) == 1 && key[0] == 'p': // Price
			t.Price, i, err = parseDecimalFast(data, i)
			if err != nil {
				return t, err
			}
		case len(key) == 1 && key[0] == 's': // Size
			t.Size, i = parseIntFast(data, i)
		case len(key) == 1 && key[0] == 'c': // Conditions
			t.Conditions, i = parseTradeCondFast(data, i)
		case len(key) == 1 && key[0] == 't': // Timestamp
			t.Timestamp, i, err = parseTimestampFast(data, i)
			if err != nil {
				return t, err
			}
		case len(key) == 1 && key[0] == 'z': // Tape
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
			// Skip unknown field value
			i = skipValue(data, i)
		}
	}
	return t, nil
}

// ParseSIPQuoteFast parses a SIP quote message with minimal allocations.
func ParseSIPQuoteFast(data []byte) (SIPQuote, error) {
	var q SIPQuote
	var err error
	for i := 0; i < len(data); {
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
		key := data[keyStart:i]
		i++ // skip closing quote

		// Skip to value
		for i < len(data) && data[i] != ':' {
			i++
		}
		i++ // skip colon
		for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
			i++
		}

		// Parse value based on key
		switch {
		case len(key) == 1 && key[0] == 'T': // Type
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
		case len(key) == 1 && key[0] == 'S': // Symbol
			if i < len(data) && data[i] == '"' {
				i++
				start := i
				for i < len(data) && data[i] != '"' {
					i++
				}
				q.Symbol = InternSymbol(data[start:i])
				i++ // skip closing quote
			}
		case len(key) == 2 && key[0] == 'a' && key[1] == 'x': // AskExchange
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
		case len(key) == 2 && key[0] == 'a' && key[1] == 'p': // AskPrice
			q.AskPrice, i, err = parseDecimalFast(data, i)
			if err != nil {
				return q, err
			}
		case len(key) == 2 && key[0] == 'a' && key[1] == 's': // AskSize
			q.AskSize, i = parseIntFast(data, i)
		case len(key) == 2 && key[0] == 'b' && key[1] == 'x': // BidExchange
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
		case len(key) == 2 && key[0] == 'b' && key[1] == 'p': // BidPrice
			q.BidPrice, i, err = parseDecimalFast(data, i)
			if err != nil {
				return q, err
			}
		case len(key) == 2 && key[0] == 'b' && key[1] == 's': // BidSize
			q.BidSize, i = parseIntFast(data, i)
		case len(key) == 1 && key[0] == 'c': // Conditions
			q.Conditions, i = parseQuoteCondFast(data, i)
		case len(key) == 1 && key[0] == 't': // Timestamp
			q.Timestamp, i, err = parseTimestampFast(data, i)
			if err != nil {
				return q, err
			}
		case len(key) == 1 && key[0] == 'z': // Tape
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
	}
	return q, nil
}

// ParseSIPBarFast parses a SIP bar message with minimal allocations.
func ParseSIPBarFast(data []byte) (SIPBar, error) {
	var b SIPBar
	var err error
	for i := 0; i < len(data); {
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
		key := data[keyStart:i]
		i++ // skip closing quote

		// Skip to value
		for i < len(data) && data[i] != ':' {
			i++
		}
		i++ // skip colon
		for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
			i++
		}

		// Parse value based on key
		switch {
		case len(key) == 1 && key[0] == 'T': // Type
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
		case len(key) == 1 && key[0] == 'S': // Symbol
			if i < len(data) && data[i] == '"' {
				i++
				start := i
				for i < len(data) && data[i] != '"' {
					i++
				}
				b.Symbol = InternSymbol(data[start:i])
				i++ // skip closing quote
			}
		case len(key) == 1 && key[0] == 'o': // Open
			b.Open, i, err = parseDecimalFast(data, i)
			if err != nil {
				return b, err
			}
		case len(key) == 1 && key[0] == 'h': // High
			b.High, i, err = parseDecimalFast(data, i)
			if err != nil {
				return b, err
			}
		case len(key) == 1 && key[0] == 'l': // Low
			b.Low, i, err = parseDecimalFast(data, i)
			if err != nil {
				return b, err
			}
		case len(key) == 1 && key[0] == 'c': // Close
			b.Close, i, err = parseDecimalFast(data, i)
			if err != nil {
				return b, err
			}
		case len(key) == 1 && key[0] == 'v': // Volume
			b.Volume, i = parseIntFast(data, i)
		case len(key) == 2 && key[0] == 'v' && key[1] == 'w': // VWAP
			b.VWAP, i, err = parseDecimalFast(data, i)
			if err != nil {
				return b, err
			}
		case len(key) == 1 && key[0] == 'n': // NumTrades
			b.NumTrades, i = parseIntFast(data, i)
		case len(key) == 1 && key[0] == 't': // Timestamp
			b.Timestamp, i, err = parseTimestampFast(data, i)
			if err != nil {
				return b, err
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
	d := decimal.Parse(string(data[start:i])) // panics on invalid input
	if quoted && i < len(data) && data[i] == '"' {
		i++
	}
	return d, i, nil
}

func parseTimestampFast(data []byte, i int) (clocky.Time, int, error) {
	if i >= len(data) || data[i] != '"' {
		return 0, i, nil
	}
	i++ // skip opening quote
	start := i
	for i < len(data) && data[i] != '"' {
		i++
	}
	t, err := clocky.ParseTime(string(data[start:i]))
	if i < len(data) {
		i++ // skip closing quote
	}
	return t, i, err
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
			if data[i] == '{' {
				depth++
			} else if data[i] == '}' {
				depth--
			} else if data[i] == '"' {
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
	for i := 0; i < len(data); {
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
		key := data[keyStart:i]
		i++ // skip closing quote

		// Skip to value
		for i < len(data) && data[i] != ':' {
			i++
		}
		i++ // skip colon
		for i < len(data) && (data[i] == ' ' || data[i] == '\t') {
			i++
		}

		// Parse value based on key
		switch {
		case len(key) == 1 && key[0] == 'T': // Type
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
		case len(key) == 1 && key[0] == 'S': // Symbol
			if i < len(data) && data[i] == '"' {
				i++
				start := i
				for i < len(data) && data[i] != '"' {
					i++
				}
				s.Symbol = InternSymbol(data[start:i])
				i++ // skip closing quote
			}
		case len(key) == 2 && key[0] == 's' && key[1] == 'c': // Status Code
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
		case len(key) == 2 && key[0] == 's' && key[1] == 'm': // Status Message
			if i < len(data) && data[i] == '"' {
				i++
				start := i
				for i < len(data) && data[i] != '"' {
					if data[i] == '\\' {
						i++
					}
					i++
				}
				s.Message = string(data[start:i]) // Message not interned
				i++                               // skip closing quote
			}
		case len(key) == 2 && key[0] == 'r' && key[1] == 'c': // Reason Code
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
				i++ // skip closing quote
			}
		case len(key) == 1 && key[0] == 't': // Timestamp
			s.Timestamp, i, err = parseTimestampFast(data, i)
			if err != nil {
				return s, err
			}
		default:
			i = skipValue(data, i)
		}
	}
	return s, nil
}
