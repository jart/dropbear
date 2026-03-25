package main

import (
	"dropbear/decimal"
	"dropbear/ds/osi"
)

const (
	kStrategyBuyCall          = "buy call"
	kStrategyBuyPut           = "buy put"
	kStrategySellCall         = "sell call"
	kStrategySellPut          = "sell put"
	kStrategyBuyCombo         = "buy combo"
	kStrategySellCombo        = "sell combo"
	kStrategySellCallVertical = "sell call vertical"
	kStrategySellPutVertical  = "sell put vertical"
	kStrategyBuyCallVertical  = "buy call vertical"
	kStrategyBuyPutVertical   = "buy put vertical"
)

var gStrategyEnabled = map[string]bool{
	kStrategyBuyCall:          false,
	kStrategyBuyPut:           false,
	kStrategySellCall:         false,
	kStrategySellPut:          false,
	kStrategyBuyCombo:         false,
	kStrategySellCombo:        false,
	kStrategySellCallVertical: true,
	kStrategySellPutVertical:  true,
	kStrategyBuyCallVertical:  false,
	kStrategyBuyPutVertical:   false,
}

var gStrategyEnabledEOD = map[string]bool{
	kStrategySellCall: true,
	kStrategySellPut:  true,
}

func buyCall() {
	if !gStrategyEnabled[kStrategyBuyCall] {
		return
	}
	// buying calls make sense at the money
	// all the way otm to when they hit minimum cost
	for strike := gChain.AtTheMoney.Prev; strike != nil && strike.Call.Ask.Cmp(minTick()) >= 0; strike = strike.Next {
		if prune() {
			continue
		}
		buyWithTheForce(strike.Call)
		end(kStrategyBuyCall)
	}
}

func buyPut() {
	if !gStrategyEnabled[kStrategyBuyPut] {
		return
	}
	// buying puts make sense at the money
	// all the way otm to when they hit minimum cost
	for strike := gChain.AtTheMoney.Next; strike != nil && strike.Put.Ask.Cmp(minTick()) >= 0; strike = strike.Prev {
		if prune() {
			continue
		}
		buyWithTheForce(strike.Put)
		end(kStrategyBuyPut)
	}
}

func sellCall() {
	if !gStrategyEnabled[kStrategySellCall] {
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
		sellWithTheForce(strike.Call)
		end(kStrategySellCall)
	}
}

func sellPut() {
	if !gStrategyEnabled[kStrategySellPut] {
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
		sellWithTheForce(strike.Put)
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
