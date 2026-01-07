package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
)

// SIPQuote represents a quote update from the SIP feed.
type SIPQuote struct {
	Symbol      string          `json:"S"`  // stock symbol
	AskPrice    decimal.Decimal `json:"ap"` // ask price
	BidPrice    decimal.Decimal `json:"bp"` // bid price
	AskSize     int64           `json:"as"` // ask size
	BidSize     int64           `json:"bs"` // bid size
	AskExchange SIPExchange     `json:"ax"` // ask exchange code
	BidExchange SIPExchange     `json:"bx"` // bid exchange code
	Type        SIPMessageType  `json:"T"`  // SIPMessageTypeQuote
	Tape        SIPTape         `json:"z"`  // tape designation (A, B, C)
	Conditions  SIPQuoteCond    `json:"c"`  // quote conditions (bitmask)
	Timestamp   clocky.Time     `json:"t"`  // RFC-3339 timestamp
}

// Midpoint returns the midpoint between bid and ask prices.
func (q *SIPQuote) Midpoint() decimal.Decimal {
	return q.BidPrice.Add(q.AskPrice).DivInt(2)
}

// Spread returns the bid-ask spread.
func (q *SIPQuote) Spread() decimal.Decimal {
	return q.AskPrice.Sub(q.BidPrice)
}

// IsNonFirm returns true if any condition indicates a non-firm quote.
func (q *SIPQuote) IsNonFirm() bool {
	return q.Conditions.IsNonFirm()
}

// IsSlow returns true if any condition indicates a slow quote.
func (q *SIPQuote) IsSlow() bool {
	return q.Conditions.IsSlow()
}
