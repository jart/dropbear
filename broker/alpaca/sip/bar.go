package sip

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/symbol"
)

// Bar represents an aggregated bar from the SIP feed.
type Bar struct {
	Type      MessageType     `json:"T"`  // MessageTypeBar, MessageTypeDailyBar, or MessageTypeUpdatedBar
	NumTrades uint32          `json:"n"`  // number of trades
	Timestamp clocky.Time     `json:"t"`  // RFC-3339 timestamp
	Symbol    symbol.Symbol   `json:"S"`  // stock symbol
	Open      decimal.Decimal `json:"o"`  // opening price
	High      decimal.Decimal `json:"h"`  // highest price
	Low       decimal.Decimal `json:"l"`  // lowest price
	Close     decimal.Decimal `json:"c"`  // closing price
	Volume    int64           `json:"v"`  // total volume
	VWAP      decimal.Decimal `json:"vw"` // volume-weighted average price
}

// ParseBar parses a SIP bar message with minimal allocations.
func ParseBar(data []byte) (Bar, error) {
	var b Bar
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
						b.Type = MessageType(data[i])
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
						return b, err
					}
					b.Symbol = sym
					i++
				}
			case 'o': // Open
				b.Open, i, err = parseDecimal(data, i)
				if err != nil {
					return b, err
				}
			case 'h': // High
				b.High, i, err = parseDecimal(data, i)
				if err != nil {
					return b, err
				}
			case 'l': // Low
				b.Low, i, err = parseDecimal(data, i)
				if err != nil {
					return b, err
				}
			case 'c': // Close
				b.Close, i, err = parseDecimal(data, i)
				if err != nil {
					return b, err
				}
			case 'v': // Volume
				b.Volume, i = parseInt(data, i)
			case 'n': // NumTrades
				var x int64
				x, i = parseInt(data, i)
				if x != int64(uint32(x)) {
					return b, ErrOverflow
				}
				b.NumTrades = uint32(x)
			case 't': // Timestamp
				b.Timestamp, i, err = parseTimestamp(data, i)
				if err != nil {
					return b, err
				}
			default:
				i = skipValue(data, i)
			}
		case 2:
			if data[keyStart] == 'v' && data[keyStart+1] == 'w' { // VWAP
				b.VWAP, i, err = parseDecimal(data, i)
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
