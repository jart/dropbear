package cubby

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"sync"
)

// Holding represents a stock position (not cash - cash is on Exchange).
type Holding struct {
	Lock       sync.RWMutex
	Exchange   *Exchange
	Symbol     string // e.g. AAPL, TSLA (not USD)
	Quantity   decimal.Decimal
	Available  decimal.Decimal // shares not reserved for pending sell orders
	Volume     float64
	BuyVolume  float64
	SellVolume float64
	WinCount   int
	LossCount  int
	Lots       *ds.Lots
}

func newHolding(exchange *Exchange, symbol string) *Holding {
	h := &Holding{
		Exchange: exchange,
		Symbol:   symbol,
		Lots:     ds.NewLots(GetCostBasisMethod()),
	}
	if Live {
		h.fetchAlpacaPosition()
	}
	return h
}

func (h *Holding) String() string {
	return h.Symbol
}

func (h *Holding) Check() {
	qty := h.Quantity.Load()
	avail := h.Available.Load()
	if qty.IsNegative() {
		loggy.Fatalf("accounting invariant violated: %s Quantity is negative: %s",
			h.Symbol, qty)
	}
	if avail.IsNegative() {
		loggy.Fatalf("accounting invariant violated: %s Available is negative: %s",
			h.Symbol, avail)
	}
	if avail.Cmp(qty) > 0 {
		loggy.Fatalf("accounting invariant violated: %s Available (%s) > Quantity (%s)",
			h.Symbol, avail, qty)
	}
	if h.Quantity.Load().Cmp(h.Lots.Size) != 0 {
		loggy.Fatalf("accounting invariant violated: %s Quantity (%s) != Lots.Size (%s)",
			h.Symbol, h.Quantity.Load(), h.Lots.Size)
	}
}

func (h *Holding) fetchAlpacaPosition() {
	positions, err := AlpacaClient.GetPositions()
	if err != nil {
		loggy.Fatalf("alpaca: error fetching positions: %v", err)
	}
	for _, pos := range positions {
		if pos.Symbol == h.Symbol {
			h.Lock.Lock()
			qty := decimal.Parse(pos.Qty)
			h.Quantity.Store(qty)
			h.Lock.Unlock()
			return
		}
	}
}
