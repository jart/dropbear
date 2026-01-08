package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/symbol"
)

// SIPBar represents an aggregated bar from the SIP feed.
type SIPBar struct {
	Type      SIPMessageType  `json:"T"`  // SIPMessageTypeBar, SIPMessageTypeDailyBar, or SIPMessageTypeUpdatedBar
	NumTrades uint32          `json:"n"`  // number of trades
	Timestamp clocky.Time     `json:"t"`  // RFC-3339 timestamp
	Symbol    symbol.Symbol   `json:"S"`  // stock symbol
	Open      decimal.Decimal `json:"o"`  // opening price
	High      decimal.Decimal `json:"h"`  // highest price
	Low       decimal.Decimal `json:"l"`  // lowest price
	Close     decimal.Decimal `json:"c"`  // closing price
	Volume    int64           `json:"v"`  // total volume
	VWAP      decimal.Decimal `json:"vw"` // volume-weighted average price
}
