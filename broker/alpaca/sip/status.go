package sip

import (
	"dropbear/clocky"
	"dropbear/ds/symbol"
)

// Status represents a trading status message.
type Status struct {
	Type      MessageType   `json:"T"`  // MessageTypeStatus
	Code      StatusCode    `json:"sc"` // status code
	Reason    ReasonCode    `json:"rc"` // reason code
	Timestamp clocky.Time   `json:"t"`  // RFC-3339 timestamp
	Symbol    symbol.Symbol `json:"S"`  // stock symbol
	Message   string        `json:"sm"` // status message
}

// ParseStatus parses a SIP status message with minimal allocations.
func ParseStatus(data []byte) (Status, error) {
	var s Status
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
						s.Type = MessageType(data[i])
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
						return s, err
					}
					s.Symbol = sym
					i++
				}
			case 't': // Timestamp
				s.Timestamp, i, err = parseTimestamp(data, i)
				if err != nil {
					return s, err
				}
			default:
				i = skipValue(data, i)
			}
		case 2:
			switch key2(data[keyStart], data[keyStart+1]) {
			case key2('s', 'c'): // Status Code
				if i < len(data) && data[i] == '"' {
					i++
					if i < len(data) {
						s.Code = StatusCode(data[i])
						i++
					}
					if i < len(data) && data[i] == '"' {
						i++
					}
				}
			case key2('s', 'm'): // Status Message
				if i < len(data) && data[i] == '"' {
					i++
					start := i
					for i < len(data) && data[i] != '"' {
						if data[i] == '\\' {
							i++
						}
						i++
					}
					s.Message = string(data[start:i])
					i++
				}
			case key2('r', 'c'): // Reason Code
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
					i++
				}
			default:
				i = skipValue(data, i)
			}
		default:
			i = skipValue(data, i)
		}
	}
	return s, nil
}
