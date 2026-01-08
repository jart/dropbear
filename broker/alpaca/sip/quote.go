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
	AskSize     int64           `json:"as"` // ask size
	BidSize     int64           `json:"bs"` // bid size
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

// ParseQuote parses a SIP quote message with minimal allocations.
func ParseQuote(data []byte) (Quote, error) {
	var q Quote
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
						q.Type = MessageType(data[i])
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
						return q, err
					}
					q.Symbol = sym
					i++
				}
			case 'c': // Conditions
				q.Conditions, i = parseQuoteCond(data, i)
			case 't': // Timestamp
				q.Timestamp, i, err = parseTimestamp(data, i)
				if err != nil {
					return q, err
				}
			case 'z': // Tape
				if i < len(data) && data[i] == '"' {
					i++
					if i < len(data) {
						q.Tape = Tape(data[i])
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
						q.AskExchange = Exchange(data[i])
						i++
					}
					if i < len(data) && data[i] == '"' {
						i++
					}
				}
			case key2('a', 'p'): // AskPrice
				q.AskPrice, i, err = parseDecimal(data, i)
				if err != nil {
					return q, err
				}
			case key2('a', 's'): // AskSize
				q.AskSize, i = parseInt(data, i)
			case key2('b', 'x'): // BidExchange
				if i < len(data) && data[i] == '"' {
					i++
					if i < len(data) {
						q.BidExchange = Exchange(data[i])
						i++
					}
					if i < len(data) && data[i] == '"' {
						i++
					}
				}
			case key2('b', 'p'): // BidPrice
				q.BidPrice, i, err = parseDecimal(data, i)
				if err != nil {
					return q, err
				}
			case key2('b', 's'): // BidSize
				q.BidSize, i = parseInt(data, i)
			default:
				i = skipValue(data, i)
			}
		default:
			i = skipValue(data, i)
		}
	}
	return q, nil
}
