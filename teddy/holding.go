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
	Volume     float64
	BuyVolume  float64
	SellVolume float64
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
		switch h.Exchange.Exchange {
		case ds.ExchangeCoinbase:
			h.fetchCoinbaseHolding()
		}
	}
	return h
}

func (h *Holding) String() string {
	return h.Symbol
}

func (h *Holding) Check() {
	if h.Quantity.Load().IsNegative() {
		loggy.Fatalf("accounting invariant violated: %s Quantity is negative: %s",
			h.Symbol, h.Quantity.Load())
	}
	if h.Available.Load().IsNegative() {
		loggy.Fatalf("accounting invariant violated: %s Available is negative: %s",
			h.Symbol, h.Available.Load())
	}
	if h.Available.Load().Cmp(h.Quantity.Load()) > 0 {
		loggy.Fatalf("accounting invariant violated: %s Available (%s) > Quantity (%s)",
			h.Symbol, h.Available.Load(), h.Quantity.Load())
	}
	if !h.IsCash {
		if h.Quantity.Load().Cmp(h.Lots.Size) != 0 {
			loggy.Fatalf("accounting invariant violated: %s Quantity (%s) != Lots.Size (%s)",
				h.Symbol, h.Quantity.Load(), h.Lots.Size)
		}
	}
}

func (h *Holding) fetchCoinbaseHolding() {
	for _, account := range getCoinbaseAccounts() {
		if account.Currency == h.Symbol {
			h.IsFiat = account.Type == "ACCOUNT_TYPE_FIAT"
			h.IsCash = h.IsFiat || looksLikeCashSymbol(h.Symbol)
			if !h.IsFiat {
				err := CoinbaseClient.SyncTransactions(h.Symbol)
				if err != nil {
					loggy.Fatalf("coinbase: error syncing transactions for asset %s: %v", h.Symbol, err)
				}
			}
			h.Lock.Lock()
			quantity := decimal.Parse(account.AvailableBalance.Value)
			hold := decimal.Parse(account.Hold.Value)
			h.Quantity.Store(quantity)
			h.Available.Store(quantity.Sub(hold))
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
			h.Lock.Unlock()
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
