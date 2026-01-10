package sip

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/symbol"
)

// Quote represents a quote update from the SIP feed.
type Quote struct {
	Type        MessageType     `json:"T"`  // MessageTypeQuote
	Tape        Tape            `json:"z"`  // tape designation (A, B, C)
	AskExchange Exchange        `json:"ax"` // ask exchange code
	BidExchange Exchange        `json:"bx"` // bid exchange code
	Timestamp   clocky.Time     `json:"t"`  // RFC-3339 timestamp
	Symbol      symbol.Symbol   `json:"S"`  // stock symbol
	AskPrice    decimal.Decimal `json:"ap"` // ask price
	BidPrice    decimal.Decimal `json:"bp"` // bid price
	AskSize     int32           `json:"as"` // ask size
	BidSize     int32           `json:"bs"` // bid size
	Conditions  QuoteCond       `json:"c"`  // quote conditions (bitmask)
}

// Midpoint returns the midpoint between bid and ask prices.
func (q *Quote) Midpoint() decimal.Decimal {
	return q.BidPrice.Add(q.AskPrice).DivInt(2)
}

// Spread returns the bid-ask spread.
func (q *Quote) Spread() decimal.Decimal {
	return q.AskPrice.Sub(q.BidPrice)
}

// IsNonFirm returns true if any condition indicates a non-firm quote.
func (q *Quote) IsNonFirm() bool {
	return q.Conditions.IsNonFirm()
}

// IsSlow returns true if any condition indicates a slow quote.
func (q *Quote) IsSlow() bool {
	return q.Conditions.IsSlow()
}

// Parse parses a SIP quote message.
// This is a strict parser that assumes Alpaca sent minified JSON with all and only the expected fields.
// Returns index past closing '}' on success.
func (q *Quote) Parse(data []byte) (int, error) {
	const (
		gotTape = 1 << iota
		gotAskExchange
		gotBidExchange
		gotTimestamp
		gotSymbol
		gotAskPrice
		gotBidPrice
		gotAskSize
		gotBidSize
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
				gotAskExchange+
				gotBidExchange+
				gotTimestamp+
				gotSymbol+
				gotAskPrice+
				gotBidPrice+
				gotAskSize+
				gotBidSize+
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
			case 'T': // "T":"q"
				if i+5 > n {
					return 0, ErrParsingError
				}
				if data[i] != '"' || data[i+1] != ':' || data[i+2] != '"' || data[i+4] != '"' {
					return 0, ErrParsingError
				}
				if data[i+3] != 'q' {
					return 0, ErrTypeMismatch
				}
				q.Type = MessageTypeQuote
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
				q.Symbol = sym
				g |= gotSymbol
				i = j + 1
			case 't': // "t":"2026-01-06T20:12:59.99839309Z"
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				q.Timestamp, i, err = parseTimestamp(data, i)
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
				q.Tape = Tape(data[i+3])
				g |= gotTape
				i += 5
			case 'c': // "c":["R"]
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				q.Conditions, i = parseQuoteCond(data, i)
				g |= gotConditions
			case 'a': // "ap", "as", "ax"
				if i+3 > n || data[i+1] != '"' || data[i+2] != ':' {
					return 0, ErrParsingError
				}
				switch data[i] {
				case 'p': // "ap":150.50
					i += 3
					q.AskPrice, i, err = parseDecimal(data, i)
					if err != nil {
						return 0, err
					}
					g |= gotAskPrice
				case 's': // "as":100
					i += 3
					var x int64
					x, i = parseInt(data, i)
					if x < 0 {
						return 0, ErrNegativeQuoteSize
					}
					if x > 0x7fffffff {
						return 0, ErrOverflow
					}
					q.AskSize = int32(x)
					g |= gotAskSize
				case 'x': // "ax":"Q"
					if i+5 > n || data[i+3] != '"' || data[i+5] != '"' {
						return 0, ErrParsingError
					}
					q.AskExchange = Exchange(data[i+4])
					g |= gotAskExchange
					i += 6
				default:
					return 0, ErrUnrecognizedField
				}
			case 'b': // "bp", "bs", "bx"
				if i+3 > n || data[i+1] != '"' || data[i+2] != ':' {
					return 0, ErrParsingError
				}
				switch data[i] {
				case 'p': // "bp":150.49
					i += 3
					q.BidPrice, i, err = parseDecimal(data, i)
					if err != nil {
						return 0, err
					}
					g |= gotBidPrice
				case 's': // "bs":200
					i += 3
					var x int64
					x, i = parseInt(data, i)
					if x < 0 {
						return 0, ErrNegativeQuoteSize
					}
					if x > 0x7fffffff {
						return 0, ErrOverflow
					}
					q.BidSize = int32(x)
					g |= gotBidSize
				case 'x': // "bx":"N"
					if i+5 > n || data[i+3] != '"' || data[i+5] != '"' {
						return 0, ErrParsingError
					}
					q.BidExchange = Exchange(data[i+4])
					g |= gotBidExchange
					i += 6
				default:
					return 0, ErrUnrecognizedField
				}
			default:
				return 0, ErrUnrecognizedField
			}
		default:
			return 0, ErrParsingError
		}
	}
	return 0, ErrParsingError
}
