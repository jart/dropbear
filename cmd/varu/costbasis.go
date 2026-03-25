package main

import "dropbear/decimal"

// CostLot represents a FIFO cost basis lot.
type CostLot struct {
	Qty   decimal.Decimal // always positive
	Price decimal.Decimal // per-contract price paid (positive = cost, negative = credit)
}

// gCostBasis tracks FIFO lots per OSI symbol.
var gCostBasis = map[string][]CostLot{}

// gRealizedPnL is the cumulative realized profit/loss from closed positions.
var gRealizedPnL decimal.Decimal

// recordTrade updates the FIFO cost basis and realized P&L.
// qty is signed: positive = opening long / adding, negative = closing long / reducing.
// price is the per-contract price (always positive).
// For buys, qty > 0 and cash outflow = price * qty.
// For sells, qty < 0 and cash inflow = price * |qty|.
func recordTrade(sym string, qty decimal.Decimal, price decimal.Decimal) {
	lots := gCostBasis[sym]
	if qty.IsPositive() {
		// opening: push a new lot
		gCostBasis[sym] = append(lots, CostLot{Qty: qty, Price: price})
		return
	}

	// closing: consume lots FIFO and realize P&L
	remaining := qty.Neg() // make positive for matching
	for len(lots) > 0 && remaining.IsPositive() {
		lot := &lots[0]
		if lot.Qty.Cmp(remaining) <= 0 {
			// consume entire lot
			pnl := price.Sub(lot.Price).Mul(lot.Qty).MulInt(kMultiplier)
			gRealizedPnL = gRealizedPnL.Add(pnl)
			remaining = remaining.Sub(lot.Qty)
			lots = lots[1:]
		} else {
			// partially consume lot
			pnl := price.Sub(lot.Price).Mul(remaining).MulInt(kMultiplier)
			gRealizedPnL = gRealizedPnL.Add(pnl)
			lot.Qty = lot.Qty.Sub(remaining)
			remaining = decimal.Zero
		}
	}

	if remaining.IsPositive() {
		// opening a short position with no prior long lots
		gCostBasis[sym] = append(lots, CostLot{Qty: remaining, Price: price.Neg()})
	} else {
		gCostBasis[sym] = lots
	}
}
