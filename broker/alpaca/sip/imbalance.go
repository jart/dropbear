package sip

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/symbol"
)

// Imbalance represents an order imbalance message from the SIP feed.
// These are sent during limit-up/limit-down and trading halts.
type Imbalance struct {
	Type      MessageType     `json:"T"` // MessageTypeImbalance
	Tape      Tape            `json:"z"` // tape designation (A, B, C)
	_         [6]byte         //
	Timestamp clocky.Time     `json:"t"` // RFC-3339 timestamp
	Symbol    symbol.Symbol   `json:"S"` // stock symbol
	Price     decimal.Decimal `json:"p"` // imbalance price
	_         [24]byte        // padding to match Message union size (56 bytes)
}

// Parse parses a SIP imbalance message.
// Returns index past closing '}' on success.
func (m *Imbalance) Parse(data []byte) (int, error) {
	const (
		gotTape = 1 << iota
		gotTimestamp
		gotSymbol
		gotPrice
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
				gotTimestamp+
				gotSymbol+
				gotPrice {
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
			case 'T': // "T":"i"
				if i+5 > n {
					return 0, ErrParsingError
				}
				if data[i] != '"' || data[i+1] != ':' || data[i+2] != '"' || data[i+4] != '"' {
					return 0, ErrParsingError
				}
				if data[i+3] != 'i' {
					return 0, ErrTypeMismatch
				}
				m.Type = MessageTypeImbalance
				i += 5
			case 'S': // "S":"INAQU"
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
				m.Symbol = sym
				g |= gotSymbol
				i = j + 1
			case 'p': // "p":9.12
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				m.Price, i, err = parseDecimal(data, i)
				if err != nil {
					return 0, err
				}
				g |= gotPrice
			case 'z': // "z":"C"
				if i+5 > n {
					return 0, ErrParsingError
				}
				if data[i] != '"' || data[i+1] != ':' || data[i+2] != '"' || data[i+4] != '"' {
					return 0, ErrParsingError
				}
				m.Tape = Tape(data[i+3])
				g |= gotTape
				i += 5
			case 't': // "t":"2024-12-13T19:58:09.242138635Z"
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				m.Timestamp, i, err = parseTimestamp(data, i)
				if err != nil {
					return 0, err
				}
				g |= gotTimestamp
			default:
				return 0, ErrUnrecognizedField
			}
		default:
			return 0, ErrParsingError
		}
	}
	return 0, ErrParsingError
}
