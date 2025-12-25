package teddy

import (
	"dropbear/decimal"
	"dropbear/ds"
	"sync"
)

type Holding struct {
	Lock       sync.RWMutex
	Exchange   *Exchange
	Symbol     string // e.g. USD, BTC
	Quantity   decimal.Decimal
	Available  decimal.Decimal
	Volume     decimal.Decimal
	BuyVolume  decimal.Decimal
	SellVolume decimal.Decimal
	WinCount   int // number of profitable sells
	LossCount  int // number of unprofitable sells
	Lots       *ds.Lots
	IsCash     bool
}

func newHolding(exchange *Exchange, symbol string) *Holding {
	isCash := looksLikeCashSymbol(symbol)
	return &Holding{
		Exchange: exchange,
		Symbol:   symbol,
		IsCash:   isCash,
		Lots:     ds.NewLots(GetCostBasisMethod()),
	}
}

func looksLikeCashSymbol(symbol string) bool {
	switch symbol {
	case "USD", "USDC", "USDT", "FDUSD", "EUR", "GBP", "JPY", "AUD", "CAD", "CHF", "CNY":
		return true
	default:
		return false
	}
}
