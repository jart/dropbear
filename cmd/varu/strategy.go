package main

import (
	"dropbear/decimal"
	"dropbear/ds/options"
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
	kStrategyLiquidateCallVertical: false,
	kStrategyLiquidatePutVertical:  false,
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
	for strike := gChain.AtTheMoney.Prev; strike != nil && strike.Call.Ask.Cmp(minTick(gSymbol)) >= 0; strike = strike.Next {
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
	for strike := gChain.AtTheMoney.Next; strike != nil && strike.Put.Ask.Cmp(minTick(gSymbol)) >= 0; strike = strike.Prev {
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
	for strike := gChain.AtTheMoney.Prev; strike != nil && strike.Call.Ask.Cmp(minTick(gSymbol)) >= 0; strike = strike.Next {
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
	for strike := gChain.AtTheMoney.Next; strike != nil && strike.Put.Ask.Cmp(minTick(gSymbol)) >= 0; strike = strike.Prev {
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
		for sb := ss.Next; sb != nil && sb.Price.Cmp(hi) <= 0; sb = sb.Next {
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
	for option := range gHoldings.Positions {
		if option.Class != 'C' {
			continue
		}
		sell(option)
		end(kStrategyLiquidateCall)
	}
}

func liquidatePut() {
	if !gStrategyEnabled[kStrategyLiquidatePut] {
		return
	}
	for option := range gHoldings.Positions {
		if option.Class != 'P' {
			continue
		}
		sell(option)
		end(kStrategyLiquidatePut)
	}
}

func liquidateCallVertical() {
	if !gStrategyEnabled[kStrategyLiquidateCallVertical] {
		return
	}
	for s, sh := range gHoldings.Positions {
		if s.Class != 'C' || sh.Quantity.IsPositive() {
			continue // need a short call
		}
		for l, lh := range gHoldings.Positions {
			if l.Class != 'C' || lh.Quantity.IsNegative() {
				continue // need a long call
			}
			if prune() {
				continue
			}
			if !isSpreadProfitableToClose(s, sh, l, lh) {
				continue
			}
			buy(s)
			sell(l)
			end(kStrategyLiquidateCallVertical)
		}
	}
}

func liquidatePutVertical() {
	if !gStrategyEnabled[kStrategyLiquidatePutVertical] {
		return
	}
	for s, sh := range gHoldings.Positions {
		if s.Class != 'P' || sh.Quantity.IsPositive() {
			continue // need a short put
		}
		for l, lh := range gHoldings.Positions {
			if l.Class != 'P' || lh.Quantity.IsNegative() {
				continue // need a long put
			}
			if prune() {
				continue
			}
			if !isSpreadProfitableToClose(s, sh, l, lh) {
				continue
			}
			buy(s)
			sell(l)
			end(kStrategyLiquidatePutVertical)
		}
	}
}

func isSpreadProfitableToClose(short *options.Option, sh *Holding, long *options.Option, lh *Holding) bool {
	openCredit := sh.AverageCost.Sub(lh.AverageCost)
	closeCost := short.MidPrice().Sub(long.MidPrice())
	return closeCost.Mul(*demandFlag).Cmp(openCredit) < 0
}
