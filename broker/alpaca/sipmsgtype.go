package alpaca

import "fmt"

// SIPMessageType represents the type of SIP message.
type SIPMessageType byte

const (
	SIPMessageTypeUnknown   SIPMessageType = 0
	SIPMessageTypeTrade     SIPMessageType = 't' // trade execution
	SIPMessageTypeQuote     SIPMessageType = 'q' // quote update
	SIPMessageTypeBar       SIPMessageType = 'b' // minute bar
	SIPMessageTypeDailyBar  SIPMessageType = 'd' // daily bar
	SIPMessageTypeUpdatedBar SIPMessageType = 'u' // updated bar (correction)
	SIPMessageTypeStatus    SIPMessageType = 's' // trading status
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
		return "SIPMessageTypeTrade"
	case SIPMessageTypeQuote:
		return "SIPMessageTypeQuote"
	case SIPMessageTypeBar:
		return "SIPMessageTypeBar"
	case SIPMessageTypeDailyBar:
		return "SIPMessageTypeDailyBar"
	case SIPMessageTypeUpdatedBar:
		return "SIPMessageTypeUpdatedBar"
	case SIPMessageTypeStatus:
		return "SIPMessageTypeStatus"
	default:
		return fmt.Sprintf("SIPMessageType(%c)", byte(t))
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
