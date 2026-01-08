package alpaca

import "fmt"

// SIPMessageType represents the type of SIP message.
type SIPMessageType byte

const (
	SIPMessageTypeUnknown    SIPMessageType = 0
	SIPMessageTypeTrade      SIPMessageType = 't' // trade execution
	SIPMessageTypeQuote      SIPMessageType = 'q' // quote update
	SIPMessageTypeBar        SIPMessageType = 'b' // minute bar
	SIPMessageTypeDailyBar   SIPMessageType = 'd' // daily bar
	SIPMessageTypeUpdatedBar SIPMessageType = 'u' // updated bar (correction)
	SIPMessageTypeStatus     SIPMessageType = 's' // trading status
	SIPMessageTypeLULD       SIPMessageType = 'l' // limit up-limit down
)

// Pre-computed JSON representations (no allocation on marshal)
var sipMessageTypeJSON [256][]byte

func init() {
	for i := range sipMessageTypeJSON {
		sipMessageTypeJSON[i] = []byte{'"', byte(i), '"'}
	}
}

func (t SIPMessageType) GoString() string {
	switch t {
	case SIPMessageTypeTrade:
		return "alpaca.SIPMessageTypeTrade"
	case SIPMessageTypeQuote:
		return "alpaca.SIPMessageTypeQuote"
	case SIPMessageTypeBar:
		return "alpaca.SIPMessageTypeBar"
	case SIPMessageTypeDailyBar:
		return "alpaca.SIPMessageTypeDailyBar"
	case SIPMessageTypeUpdatedBar:
		return "alpaca.SIPMessageTypeUpdatedBar"
	case SIPMessageTypeStatus:
		return "alpaca.SIPMessageTypeStatus"
	case SIPMessageTypeLULD:
		return "alpaca.SIPMessageTypeLULD"
	default:
		return fmt.Sprintf("alpaca.SIPMessageType(%c)", byte(t))
	}
}

func (t SIPMessageType) String() string {
	return t.GoString()
}

func (t SIPMessageType) MarshalJSON() ([]byte, error) {
	return sipMessageTypeJSON[t], nil
}

func (t *SIPMessageType) UnmarshalJSON(data []byte) error {
	if len(data) == 3 && data[0] == '"' && data[2] == '"' {
		*t = SIPMessageType(data[1])
		return nil
	}
	if len(data) == 2 && data[0] == '"' && data[1] == '"' {
		*t = SIPMessageTypeUnknown
		return nil
	}
	return fmt.Errorf("invalid SIPMessageType: %s", data)
}
