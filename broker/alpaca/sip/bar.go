package sip

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/symbol"
	"errors"
)

var ErrShortOverflow = errors.New("bar price overflow: value cannot fit in decimal.Short")

// Bar represents an aggregated bar from the SIP feed.
type Bar struct {
	Type      MessageType     `json:"T"`  // MessageTypeBar, MessageTypeDailyBar, or MessageTypeUpdatedBar
	NumTrades uint32          `json:"n"`  // number of trades
	Timestamp clocky.Time     `json:"t"`  // RFC-3339 timestamp
	Symbol    symbol.Symbol   `json:"S"`  // stock symbol
	Open      decimal.Short   `json:"o"`  // opening price
	High      decimal.Short   `json:"h"`  // highest price
	Low       decimal.Short   `json:"l"`  // lowest price
	Close     decimal.Short   `json:"c"`  // closing price
	Volume    int64           `json:"v"`  // total volume
	VWAP      decimal.Decimal `json:"vw"` // volume-weighted average price
}

// Parse parses a SIP bar message.
// This is a strict parser that assumes Alpaca sent minified JSON with all and only the expected fields.
// Returns index past closing '}' on success.
func (b *Bar) Parse(data []byte) (int, error) {
	const (
		gotTimestamp = 1 << iota
		gotSymbol
		gotOpen
		gotHigh
		gotLow
		gotClose
		gotVolume
		gotVWAP
		gotNumTrades
	)
	var err, punt error
	var tmp decimal.Decimal
	g := 0
	i := 1
	n := len(data)
	if n < 2 || data[0] != '{' {
		return 0, ErrParsingError
	}
	for i < n {
		c := data[i]
		i++
		switch c {
		case '}':
			if g != gotTimestamp+
				gotSymbol+
				gotOpen+
				gotHigh+
				gotLow+
				gotClose+
				gotVolume+
				gotVWAP+
				gotNumTrades {
				return 0, ErrMissingField
			}
			return i, punt
		case ',':
			// do nothing
		case '"':
			if i >= n {
				return 0, ErrParsingError
			}
			c := data[i]
			i++
			switch c {
			case 'T': // "T":"b", "T":"d", or "T":"u"
				if i+5 > n {
					return 0, ErrParsingError
				}
				if data[i] != '"' || data[i+1] != ':' || data[i+2] != '"' || data[i+4] != '"' {
					return 0, ErrParsingError
				}
				switch data[i+3] {
				case 'b':
					b.Type = MessageTypeBar
				case 'd':
					b.Type = MessageTypeDailyBar
				case 'u':
					b.Type = MessageTypeUpdatedBar
				default:
					return 0, ErrTypeMismatch
				}
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
				b.Symbol = sym
				g |= gotSymbol
				i = j + 1
			case 't': // "t":"2026-01-06T20:12:59.99839309Z"
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				b.Timestamp, i, err = parseTimestamp(data, i)
				if err != nil {
					return 0, err
				}
				g |= gotTimestamp
			case 'o': // "o":150.50
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				tmp, i, err = parseDecimal(data, i)
				if err != nil {
					return 0, err
				}
				if tmp.CanShort() {
					b.Open = tmp.Short()
				} else {
					punt = ErrShortOverflow
				}
				g |= gotOpen
			case 'h': // "h":152.00
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				tmp, i, err = parseDecimal(data, i)
				if err != nil {
					return 0, err
				}
				if tmp.CanShort() {
					b.High = tmp.Short()
				} else {
					punt = ErrShortOverflow
				}
				g |= gotHigh
			case 'l': // "l":149.50
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				tmp, i, err = parseDecimal(data, i)
				if err != nil {
					return 0, err
				}
				if tmp.CanShort() {
					b.Low = tmp.Short()
				} else {
					punt = ErrShortOverflow
				}
				g |= gotLow
			case 'c': // "c":151.00
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				tmp, i, err = parseDecimal(data, i)
				if err != nil {
					return 0, err
				}
				if tmp.CanShort() {
					b.Close = tmp.Short()
				} else {
					punt = ErrShortOverflow
				}
				g |= gotClose
			case 'n': // "n":1234
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				var x int64
				x, i = parseInt(data, i)
				if x != int64(uint32(x)) {
					return 0, ErrOverflow
				}
				b.NumTrades = uint32(x)
				g |= gotNumTrades
			case 'v': // "v":10000 or "vw":150.75
				if i >= n {
					return 0, ErrParsingError
				}
				switch data[i] {
				case '"': // "v":10000
					if i+2 > n || data[i+1] != ':' {
						return 0, ErrParsingError
					}
					i += 2
					b.Volume, i = parseInt(data, i)
					g |= gotVolume
				case 'w': // "vw":150.75
					if i+3 > n || data[i+1] != '"' || data[i+2] != ':' {
						return 0, ErrParsingError
					}
					i += 3
					b.VWAP, i, err = parseDecimal(data, i)
					if err != nil {
						return 0, err
					}
					g |= gotVWAP
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
