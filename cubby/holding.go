package cubby

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"sync"
)

// Holding represents a stock position (not cash - cash is on Broker).
// Quantity is positive for long positions, negative for short positions.
type Holding struct {
	Lock       sync.RWMutex
	Broker     *Broker
	Symbol     string // e.g. AAPL, TSLA (not USD)
	Quantity   decimal.Decimal
	Available  decimal.Decimal // shares not reserved for pending sell/cover orders
	Volume     float64
	BuyVolume  float64
	SellVolume float64
	WinCount   int
	LossCount  int
	Lots       *ds.Lots // long position cost basis (FIFO)
	ShortLots  *ds.Lots // short position cost basis (FIFO) - tracks proceeds from shorts
}

func newHolding(broker *Broker, symbol string) *Holding {
	h := &Holding{
		Broker:    broker,
		Symbol:    symbol,
		Lots:      ds.NewLots(ds.CostBasisMethodFIFO),
		ShortLots: ds.NewLots(ds.CostBasisMethodFIFO),
	}
	if Live {
		h.fetchAlpacaPosition()
	}
	return h
}

func (h *Holding) String() string {
	return h.Symbol
}

// IsLong returns true if this is a long position (positive quantity).
func (h *Holding) IsLong() bool {
	return h.Quantity.Load().IsPositive()
}

// IsShort returns true if this is a short position (negative quantity).
func (h *Holding) IsShort() bool {
	return h.Quantity.Load().IsNegative()
}

// IsFlat returns true if there is no position (zero quantity).
func (h *Holding) IsFlat() bool {
	return h.Quantity.Load().IsZero()
}

func (h *Holding) Check() {
	qty := h.Quantity.Load()
	avail := h.Available.Load()

	// Available must always be non-negative (represents shares we can sell/cover)
	if avail.IsNegative() {
		loggy.Fatalf("accounting invariant violated: %s Available is negative: %s",
			h.Symbol, avail)
	}

	if qty.IsPositive() {
		// Long position: Available <= Quantity
		if avail.Cmp(qty) > 0 {
			loggy.Fatalf("accounting invariant violated: %s Available (%s) > Quantity (%s)",
				h.Symbol, avail, qty)
		}
		// Long position: Quantity must match Lots.Size
		if qty.Cmp(h.Lots.Size) != 0 {
			loggy.Fatalf("accounting invariant violated: %s Quantity (%s) != Lots.Size (%s)",
				h.Symbol, qty, h.Lots.Size)
		}
	} else if qty.IsNegative() {
		// Short position: Available <= |Quantity| (shares available to cover)
		absQty := qty.Neg()
		if avail.Cmp(absQty) > 0 {
			loggy.Fatalf("accounting invariant violated: %s Available (%s) > |Quantity| (%s)",
				h.Symbol, avail, absQty)
		}
		// Short position: |Quantity| must match ShortLots.Size
		if absQty.Cmp(h.ShortLots.Size) != 0 {
			loggy.Fatalf("accounting invariant violated: %s |Quantity| (%s) != ShortLots.Size (%s)",
				h.Symbol, absQty, h.ShortLots.Size)
		}
	}
	// Zero quantity: nothing to check (flat position)
}

func (h *Holding) fetchAlpacaPosition() {
	positions, err := AlpacaClient.GetPositions()
	if err != nil {
		loggy.Fatalf("alpaca: error fetching positions: %v", err)
	}
	for _, pos := range positions {
		if pos.Symbol == h.Symbol {
			qty := decimal.Parse(pos.Qty)
			h.Quantity.Store(qty)
			return
		}
	}
}
