package databento

import (
	"dropbear/clocky"
	"strings"
	"unsafe"
)

type MBP10 struct {
	Header    RecordHeader
	Price     int64       // order price expressed as a signed integer in units of 1e-9
	Size      uint32      // order quantity
	Action    Action      // Add, Cancel, Modify, cleaR book, or Trade
	Side      Side        // side that initiates event; can be Ask for the sell aggressor in a trade, Bid for the buy aggressor in a trade, or None where no side is specified by the original trade or the record was not a trade
	Flags     FlagSet     // bit field indicating event end, message characteristics, and data quality
	Depth     uint8       // book level where update event occurred
	TSRecv    clocky.Time // capture-server-received timestamp expressed as the number of nanoseconds since the UNIX epoch
	TSInDelta int32       // matching-engine-sending timestamp expressed as the number of nanoseconds before ts_recv
	Sequence  uint32      // message sequence number assigned at the venue
	Levels    [10]BidAskPair
}

func (m *MBP10) InstrumentID() uint32 {
	return m.Header.InstrumentID
}

func (m *MBP10) GetTSRecv() clocky.Time {
	return m.TSRecv
}

func (m *MBP10) GoString() string {
	var b strings.Builder
	b.WriteString("MBP10{\n")
	appendName(&b, "Header")
	appendField(&b, &m.Header)
	appendName(&b, "Price")
	appendPrice(&b, m.Price)
	appendName(&b, "Size")
	appendField(&b, m.Size)
	appendName(&b, "Action")
	appendField(&b, m.Action)
	appendName(&b, "Side")
	appendField(&b, m.Side)
	appendName(&b, "Flags")
	b.WriteString(m.Flags.GoString())
	b.WriteString(",\n")
	appendName(&b, "Depth")
	appendField(&b, m.Depth)
	appendName(&b, "TSRecv")
	appendTimestamp(&b, m.TSRecv)
	appendName(&b, "TSInDelta")
	appendInt32(&b, m.TSInDelta)
	appendName(&b, "Sequence")
	appendField(&b, m.Sequence)
	b.WriteString("Levels: [10]BidAskPair{")
	for i := 0; i < 10; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(m.Levels[i].GoString())
	}
	b.WriteString("},\n")
	b.WriteString("}")
	return b.String()
}

func (m *MBP10) Encode() []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(m)), unsafe.Sizeof(*m))
}
