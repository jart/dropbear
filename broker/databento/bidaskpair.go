package databento

import "strings"

type BidAskPair struct {
	BidPx int64  // bid price
	AskPx int64  // ask price
	BidSz uint32 // number of contracts available at bid
	AskSz uint32 // number of contracts available at ask
	BidCt uint32 // number of orders at bid
	AskCt uint32 // number of orders at ask
}

func (p *BidAskPair) GoString() string {
	var b strings.Builder
	b.WriteString("BidAskPair{\n")
	appendName(&b, "BidPx")
	appendPrice(&b, p.BidPx)
	appendName(&b, "AskPx")
	appendPrice(&b, p.AskPx)
	appendName(&b, "BidSz")
	appendField(&b, p.BidSz)
	appendName(&b, "AskSz")
	appendField(&b, p.AskSz)
	appendName(&b, "BidCt")
	appendField(&b, p.BidCt)
	appendName(&b, "AskCt")
	appendField(&b, p.AskCt)
	b.WriteString("}")
	return b.String()
}
