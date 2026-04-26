package sip

import (
	"dropbear/clocky"
	"dropbear/symbol"
	"log"
	"unsafe"
)

// Message is a union type for Trade, Quote, Status, LULD, and Imbalance.
// All message types share common header fields at fixed offsets:
//   - Type at offset 0
//   - Tape at offset 1
//   - Timestamp at offset 8
//   - Symbol at offset 16
//
// Use the accessor methods Trade(), Quote(), Status(), LULD(), Imbalance()
// to interpret the message as a specific type after checking Type.
type Message struct {
	Type      MessageType   // message type (t, q, s, l, i)
	Tape      Tape          // tape designation (A, B, C)
	_         [6]byte       // padding to align Timestamp
	Timestamp clocky.Time   // RFC-3339 timestamp
	Symbol    symbol.Symbol // stock symbol
	_         [32]byte      // padding for type-specific fields
}

// Trade returns a pointer to this message interpreted as a Trade.
// The caller must ensure Type == MessageTypeTrade before calling.
func (m *Message) Trade() *Trade {
	return (*Trade)(unsafe.Pointer(m))
}

// Quote returns a pointer to this message interpreted as a Quote.
// The caller must ensure Type == MessageTypeQuote before calling.
func (m *Message) Quote() *Quote {
	return (*Quote)(unsafe.Pointer(m))
}

// Status returns a pointer to this message interpreted as a Status.
// The caller must ensure Type == MessageTypeStatus before calling.
func (m *Message) Status() *Status {
	return (*Status)(unsafe.Pointer(m))
}

// LULD returns a pointer to this message interpreted as a LULD.
// The caller must ensure Type == MessageTypeLULD before calling.
func (m *Message) LULD() *LULD {
	return (*LULD)(unsafe.Pointer(m))
}

// Imbalance returns a pointer to this message interpreted as an Imbalance.
// The caller must ensure Type == MessageTypeImbalance before calling.
func (m *Message) Imbalance() *Imbalance {
	return (*Imbalance)(unsafe.Pointer(m))
}

// Parse decodes an Alpaca SIP message from JSON in a strict way.
// Returns index past closing '}' on success.
func (m *Message) Parse(data []byte) (int, error) {
	// Alpaca always sends them like: {"T":"q",...}
	// The sub-parsers will verify the rest of the structure.
	if len(data) < 7 {
		log.Printf("boop %s", string(data))
		return 0, ErrParsingError
	}
	switch data[6] {
	case 't':
		return m.Trade().Parse(data)
	case 'q':
		return m.Quote().Parse(data)
	case 's':
		return m.Status().Parse(data)
	case 'l':
		return m.LULD().Parse(data)
	case 'i':
		return m.Imbalance().Parse(data)
	default:
		return 0, ErrInvalidMessageType
	}
}
