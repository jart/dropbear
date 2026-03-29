package sip

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/symbol"
)

// LULD represents a Limit Up-Limit Down price band message.
//
// These messages are published by the SIP every 30 seconds to inform market
// participants of the current LULD price bands for a security. The bands define
// the price range within which trading is permitted.
//
// Example interpretation:
//
//	LULD{Symbol:"GOOG", LowerLimit:291.74, UpperLimit:356.57, Indicator:LULDIndicatorRepublished}
//
// This means GOOG is currently trading normally. If the price tries to go below
// $291.74 or above $356.57, the NBBO will enter a "Limit State". If it stays
// there for 15 seconds, trading pauses for 5 minutes.
//
// When Indicator is A or B, the price limits are valid.
// When Indicator is C-J (limit state transitions), the price limits are zero.
type LULD struct {
	Type       MessageType     `json:"T"` // MessageTypeLULD
	Tape       Tape            `json:"z"` // tape identifier
	Indicator  LULDIndicator   `json:"i"` // LULD indicator (see LULDIndicator docs)
	Timestamp  clocky.Time     `json:"t"` // RFC-3339 timestamp
	Symbol     symbol.Symbol   `json:"S"` // stock symbol
	UpperLimit decimal.Decimal `json:"u"` // upper price limit (zero if Indicator is C-J)
	LowerLimit decimal.Decimal `json:"d"` // lower price limit (zero if Indicator is C-J)
	_          [16]byte        // padding for uniform struct size
}

// Parse parses a SIP LULD (Limit Up-Limit Down) message.
// This is a strict parser that assumes Alpaca sent minified JSON with all and only the expected fields.
// Returns index past closing '}' on success.
func (l *LULD) Parse(data []byte) (int, error) {
	const (
		gotTape = 1 << iota
		gotIndicator
		gotTimestamp
		gotSymbol
		gotUpperLimit
		gotLowerLimit
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
				gotIndicator+
				gotTimestamp+
				gotSymbol+
				gotUpperLimit+
				gotLowerLimit {
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
			case 'T': // "T":"l"
				if i+5 > n {
					return 0, ErrParsingError
				}
				if data[i] != '"' || data[i+1] != ':' || data[i+2] != '"' || data[i+4] != '"' {
					return 0, ErrParsingError
				}
				if data[i+3] != 'l' {
					return 0, ErrTypeMismatch
				}
				l.Type = MessageTypeLULD
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
				l.Symbol = sym
				g |= gotSymbol
				i = j + 1
			case 'u': // "u":356.57
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				l.UpperLimit, i, err = parseDecimal(data, i)
				if err != nil {
					return 0, err
				}
				g |= gotUpperLimit
			case 'd': // "d":291.74
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				l.LowerLimit, i, err = parseDecimal(data, i)
				if err != nil {
					return 0, err
				}
				g |= gotLowerLimit
			case 'i': // "i":"A"
				if i+5 > n {
					return 0, ErrParsingError
				}
				if data[i] != '"' || data[i+1] != ':' || data[i+2] != '"' || data[i+4] != '"' {
					return 0, ErrParsingError
				}
				l.Indicator = LULDIndicator(data[i+3])
				g |= gotIndicator
				i += 5
			case 't': // "t":"2026-01-06T20:12:59.99839309Z"
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				l.Timestamp, i, err = parseTimestamp(data, i)
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
				l.Tape = Tape(data[i+3])
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
