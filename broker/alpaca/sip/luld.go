package sip

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/symbol"
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
}

// ParseLULD parses a SIP LULD (Limit Up-Limit Down) message with minimal allocations.
func ParseLULD(data []byte) (LULD, error) {
	var l LULD
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

		// All LULD fields are 1-char keys
		if keyLen != 1 {
			i = skipValue(data, i)
			continue
		}

		switch keyByte {
		case 'T': // Type
			if i < len(data) && data[i] == '"' {
				i++
				if i < len(data) {
					l.Type = MessageType(data[i])
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
					return l, err
				}
				l.Symbol = sym
				i++
			}
		case 'u': // Upper limit
			l.UpperLimit, i, err = parseDecimal(data, i)
			if err != nil {
				return l, err
			}
		case 'd': // Lower limit (down)
			l.LowerLimit, i, err = parseDecimal(data, i)
			if err != nil {
				return l, err
			}
		case 'i': // Indicator
			if i < len(data) && data[i] == '"' {
				i++
				if i < len(data) && data[i] != '"' {
					l.Indicator = LULDIndicator(data[i])
					i++
				}
				if i < len(data) && data[i] == '"' {
					i++
				}
			}
		case 't': // Timestamp
			l.Timestamp, i, err = parseTimestamp(data, i)
			if err != nil {
				return l, err
			}
		case 'z': // Tape
			if i < len(data) && data[i] == '"' {
				i++
				if i < len(data) {
					l.Tape = Tape(data[i])
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
	return l, nil
}
