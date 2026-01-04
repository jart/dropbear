package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
)

type Bar struct {
	Timestamp  clocky.Time     `json:"t"`
	Open       decimal.Decimal `json:"o"`
	High       decimal.Decimal `json:"h"`
	Low        decimal.Decimal `json:"l"`
	Close      decimal.Decimal `json:"c"`
	Volume     decimal.Decimal `json:"v"`
	TradeCount int64           `json:"n"`
	VWAP       decimal.Decimal `json:"vw"`
}

type BarsResponse struct {
	Bars      []Bar  `json:"bars"`
	NextToken string `json:"next_page_token"`
}
