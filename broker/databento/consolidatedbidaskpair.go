package databento

import "strings"

// ConsolidatedBidAskPair is a price level consolidated from multiple venues.
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
