package databento

import (
	"dropbear/clocky"
	"strings"
	"unsafe"
)

// CBBO is the record type for CBBO-1s and CBBO-1m schemas (subsampled
// consolidated best bid/offer). 80 bytes.
type CBBO struct {
	Header RecordHeader              // 16 bytes
	Price  int64                     // last event price in units of 1e-9
	Size   uint32                    // last event size
	_pad1  byte                      //
	Side   Side                      // side of the last event
	Flags  FlagSet                   // event flags
	_pad2  byte                      //
	TSRecv clocky.Time               // capture-server-received timestamp (nanoseconds since UNIX epoch)
	_pad3  [8]byte                   //
	Levels [1]ConsolidatedBidAskPair // top of book (NBBO)
}

func (c *CBBO) InstrumentID() uint32 {
	return c.Header.InstrumentID
}

func (c *CBBO) GoString() string {
	var b strings.Builder
	b.WriteString("CBBO{\n")
	appendName(&b, "Header")
	appendField(&b, &c.Header)
	appendName(&b, "Price")
	appendPrice(&b, c.Price)
	appendName(&b, "Size")
	appendField(&b, c.Size)
	appendName(&b, "Side")
	appendField(&b, c.Side)
	appendName(&b, "Flags")
	b.WriteString(c.Flags.GoString())
	b.WriteString(",\n")
	appendName(&b, "TSRecv")
	appendTimestamp(&b, c.TSRecv)
	b.WriteString("Levels: [1]ConsolidatedBidAskPair{")
	b.WriteString(c.Levels[0].GoString())
	b.WriteString("},\n")
	b.WriteString("}")
	return b.String()
}

func (c *CBBO) Encode() []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(c)), unsafe.Sizeof(*c))
}
