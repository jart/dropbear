package sip

import (
	"dropbear/clocky"
	"dropbear/symbol"
	"fmt"
)

// Status represents a trading status message.
type Status struct {
	Type      MessageType   `json:"T"`  // MessageTypeStatus
	Tape      Tape          `json:"z"`  // tape
	Code      StatusCode    `json:"sc"` // status code
	Reason    ReasonCode    `json:"rc"` // reason code
	Timestamp clocky.Time   `json:"t"`  // RFC-3339 timestamp
	Symbol    symbol.Symbol `json:"S"`  // stock symbol
	Message   StatusMessage `json:"sm"` // status message (truncated to 16 bytes)
	ReasonMsg ReasonMessage `json:"rm"` // reason message (truncated to 16 bytes)
}

// Halt returns true if this status message indicates a trading halt.
func (s *Status) Halt() bool {
	switch s.Tape {
	case TapeA, TapeB:
		switch s.Code {
		case StatusCodeTradingHaltCTA:
			return true
		default:
			return false
		}
	case TapeC, TapeO:
		switch s.Code {
		case StatusCodeTradingHaltUTP, StatusCodeVolatilityPause:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

// Resume returns true if this status message indicates a trading resume.
func (s *Status) Resume() bool {
	switch s.Tape {
	case TapeA, TapeB:
		switch s.Code {
		case StatusCodeResumeCTA:
			return true
		default:
			return false
		}
	case TapeC, TapeO:
		switch s.Code {
		case StatusCodeTradingResumptionUTP:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func (s *Status) String() string {
	return fmt.Sprintf("Status{Tape:%q Code:%q Reason:%q Timestamp:%s Symbol:%s Message:%q ReasonMsg:%q}",
		s.Tape, s.Code, s.Reason, s.Timestamp, s.Symbol, s.Message, s.ReasonMsg)
}

// Parse parses a SIP status message.
// This is a strict parser that assumes Alpaca sent minified JSON with all and only the expected fields.
// Returns index past closing '}' on success.
func (s *Status) Parse(data []byte) (int, error) {
	const (
		gotTape = 1 << iota
		gotTimestamp
		gotSymbol
		gotCode
		gotReason
		gotMessage
		gotReasonMsg
		gotRequired = gotTimestamp + gotSymbol + gotCode + gotReason + gotMessage + gotReasonMsg
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
			if g&gotRequired != gotRequired {
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
			case 'T': // "T":"s"
				if i+5 > n {
					return 0, ErrParsingError
				}
				if data[i] != '"' || data[i+1] != ':' || data[i+2] != '"' || data[i+4] != '"' {
					return 0, ErrParsingError
				}
				if data[i+3] != 's' {
					return 0, ErrTypeMismatch
				}
				s.Type = MessageTypeStatus
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
				s.Symbol = sym
				g |= gotSymbol
				i = j + 1
			case 't': // "t":"2026-01-06T20:12:59.99839309Z"
				if i+2 > n || data[i] != '"' || data[i+1] != ':' {
					return 0, ErrParsingError
				}
				i += 2
				s.Timestamp, i, err = parseTimestamp(data, i)
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
				s.Tape = Tape(data[i+3])
				g |= gotTape
				i += 5
			case 's': // "sc" or "sm"
				if i+4 > n || data[i+1] != '"' || data[i+2] != ':' {
					return 0, ErrParsingError
				}
				switch data[i] {
				case 'c': // "sc":"H"
					if i+6 > n || data[i+3] != '"' || data[i+5] != '"' {
						return 0, ErrParsingError
					}
					s.Code = StatusCode(data[i+4])
					g |= gotCode
					i += 6
				case 'm': // "sm":"Trading Halt"
					i += 3
					if i >= n || data[i] != '"' {
						return 0, ErrParsingError
					}
					i++
					j := i
					for j < n && data[j] != '"' {
						if data[j] == '\\' {
							j++
						}
						j++
					}
					if j >= n {
						return 0, ErrParsingError
					}
					s.Message.SetBytes(data[i:j])
					g |= gotMessage
					i = j + 1
				default:
					return 0, ErrUnrecognizedField
				}
			case 'r': // "rc" or "rm"
				if i+4 > n || data[i+1] != '"' || data[i+2] != ':' {
					return 0, ErrParsingError
				}
				switch data[i] {
				case 'c': // "rc":"T1"
					i += 3
					if i >= n || data[i] != '"' {
						return 0, ErrParsingError
					}
					i++
					j := i
					for j < n && data[j] != '"' {
						j++
					}
					if j >= n {
						return 0, ErrParsingError
					}
					s.Reason, err = ParseReasonCode(data[i:j])
					if err != nil {
						return 0, err
					}
					g |= gotReason
					i = j + 1
				case 'm': // "rm":"Halt News Pending"
					i += 3
					if i >= n || data[i] != '"' {
						return 0, ErrParsingError
					}
					i++
					j := i
					for j < n && data[j] != '"' {
						if data[j] == '\\' {
							j++
						}
						j++
					}
					if j >= n {
						return 0, ErrParsingError
					}
					s.ReasonMsg.SetBytes(data[i:j])
					g |= gotReasonMsg
					i = j + 1
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
