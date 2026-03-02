// CBBO (Consolidated Best Bid/Offer) record for NBBO data.
// Source: ~/vendor/databento-cpp/include/databento/record.hpp lines 265-284
package databento

import (
	"dropbear/clocky"
	"strings"
)

// ConsolidatedBidAskPair is a price level consolidated from multiple venues.
// 32 bytes.
type ConsolidatedBidAskPair struct {
	BidPx int64     // best bid price in units of 1e-9
	AskPx int64     // best ask price in units of 1e-9
	BidSz uint32    // aggregate bid size at best
	AskSz uint32    // aggregate ask size at best
	BidPb Publisher // publisher ID of the venue with the best bid
	_pad1 [2]byte
	AskPb Publisher // publisher ID of the venue with the best ask
	_pad2 [2]byte
}

func (p *ConsolidatedBidAskPair) GoString() string {
	var b strings.Builder
	b.WriteString("ConsolidatedBidAskPair{\n")
	appendName(&b, "BidPx")
	appendPrice(&b, p.BidPx)
	appendName(&b, "AskPx")
	appendPrice(&b, p.AskPx)
	appendName(&b, "BidSz")
	appendField(&b, p.BidSz)
	appendName(&b, "AskSz")
	appendField(&b, p.AskSz)
	appendName(&b, "BidPb")
	appendPublisher(&b, p.BidPb)
	appendName(&b, "AskPb")
	appendPublisher(&b, p.AskPb)
	b.WriteString("}")
	return b.String()
}

// CBBO is the record type for CBBO-1s and CBBO-1m schemas (subsampled
// consolidated best bid/offer). 80 bytes.
type CBBO struct {
	Header RecordHeader // 16 bytes
	Price  int64        // last event price in units of 1e-9
	Size   uint32       // last event size
	_pad1  byte
	Side   Side    // side of the last event
	Flags  FlagSet // event flags
	_pad2  byte
	TSRecv clocky.Time // capture-server-received timestamp (nanoseconds since UNIX epoch)
	_pad3  [8]byte
	Levels [1]ConsolidatedBidAskPair // top of book (NBBO)
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
