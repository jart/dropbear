package teddy

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
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
	WinCount   int
	LossCount  int
	Lots       *ds.Lots
	IsCash     bool
}

func newHolding(exchange *Exchange, symbol string) *Holding {
	return &Holding{
		Exchange: exchange,
		Symbol:   symbol,
		IsCash:   looksLikeCashSymbol(symbol),
		Lots:     ds.NewLots(GetCostBasisMethod()),
	}
}

// check verifies critical accounting invariants.
func (h *Holding) Check() {
	if h.Quantity.IsNegative() {
		loggy.Fatalf("accounting invariant violated: %s Quantity is negative: %s",
			h.Symbol, h.Quantity)
	}
	if h.Available.IsNegative() {
		loggy.Fatalf("accounting invariant violated: %s Available is negative: %s",
			h.Symbol, h.Available)
	}
	if h.Available.Cmp(h.Quantity) > 0 {
		loggy.Fatalf("accounting invariant violated: %s Available (%s) > Quantity (%s)",
			h.Symbol, h.Available, h.Quantity)
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
