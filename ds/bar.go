package ds

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

func (b *Bar) String() string {
	return b.GoString()
}

func (b *Bar) GoString() string {
	return "Bar{" +
		"Timestamp:" + b.Timestamp.String() +
		", Open:" + b.Open.String() +
		", High:" + b.High.String() +
		", Low:" + b.Low.String() +
		", Close:" + b.Close.String() +
		", Volume:" + b.Volume.String() +
		", TradeCount:" + decimal.FromInt64(b.TradeCount).String() +
		", VWAP:" + b.VWAP.String() +
		"}"
}
