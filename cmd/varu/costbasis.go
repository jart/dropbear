package main

import "dropbear/decimal"

// gRealizedPnL is the cumulative realized profit/loss from closed positions.
var gRealizedPnL decimal.Decimal

// gCostPerContract tracks the average cost per contract for each open position.
// For longs this is the price we paid. For shorts this is the price we received.
var gCostPerContract = map[string]decimal.Decimal{}

// recordFill updates cost basis and realizes P&L when positions reduce or close.
// qty is signed (positive = buy, negative = sell).
// price is the per-contract fill price (always positive).
func recordFill(sym string, qty decimal.Decimal, price decimal.Decimal) {
	existing, _ := gPositions.Get(sym)
	prev := existing.Sub(qty) // position before this fill

	if prev.IsZero() {
		// opening from flat: just record cost
		gCostPerContract[sym] = price
		return
	}

	// check if we're adding to the position (same direction)
	if (prev.IsPositive() && qty.IsPositive()) || (prev.IsNegative() && qty.IsNegative()) {
		// averaging in: weighted average cost
		oldCost := gCostPerContract[sym]
		totalQty := prev.Abs().Add(qty.Abs())
		gCostPerContract[sym] = oldCost.Mul(prev.Abs()).Add(price.Mul(qty.Abs())).Div(totalQty)
		return
	}

	// reducing or closing: realize P&L on the closed quantity
	cost := gCostPerContract[sym]
	closed := qty.Abs().Min(prev.Abs())
	if prev.IsPositive() {
		// was long, selling: profit = (sell price - cost) * qty * multiplier
		gRealizedPnL = gRealizedPnL.Add(price.Sub(cost).Mul(closed).MulInt(kMultiplier))
	} else {
		// was short, buying: profit = (cost - buy price) * qty * multiplier
		gRealizedPnL = gRealizedPnL.Add(cost.Sub(price).Mul(closed).MulInt(kMultiplier))
	}

	if existing.IsZero() {
		delete(gCostPerContract, sym)
	} else if (prev.IsPositive() && existing.IsNegative()) || (prev.IsNegative() && existing.IsPositive()) {
		// flipped sides: the excess is a new position at this price
		gCostPerContract[sym] = price
	}
	// otherwise still same side, cost unchanged
}
