package main

import (
	"dropbear/decimal"
	"dropbear/options"
	"fmt"
	"maps"
	"slices"
	"strings"
)

type Holdings struct {
	Cash         decimal.Decimal               // total cash possessed, in dollars
	Positions    map[options.Security]*Holding // never contains zero-quantity holdings
	RealizedPnL  decimal.Decimal               // total profit and loss from closed positions, in dollars
	TotalDebits  decimal.Decimal               // total cash flow out from filled trades, in dollars
	TotalCredits decimal.Decimal               // total cash flow in from filled trades, in dollars
	TotalError   decimal.Decimal               // total error from quantization, in dollars
	TotalFees    decimal.Decimal               // total commissions, fees, and regulatory costs
	Volume       decimal.Decimal               // total number of contracts traded
}

func (hs *Holdings) LiquidationValue() decimal.Decimal {
	value := hs.Cash
	for security, holding := range hs.Positions {
		value = value.Add(security.MidPrice().Mul(holding.Quantity).MulInt(security.Multiplier()))
	}
	return value
}

// Sorted returns holdings sorted by option OSI symbol.
func (h *Holdings) Sorted() []*Holding {
	holdings := slices.Collect(maps.Values(h.Positions))
	slices.SortFunc(holdings, func(a, b *Holding) int {
		return strings.Compare(a.Security.Name(), b.Security.Name())
	})
	return holdings
}

// Add updates holdings to reflect fill.
// delta is positive for buy orders and negative for sell orders.
// price is always positive and reflects dollars per underlying share.
func (h *Holdings) Add(security options.Security, delta, price decimal.Decimal) {
	if delta.IsZero() {
		panic("zero quantity change")
	}
	if !price.IsPositive() {
		panic("non-positive price")
	}
	defer h.check()

	// track volume (fees is responsibility of caller)
	h.Volume = h.Volume.Add(delta.Abs())

	// update cash and ledger
	dollars := price.Mul(delta.Abs()).MulInt(security.Multiplier())
	if delta.IsPositive() {
		h.Cash = h.Cash.Sub(dollars)
		h.TotalDebits = h.TotalDebits.Add(dollars)
	} else {
		h.Cash = h.Cash.Add(dollars)
		h.TotalCredits = h.TotalCredits.Add(dollars)
	}

	// handle case of opening a new position
	holding := h.Positions[security]
	if holding == nil {
		h.Positions[security] = &Holding{
			Security:    security,
			Quantity:    delta,
			AverageCost: price,
		}
		return
	}
	oldQuantity := holding.Quantity
	newQuantity := oldQuantity.Add(delta)

	// handle simple case of growing a position
	if oldQuantity.IsPositive() == delta.IsPositive() {
		oldTotalCost := holding.AverageCost.Mul(oldQuantity.Abs())
		newTotalCost := oldTotalCost.Add(price.Mul(delta.Abs()))
		holding.AverageCost = newTotalCost.Div(newQuantity.Abs())
		holding.Quantity = newQuantity
		return
	}

	// compute realized profit and loss on closed portion of position
	var profitPerShare decimal.Decimal
	if oldQuantity.IsPositive() {
		profitPerShare = price.Sub(holding.AverageCost)
	} else {
		profitPerShare = holding.AverageCost.Sub(price)
	}
	closedQuantity := delta.Abs().Min(oldQuantity.Abs())
	profitPerContract := profitPerShare.MulInt(security.Multiplier())
	profitForFill := profitPerContract.Mul(closedQuantity)
	h.RealizedPnL = h.RealizedPnL.Add(profitForFill)

	// update holding to reflect remaining open position, if any
	if newQuantity.IsZero() {
		delete(h.Positions, security)
	} else {
		holding.Quantity = newQuantity
		if oldQuantity.IsPositive() != newQuantity.IsPositive() {
			holding.AverageCost = price // delta overlapped zero
		}
	}
}

func (h *Holdings) GetQuantity(security options.Security) decimal.Decimal {
	holding := h.Positions[security]
	if holding == nil {
		return decimal.Zero
	}
	return holding.Quantity
}

func (h *Holdings) check() {
	for security, holding := range h.Positions {
		holding.check()
		if holding.Security != security {
			panic("holding security mismatch")
		}
	}

	// Accounting identity: cash should equal the sum of all individual
	// cash flows. gCash is updated per-trade, gTotalCashIn/Out track
	// the same flows independently. If they diverge, something double-
	// counted or skipped a leg.
	expectedCash := h.TotalCredits.Sub(h.TotalDebits)
	if h.Cash.Cmp(expectedCash) != 0 {
		panic(fmt.Sprintf("sanity: cash=%s but expected=%s (in=%s out=%s)",
			h.Cash, expectedCash, h.TotalDebits, h.TotalCredits))
	}

	// Accounting identity:
	//
	//     cash = realized p&l + unrealized cost basis
	//
	// Small drift (usually fractions of a penny) is expected and
	// benign. It comes from division in the weighted average cost
	// basis calculation that makes the cost basis slightly disagree
	// with the actual cash that moved. The good news is that drift
	// should be self-correcting. First, our decimal library uses
	// banker's rounding, which is unbiased. Secondly, while drift
	// accumulates when positions are open, it's corrected when they
	// close because realized P&L is computed from the same rounded
	// cost. Therefore the rounding errors cancel out on exit.
	var unrealizedCost decimal.Decimal
	for security, holding := range h.Positions {
		unrealizedCost = unrealizedCost.Sub(holding.AverageCost.Mul(holding.Quantity).MulInt(security.Multiplier()))
	}
	expectedFromBooks := h.RealizedPnL.Add(unrealizedCost)
	h.TotalError = h.Cash.Sub(expectedFromBooks)
	if h.TotalError.Abs().Cmp(*maxErrorFlag) > 0 {
		panic(fmt.Sprintf("sanity: error=%s exceeds max %s (cash=%s realized=%s unrealized_cost=%s)",
			h.TotalError, *maxErrorFlag, h.Cash, h.RealizedPnL, unrealizedCost))
	}
}
