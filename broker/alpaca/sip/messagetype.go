package sip

import "fmt"

// MessageType represents the type of SIP message.
type MessageType byte

const (
	MessageTypeTrade      MessageType = 't' // trade execution
	MessageTypeQuote      MessageType = 'q' // quote update
	MessageTypeStatus     MessageType = 's' // trading status
	MessageTypeLULD       MessageType = 'l' // limit up-limit down
	MessageTypeBar        MessageType = 'b' // minute bar (not supported)
	MessageTypeDailyBar   MessageType = 'd' // daily bar (not supported)
	MessageTypeUpdatedBar MessageType = 'u' // updated bar (correction; not supported)
	MessageTypeImbalance  MessageType = 'i' // order imbalance (during LULD/halts)
)

// Pre-computed JSON representations (no allocation on marshal)
var messageTypeJSON [256][]byte

func init() {
	for i := range messageTypeJSON {
		messageTypeJSON[i] = []byte{'"', byte(i), '"'}
	}
}

func (t MessageType) GoString() string {
	switch t {
	case MessageTypeTrade:
		return "sip.MessageTypeTrade"
	case MessageTypeQuote:
		return "sip.MessageTypeQuote"
	case MessageTypeBar:
		return "sip.MessageTypeBar"
	case MessageTypeDailyBar:
		return "sip.MessageTypeDailyBar"
	case MessageTypeUpdatedBar:
		return "sip.MessageTypeUpdatedBar"
	case MessageTypeStatus:
		return "sip.MessageTypeStatus"
	case MessageTypeLULD:
		return "sip.MessageTypeLULD"
	case MessageTypeImbalance:
		return "sip.MessageTypeImbalance"
	default:
		return fmt.Sprintf("sip.MessageType(%c)", byte(t))
	}
}

func (t MessageType) String() string {
	return t.GoString()
}

func (t MessageType) MarshalJSON() ([]byte, error) {
	return messageTypeJSON[t], nil
}

func (t *MessageType) UnmarshalJSON(data []byte) error {
	if len(data) == 3 && data[0] == '"' && data[2] == '"' {
		*t = MessageType(data[1])
		return nil
	}
	return ErrInvalidMessageType
}
