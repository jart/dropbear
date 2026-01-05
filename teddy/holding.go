package teddy

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"sync"
)

type Holding struct {
	Lock       sync.RWMutex
	Broker     *Broker
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

func newHolding(broker *Broker, symbol string) *Holding {
	h := &Holding{
		Broker: broker,
		Symbol: symbol,
		IsFiat: looksLikeFiatSymbol(symbol),
		IsCash: looksLikeCashSymbol(symbol),
		Lots:   ds.NewLots(GetCostBasisMethod()),
	}
	if Live {
		switch h.Broker.Broker {
		case ds.BrokerCoinbase:
			h.fetchCoinbaseHolding()
			go h.fetchCoinbaseHoldingDaemon()
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

func (h *Holding) fetchCoinbaseHoldingDaemon() {
	for {
		Hibernate()
		h.fetchCoinbaseHolding()
	}
}

func (h *Holding) fetchCoinbaseHolding() {
	h.Lock.Lock()
	defer h.Lock.Unlock()
	accounts, err := CoinbaseClient.GetAccounts()
	if err != nil {
		loggy.Fatalf("getting coinbase accounts: %v", err)
	}
	for _, account := range accounts {
		if account.Currency == h.Symbol {
			h.IsFiat = account.Type == "ACCOUNT_TYPE_FIAT"
			h.IsCash = h.IsFiat || looksLikeCashSymbol(h.Symbol)
			quantity := account.AvailableBalance.Value
			hold := account.Hold.Value
			h.Quantity.Store(quantity)
			h.Available.Store(quantity.Sub(hold))
			if !h.IsFiat {
				err := CoinbaseClient.SyncTransactions(h.Symbol)
				if err != nil {
					loggy.Fatalf("[coinbase] error syncing transactions for asset %s: %v", h.Symbol, err)
				}
				h.Lots, err = CoinbaseClient.GetLots(h.Symbol, GetCostBasisMethod())
				if err != nil {
					loggy.Fatalf("[coinbase] error fetching lots for asset %s: %v", h.Symbol, err)
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
