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
	IsCash     bool
}

func newHolding(exchange *Exchange, symbol string) *Holding {
	h := &Holding{
		Exchange: exchange,
		Symbol:   symbol,
		IsCash:   symbol == "USD",
		Lots:     ds.NewLots(ds.CostBasisMethodFIFO),
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
	// In backtests with many stocks, small rounding errors can accumulate
	// Allow up to -$100 tolerance for cash, fatal for stocks
	// tolerance := decimal.Parse("-100")
	tolerance := decimal.Parse("0")

	qty := h.Quantity.Load()
	if qty.IsNegative() {
		// Allow negative cash when using margin, or small tolerance in backtests
		if h.IsCash && (*flagMargin > 1 || qty.Cmp(tolerance) >= 0) {
			// OK - margin or small backtest rounding error
		} else {
			loggy.Fatalf("accounting invariant violated: %s Quantity is negative: %s",
				h.Symbol, qty)
		}
	}

	avail := h.Available.Load()
	if avail.IsNegative() {
		if !h.IsCash || avail.Cmp(tolerance) < 0 {
			loggy.Fatalf("accounting invariant violated: %s Available is negative: %s",
				h.Symbol, avail)
		}
	}

	// Skip Available <= Quantity check for cash when using margin (we can spend borrowed money)
	if !h.IsCash && h.Available.Load().Cmp(h.Quantity.Load()) > 0 {
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
	if h.IsCash {
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
