package main

import (
	"dropbear/decimal"
	"dropbear/ds/options"
	"dropbear/ds/osi"
)

const (
	kStrategyBuyCall               = "buy call"
	kStrategyBuyPut                = "buy put"
	kStrategySellCall              = "sell call"
	kStrategySellPut               = "sell put"
	kStrategyBuyCombo              = "buy combo"
	kStrategySellCombo             = "sell combo"
	kStrategySellCallVertical      = "sell call vertical"
	kStrategySellPutVertical       = "sell put vertical"
	kStrategyBuyCallVertical       = "buy call vertical"
	kStrategyBuyPutVertical        = "buy put vertical"
	kStrategyLiquidateCall         = "liquidate call"
	kStrategyLiquidatePut          = "liquidate put"
	kStrategyLiquidateCallVertical = "liquidate call vertical"
	kStrategyLiquidatePutVertical  = "liquidate put vertical"
)

var gStrategyEnabled = map[string]bool{
	kStrategyBuyCall:               false,
	kStrategyBuyPut:                false,
	kStrategySellCall:              false,
	kStrategySellPut:               false,
	kStrategyBuyCombo:              false,
	kStrategySellCombo:             false,
	kStrategySellCallVertical:      true,
	kStrategySellPutVertical:       true,
	kStrategyBuyCallVertical:       false,
	kStrategyBuyPutVertical:        false,
	kStrategyLiquidateCall:         false,
	kStrategyLiquidatePut:          false,
	kStrategyLiquidateCallVertical: true,
	kStrategyLiquidatePutVertical:  true,
}

var gStrategyEnabledEOD = map[string]bool{
	kStrategyLiquidateCall:         false,
	kStrategyLiquidatePut:          false,
	kStrategyLiquidateCallVertical: true,
	kStrategyLiquidatePutVertical:  true,
}

func buyCall() {
	if !gStrategyEnabled[kStrategyBuyCall] {
		return
	}
	for strike := gChain.AtTheMoney.Prev; strike != nil && strike.Call.Ask.Cmp(minTick()) >= 0; strike = strike.Next {
		if prune() {
			continue
		}
		buy(strike.Call)
		end(kStrategyBuyCall)
	}
}

func buyPut() {
	if !gStrategyEnabled[kStrategyBuyPut] {
		return
	}
	for strike := gChain.AtTheMoney.Next; strike != nil && strike.Put.Ask.Cmp(minTick()) >= 0; strike = strike.Prev {
		if prune() {
			continue
		}
		buy(strike.Put)
		end(kStrategyBuyPut)
	}
}

func sellCall() {
	if !gStrategyEnabled[kStrategySellCall] {
		return
	}
	for strike := gChain.AtTheMoney.Prev; strike != nil && strike.Call.Ask.Cmp(minTick()) >= 0; strike = strike.Next {
		if prune() {
			continue
		}
		sell(strike.Call)
		end(kStrategySellCall)
	}
}

func sellPut() {
	if !gStrategyEnabled[kStrategySellPut] {
		return
	}
	for strike := gChain.AtTheMoney.Next; strike != nil && strike.Put.Ask.Cmp(minTick()) >= 0; strike = strike.Prev {
		if prune() {
			continue
		}
		sell(strike.Put)
		end(kStrategySellPut)
	}
}

func sellCallVertical(lo, hi decimal.Decimal) {
	if !gStrategyEnabled[kStrategySellCallVertical] {
		return
	}
	for _, ss, _ := gChain.Strikes.Ceiling(lo); ss != nil && ss.Price.Cmp(hi) <= 0; ss = ss.Next {
		for sb := ss.Prev; sb != nil && sb.Price.Cmp(hi) <= 0; sb = sb.Next {
			if prune() {
				continue
			}
			sell(ss.Call)
			buy(sb.Call)
			end(kStrategySellCallVertical)
		}
	}
}

func sellPutVertical(lo, hi decimal.Decimal) {
	if !gStrategyEnabled[kStrategySellPutVertical] {
		return
	}
	for _, ss, _ := gChain.Strikes.Ceiling(hi); ss != nil && ss.Price.Cmp(lo) >= 0; ss = ss.Prev {
		for sb := ss.Prev; sb != nil && sb.Price.Cmp(lo) >= 0; sb = sb.Prev {
			if prune() {
				continue
			}
			sell(ss.Put)
			buy(sb.Put)
			end(kStrategySellPutVertical)
		}
	}
}

func buyCallVertical(lo, hi decimal.Decimal) {
	if !gStrategyEnabled[kStrategyBuyCallVertical] {
		return
	}
	for _, sb, _ := gChain.Strikes.Ceiling(lo); sb != nil && sb.Price.Cmp(hi) <= 0; sb = sb.Next {
		for ss := sb.Next; ss != nil && ss.Price.Cmp(hi) <= 0; ss = ss.Next {
			if prune() {
				continue
			}
			buy(sb.Call)
			sell(ss.Call)
			end(kStrategyBuyCallVertical)
		}
	}
}

func buyPutVertical(lo, hi decimal.Decimal) {
	if !gStrategyEnabled[kStrategyBuyPutVertical] {
		return
	}
	for _, sb, _ := gChain.Strikes.Ceiling(hi); sb != nil && sb.Price.Cmp(lo) >= 0; sb = sb.Prev {
		for ss := sb.Prev; ss != nil && ss.Price.Cmp(lo) >= 0; ss = ss.Prev {
			if prune() {
				continue
			}
			buy(sb.Put)
			sell(ss.Put)
			end(kStrategyBuyPutVertical)
		}
	}
}

func buyCombo(lo, hi decimal.Decimal) {
	if !gStrategyEnabled[kStrategyBuyCombo] {
		return
	}
	for _, strike, _ := gChain.Strikes.Floor(lo); strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		if prune() {
			continue
		}
		buy(strike.Call)
		sell(strike.Put)
		if end(kStrategyBuyCombo) {
			break
		}
	}
}

func sellCombo(lo, hi decimal.Decimal) {
	if !gStrategyEnabled[kStrategySellCombo] {
		return
	}
	for _, strike, _ := gChain.Strikes.Floor(lo); strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		if prune() {
			continue
		}
		sell(strike.Call)
		buy(strike.Put)
		if end(kStrategySellCombo) {
			break
		}
	}
}

func liquidateCall() {
	if !gStrategyEnabled[kStrategyLiquidateCall] {
		return
	}
	// only sell calls we purchased earlier
	for it := gPositions.Iterator(); it.Next(); {
		sym, qty := it.Key(), it.Value()
		if qty <= 0 {
			continue
		}
		_, strikePrice, class, _, _, _, _ := osi.Parse(sym)
		if class != 'C' {
			continue
		}
		strike, _ := gChain.Strikes.Get(strikePrice)
		if strike == nil {
			continue
		}
		sell(strike.Call)
		end(kStrategyLiquidateCall)
	}
}

func liquidatePut() {
	if !gStrategyEnabled[kStrategyLiquidatePut] {
		return
	}
	// only sell puts we purchased earlier
	for it := gPositions.Iterator(); it.Next(); {
		sym, qty := it.Key(), it.Value()
		if qty <= 0 {
			continue
		}
		_, strikePrice, class, _, _, _, _ := osi.Parse(sym)
		if class != 'P' {
			continue
		}
		strike, _ := gChain.Strikes.Get(strikePrice)
		if strike == nil {
			continue
		}
		sell(strike.Put)
		end(kStrategyLiquidatePut)
	}
}

// liquidateCallVertical closes an existing call vertical by buying back the
// short leg and selling the long leg. Iterates over all pairs of held call
// positions where one is short and one is long.
// liquidateCallVertical closes an existing call vertical by buying back
// the short leg and selling the long leg. Only considers spreads where
// closing is profitable: the net cost to close (buy back short, sell
// long) must be less than the net credit we originally received. This
// prevents churning while allowing the robot to take profits on spreads
// where theta has decayed in our favor.
func liquidateCallVertical() {
	if !gStrategyEnabled[kStrategyLiquidateCallVertical] {
		return
	}
	shorts, longs := collectPositions('C')
	for _, s := range shorts {
		for _, l := range longs {
			if prune() {
				continue
			}
			if !isSpreadProfitableToClose(s, l) {
				continue
			}
			gStagedPositions.Put(s.OSI(), decimal.One)    // buy back short
			gStagedPositions.Put(l.OSI(), decimal.NegOne) // sell the long
			end(kStrategyLiquidateCallVertical)
		}
	}
}

// liquidatePutVertical closes an existing put vertical by buying back
// the short leg and selling the long leg.
func liquidatePutVertical() {
	if !gStrategyEnabled[kStrategyLiquidatePutVertical] {
		return
	}
	shorts, longs := collectPositions('P')
	for _, s := range shorts {
		for _, l := range longs {
			if prune() {
				continue
			}
			if !isSpreadProfitableToClose(s, l) {
				continue
			}
			gStagedPositions.Put(s.OSI(), decimal.One)    // buy back short
			gStagedPositions.Put(l.OSI(), decimal.NegOne) // sell the long
			end(kStrategyLiquidatePutVertical)
		}
	}
}

// isSpreadProfitableToClose checks whether closing a spread (buying back
// the short leg, selling the long leg) would be profitable based on cost
// basis. We originally received: shortCost - longCost (net credit).
// To close we pay: shortMid - longMid (net debit to buy back short,
// minus credit from selling long). Profitable when close cost < open credit.
func isSpreadProfitableToClose(short, long *options.Option) bool {
	shortCost := gCostPerContract[short.OSI()]
	longCost := gCostPerContract[long.OSI()]
	openCredit := shortCost.Sub(longCost)
	closeCost := short.MarketPrice().Sub(long.MarketPrice())
	return closeCost.Mul(*demandFlag).Cmp(openCredit) < 0
}

// collectPositions returns the short and long options of the given class
// from current positions.
func collectPositions(class byte) (shorts, longs []*options.Option) {
	for it := gPositions.Iterator(); it.Next(); {
		sym, qty := it.Key(), it.Value()
		if qty.IsZero() {
			continue
		}
		_, _, c, _, _, _, _ := osi.Parse(sym)
		if c != class {
			continue
		}
		opt := gOptionsByOSI[sym]
		if opt == nil {
			continue
		}
		if qty.IsNegative() {
			shorts = append(shorts, opt)
		} else {
			longs = append(longs, opt)
		}
	}
	return
}
