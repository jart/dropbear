package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/symbol"
)

// SIPTrade represents a trade execution from the SIP feed.
type SIPTrade struct {
	Type       SIPMessageType  `json:"T"` // SIPMessageTypeTrade
	Tape       SIPTape         `json:"z"` // tape designation (A, B, C)
	Exchange   SIPExchange     `json:"x"` // exchange code
	Timestamp  clocky.Time     `json:"t"` // RFC-3339 timestamp
	Symbol     symbol.Symbol   `json:"S"` // stock symbol
	TradeID    int64           `json:"i"` // unique trade identifier
	Price      decimal.Decimal `json:"p"` // execution price
	Size       int64           `json:"s"` // quantity traded
	Conditions SIPTradeCond    `json:"c"` // trade conditions (bitmask)
}

// IsRegularSale returns true if this is a normal trade (no special conditions).
func (t *SIPTrade) IsRegularSale() bool {
	return t.Conditions.IsRegularSale() || t.Conditions == 0
}

// IsOddLot returns true if this is an odd lot trade.
func (t *SIPTrade) IsOddLot() bool {
	return t.Conditions.IsOddLot()
}

// IsExtendedHours returns true if this is an extended hours trade.
func (t *SIPTrade) IsExtendedHours() bool {
	return t.Conditions.IsExtendedHours()
}
