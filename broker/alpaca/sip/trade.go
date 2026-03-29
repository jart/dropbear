package sip

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/symbol"
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

// Parse parses a SIP trade message.
// This is a strict parser that assumes Alpaca sent minified JSON with all and only the expected fields.
// Returns index past closing '}' on success.
func (t *Trade) Parse(data []byte) (int, error) {
	const (
		gotTape = 1 << iota
		gotExchange
		gotTimestamp
		gotSymbol
		gotID
		gotPrice
		gotSize
		gotConditions
	)
	var err error
	g := 0
	i := 1
	n := len(data)
	if n < 2 || data[0] != '{' {
		return 0, ErrParsingError
	}
	for i < n {
		b := data[i]
		i++
		switch b {
		case '}':
			if g != gotTape+
				gotExchange+
				gotTimestamp+
				gotSymbol+
				gotID+
				gotPrice+
				gotSize+
				gotConditions {
				return 0, ErrMissingField
			}
			return i, nil
		case ',':
			// do nothing
		case '"':
			if i >= n {
				return 0, ErrParsingError
			}
			b := data[i]
			i++
			switch b {
			case 'T': // "T":"t"
				if i+5 > n {
					return 0, ErrParsingError
				}
				if data[i] != '"' || data[i+1] != ':' || data[i+2] != '"' || data[i+4] != '"' {
					return 0, ErrParsingError
				}
				if data[i+3] != 't' {
					return 0, ErrTypeMismatch
				}
				t.Type = MessageTypeTrade
				i += 5
			case 'S': // "S":"AAPL"
				if i+4 > n {
					return 0, ErrParsingError
				}
				if data[i] != '"' || data[i+1] != ':' || data[i+2] != '"' {
					return 0, ErrParsingError
				}
				i += 3
				j := i
				for j < n && data[j] != '"' {
					j++
				}
				if j >= n || j == i {
					return 0, ErrParsingError
				}
				sym, err := symbol.ParseBytes(data[i:j])
				if err != nil {
					return 0, err
				}
				t.Symbol = sym
				g |= gotSymbol
				i = j + 1
			case 'i': // "i":12345
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				t.TradeID, i = parseInt(data, i)
				g |= gotID
			case 'x': // "x":"Q"
				if i+5 > n {
					return 0, ErrParsingError
				}
				if data[i] != '"' || data[i+1] != ':' || data[i+2] != '"' || data[i+4] != '"' {
					return 0, ErrParsingError
				}
				t.Exchange = Exchange(data[i+3])
				g |= gotExchange
				i += 5
			case 'p': // "p":150.50
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				t.Price, i, err = parseDecimal(data, i)
				if err != nil {
					return 0, err
				}
				g |= gotPrice
			case 's': // "s":100
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				var x int64
				x, i = parseInt(data, i)
				if x < 0 {
					return 0, ErrNegativeTradeSize
				}
				if x > 0x7fffffffffffffff {
					return 0, ErrOverflow
				}
				t.Size = x
				g |= gotSize
			case 'c': // "c":["@"]
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				t.Conditions, i = parseTradeCond(data, i)
				g |= gotConditions
			case 't': // "t":"2026-01-06T20:12:59.99839309Z"
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				t.Timestamp, i, err = parseTimestamp(data, i)
				if err != nil {
					return 0, err
				}
				g |= gotTimestamp
			case 'z': // "z":"A"
				if i+5 > n {
					return 0, ErrParsingError
				}
				if data[i] != '"' || data[i+1] != ':' || data[i+2] != '"' || data[i+4] != '"' {
					return 0, ErrParsingError
				}
				t.Tape = Tape(data[i+3])
				g |= gotTape
				i += 5
			default:
				return 0, ErrUnrecognizedField
			}
		default:
			return 0, ErrParsingError
		}
	}
	return 0, ErrParsingError
}
