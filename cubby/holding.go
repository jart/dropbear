package cubby

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"sync"
)

type Holding struct {
	Lock       sync.RWMutex
	Exchange   *Exchange
	Symbol     string // e.g. USD, AAPL
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
		IsFiat:   symbol == "USD",
		IsCash:   symbol == "USD",
		Lots:     ds.NewLots(GetCostBasisMethod()),
	}
	if Live {
		h.fetchAlpacaHolding()
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

func (h *Holding) fetchAlpacaHolding() {
	if h.IsFiat {
		account, err := AlpacaClient.GetAccount()
		if err != nil {
			loggy.Fatalf("alpaca: error fetching account: %v", err)
		}
		h.Lock.Lock()
		h.Quantity.Store(decimal.Parse(account.Cash))
		h.Available.Store(decimal.Parse(account.BuyingPower))
		h.Lock.Unlock()
		return
	}
	positions, err := AlpacaClient.GetPositions()
	if err != nil {
		loggy.Fatalf("alpaca: error fetching positions: %v", err)
	}
	for _, pos := range positions {
		if pos.Symbol == h.Symbol {
			h.Lock.Lock()
			qty := decimal.Parse(pos.Qty)
			h.Quantity.Store(qty)
			h.Available.Store(qty)
			h.Lock.Unlock()
			return
		}
	}
}
