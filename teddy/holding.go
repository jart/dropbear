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
	IsFiat     bool
	IsCash     bool
}

func newHolding(exchange *Exchange, symbol string) *Holding {
	h := &Holding{
		Exchange: exchange,
		Symbol:   symbol,
		IsFiat:   looksLikeFiatSymbol(symbol),
		IsCash:   looksLikeCashSymbol(symbol),
		Lots:     ds.NewLots(GetCostBasisMethod()),
	}
	if Live {
		switch exchange.Exchange {
		case ds.ExchangeCoinbase:
			h.fetchCoinbaseHolding()
		}
	}
	return h
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
	if !h.IsCash {
		if h.Quantity.Cmp(h.Lots.Size) != 0 {
			loggy.Fatalf("accounting invariant violated: %s Quantity (%s) != Lots.Size (%s)",
				h.Symbol, h.Quantity, h.Lots.Size)
		}
	}
}

func (h *Holding) fetchCoinbaseHolding() {
	for _, account := range getCoinbaseAccounts() {
		if account.Currency == h.Symbol {
			h.IsFiat = account.Type == "ACCOUNT_TYPE_FIAT"
			h.IsCash = h.IsFiat || looksLikeCashSymbol(h.Symbol)
			h.Quantity = decimal.Parse(account.AvailableBalance.Value)
			h.Available = h.Quantity.Sub(decimal.Parse(account.Hold.Value))
			if !h.IsFiat {
				err := CoinbaseClient.SyncTransactions(h.Symbol)
				if err != nil {
					loggy.Fatalf("coinbase: error syncing transactions for asset %s: %v", h.Symbol, err)
				}
				h.Lots, err = CoinbaseClient.GetLots(h.Symbol, GetCostBasisMethod())
				if err != nil {
					loggy.Fatalf("coinbase: error fetching lots for asset %s: %v", h.Symbol, err)
				}
			}
			h.Check()
			return
		}
	}
}

func looksLikeFiatSymbol(symbol string) bool {
	switch symbol {
	case "USD", "EUR", "GBP", "JPY", "AUD", "CAD", "CHF", "CNY":
		return true
	default:
		return false
	}
}

func looksLikeCashSymbol(symbol string) bool {
	if looksLikeFiatSymbol(symbol) {
		return true
	}
	switch symbol {
	case "USDC", "USDT", "FDUSD":
		return true
	default:
		return false
	}
}
