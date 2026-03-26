package main

import (
	"dropbear/decimal"
	"fmt"
)

// gTotalCashIn tracks the total cash received from all sells.
// gTotalCashOut tracks the total cash paid for all buys.
// These are independent of the cost basis system, providing a
// second ledger to cross-check against.
var (
	gTotalCashIn  decimal.Decimal
	gTotalCashOut decimal.Decimal
	gDrift        decimal.Decimal
)

// sanityCheck validates internal consistency of the accounting state.
// Call after every state mutation (fills, settlements, simulation).
func sanityCheck(context string) {

	// Every position must have a cost basis entry, and vice versa.
	for it := gPositions.Iterator(); it.Next(); {
		sym, qty := it.Key(), it.Value()
		if qty.IsZero() {
			continue
		}
		if _, ok := gCostPerContract[sym]; !ok {
			panic(fmt.Sprintf("sanity(%s): position %s qty=%s has no cost basis", context, sym, qty))
		}
	}
	for sym, cost := range gCostPerContract {
		qty, _ := gPositions.Get(sym)
		if qty.IsZero() {
			panic(fmt.Sprintf("sanity(%s): cost basis %s for zero position %s", context, cost, sym))
		}
	}

	// Cost basis must always be positive (it's a price, not a P&L).
	for sym, cost := range gCostPerContract {
		if !cost.IsPositive() {
			panic(fmt.Sprintf("sanity(%s): non-positive cost basis %s for %s", context, cost, sym))
		}
	}

	// Every position must reference a known option.
	for it := gPositions.Iterator(); it.Next(); {
		sym, qty := it.Key(), it.Value()
		if qty.IsZero() {
			continue
		}
		if gOptionsByOSI[sym] == nil {
			panic(fmt.Sprintf("sanity(%s): position %s has no option definition", context, sym))
		}
	}

	// Accounting identity: cash should equal the sum of all individual
	// cash flows. gCash is updated per-trade, gTotalCashIn/Out track
	// the same flows independently. If they diverge, something double-
	// counted or skipped a leg.
	expectedCash := gTotalCashIn.Sub(gTotalCashOut)
	if gCash.Cmp(expectedCash) != 0 {
		panic(fmt.Sprintf("sanity(%s): cash=%s but expected=%s (in=%s out=%s)",
			context, gCash, expectedCash, gTotalCashIn, gTotalCashOut))
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
	for it := gPositions.Iterator(); it.Next(); {
		sym, qty := it.Key(), it.Value()
		if qty.IsZero() {
			continue
		}
		cost := gCostPerContract[sym]
		unrealizedCost = unrealizedCost.Sub(cost.Mul(qty).MulInt(kMultiplier))
	}
	expectedFromBooks := gRealizedPnL.Add(unrealizedCost)
	gDrift = gCash.Sub(expectedFromBooks)
	if gDrift.Abs().Cmp(*maxDriftFlag) > 0 {
		panic(fmt.Sprintf("sanity(%s): drift=%s exceeds max %s (cash=%s realized=%s unrealized_cost=%s)",
			context, gDrift, *maxDriftFlag, gCash, gRealizedPnL, unrealizedCost))
	}
}
