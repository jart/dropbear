// CMBP1 (Consolidated Market-By-Price, 1 level) record for tick-level NBBO.
// Also used for TCBBO (trade with consolidated BBO).
// Source: ~/vendor/databento-cpp/include/databento/record.hpp lines 241-261
package databento

import (
	"dropbear/clocky"
	"strings"
	"unsafe"
)

// CMBP1 is the record type for CMBP-1 and TCBBO schemas. Unlike CBBO (which
// is subsampled), CMBP1 fires on every NBBO change and includes the action
// that caused the change. 80 bytes.
type CMBP1 struct {
	Header    RecordHeader // 16 bytes
	Price     int64        // last event price in units of 1e-9
	Size      uint32       // last event size
	Action    Action       // event that caused this record
	Side      Side         // side of the last event
	Flags     FlagSet      // event flags
	_pad1     byte
	TSRecv    clocky.Time // capture-server-received timestamp
	TSInDelta int32       // delta from TSEvent to send time (nanoseconds)
	_pad2     [4]byte
	Levels    [1]ConsolidatedBidAskPair // top of book (NBBO)
}

func (c *CMBP1) InstrumentID() uint32 {
	return c.Header.InstrumentID
}

func (c *CMBP1) GetTSRecv() clocky.Time {
	return c.TSRecv
}

func (c *CMBP1) GoString() string {
	var b strings.Builder
	b.WriteString("CMBP1{\n")
	appendName(&b, "Header")
	appendField(&b, &c.Header)
	appendName(&b, "Price")
	appendPrice(&b, c.Price)
	appendName(&b, "Size")
	appendField(&b, c.Size)
	appendName(&b, "Action")
	appendField(&b, c.Action)
	appendName(&b, "Side")
	appendField(&b, c.Side)
	appendName(&b, "Flags")
	b.WriteString(c.Flags.GoString())
	b.WriteString(",\n")
	appendName(&b, "TSRecv")
	appendTimestamp(&b, c.TSRecv)
	appendName(&b, "TSInDelta")
	appendField(&b, c.TSInDelta)
	b.WriteString("Levels: [1]ConsolidatedBidAskPair{")
	b.WriteString(c.Levels[0].GoString())
	b.WriteString("},\n")
	b.WriteString("}")
	return b.String()
}

func (c *CMBP1) Encode() []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(c)), unsafe.Sizeof(*c))
}
