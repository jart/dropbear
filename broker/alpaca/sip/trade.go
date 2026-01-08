package sip

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/symbol"
)

// Trade represents a trade execution from the SIP feed.
type Trade struct {
	Type       MessageType     `json:"T"` // MessageTypeTrade
	Tape       Tape            `json:"z"` // tape designation (A, B, C)
	Exchange   Exchange        `json:"x"` // exchange code
	Timestamp  clocky.Time     `json:"t"` // RFC-3339 timestamp
	Symbol     symbol.Symbol   `json:"S"` // stock symbol
	TradeID    int64           `json:"i"` // unique trade identifier
	Price      decimal.Decimal `json:"p"` // execution price
	Size       int64           `json:"s"` // quantity traded
	Conditions TradeCond       `json:"c"` // trade conditions (bitmask)
}

// IsRegularSale returns true if this is a normal trade (no special conditions).
func (t *Trade) IsRegularSale() bool {
	return t.Conditions.IsRegularSale() || t.Conditions == 0
}

// IsOddLot returns true if this is an odd lot trade.
func (t *Trade) IsOddLot() bool {
	return t.Conditions.IsOddLot()
}

// IsExtendedHours returns true if this is an extended hours trade.
func (t *Trade) IsExtendedHours() bool {
	return t.Conditions.IsExtendedHours()
}

// ParseTrade parses a SIP trade message with minimal allocations.
// Uses symbol interning and avoids json.Unmarshal overhead.
func ParseTrade(data []byte) (Trade, error) {
	var t Trade
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
					t.Type = MessageType(data[i])
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
				sym, err := symbol.ParseBytes(data[start:i])
				if err != nil {
					return t, err
				}
				t.Symbol = sym
				i++
			}
		case 'i': // TradeID
			t.TradeID, i = parseInt(data, i)
		case 'x': // Exchange
			if i < len(data) && data[i] == '"' {
				i++
				if i < len(data) {
					t.Exchange = Exchange(data[i])
					i++
				}
				if i < len(data) && data[i] == '"' {
					i++
				}
			}
		case 'p': // Price
			t.Price, i, err = parseDecimal(data, i)
			if err != nil {
				return t, err
			}
		case 's': // Size
			t.Size, i = parseInt(data, i)
		case 'c': // Conditions
			t.Conditions, i = parseTradeCond(data, i)
		case 't': // Timestamp
			t.Timestamp, i, err = parseTimestamp(data, i)
			if err != nil {
				return t, err
			}
		case 'z': // Tape
			if i < len(data) && data[i] == '"' {
				i++
				if i < len(data) {
					t.Tape = Tape(data[i])
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
