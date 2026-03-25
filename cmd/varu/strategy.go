package main

import (
	"dropbear/decimal"
	"dropbear/ds/options"
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
	kStrategySellCondor       = "sell condor"
	kStrategyBuyCondor        = "buy condor"
)

var gStrategyEnabled = map[string]bool{
	kStrategyBuyCall:          true,
	kStrategyBuyPut:           true,
	kStrategySellCall:         true,
	kStrategySellPut:          true,
	kStrategyBuyCombo:         true,
	kStrategySellCombo:        true,
	kStrategySellCallVertical: true,
	kStrategySellPutVertical:  true,
	kStrategyBuyCallVertical:  false,
	kStrategyBuyPutVertical:   false,
	kStrategySellCondor:       false,
	kStrategyBuyCondor:        false,
}

var gStrategyEnabledEOD = map[string]bool{
	kStrategySellCall: true,
	kStrategySellPut:  true,
}

func simulateBuyCalls() {
	// buying calls make sense at the money
	// all the way otm to when they hit minimum cost
	tick := optionTick(gChain.AtTheMoney.Call.Ask)
	for strike := gChain.AtTheMoney.Prev; strike != nil && strike.Call.Ask.Cmp(tick) >= 0; strike = strike.Next {
		buy(strike.Call)
		endSimulation(kStrategyBuyCall)
	}
}

func simulateBuyPuts() {
	// buying puts make sense at the money
	// all the way otm to when they hit minimum cost
	tick := optionTick(gChain.AtTheMoney.Put.Ask)
	for strike := gChain.AtTheMoney.Next; strike != nil && strike.Put.Ask.Cmp(tick) >= 0; strike = strike.Prev {
		buy(strike.Put)
		endSimulation(kStrategyBuyPut)
	}
}

func simulateSellCalls() {
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
		endSimulation(kStrategySellCall)
	}
}

func simulateSellPuts() {
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
		endSimulation(kStrategySellPut)
	}
}

func simulateSellCallVerticals(hi decimal.Decimal) {
	for _, ss := range []*options.Strike{gChain.AtTheMoney.Prev, gChain.AtTheMoney, gChain.AtTheMoney.Next} {
		for sb := gChain.AtTheMoney; sb != nil && sb.Price.Cmp(hi) <= 0; sb = sb.Next {
			if sb == ss {
				continue
			}
			sell(ss.Call)
			buy(sb.Call)
			endSimulation(kStrategySellCallVertical)
		}
	}
}

func simulateSellPutVerticals(lo decimal.Decimal) {
	for _, ss := range []*options.Strike{gChain.AtTheMoney.Prev, gChain.AtTheMoney, gChain.AtTheMoney.Next} {
		for sb := gChain.AtTheMoney; sb != nil && sb.Price.Cmp(lo) >= 0; sb = sb.Prev {
			if sb == ss {
				continue
			}
			sell(ss.Put)
			buy(sb.Put)
			endSimulation(kStrategySellPutVertical)
		}
	}
}

func simulateBuyCallVerticals(hi decimal.Decimal) {
	for _, sb := range []*options.Strike{gChain.AtTheMoney.Prev, gChain.AtTheMoney, gChain.AtTheMoney.Next} {
		for ss := gChain.AtTheMoney; ss != nil && ss.Price.Cmp(hi) <= 0; ss = ss.Next {
			if sb == ss {
				continue
			}
			buy(sb.Call)
			sell(ss.Call)
			endSimulation(kStrategyBuyCallVertical)
		}
	}
}

func simulateBuyPutVerticals(lo decimal.Decimal) {
	for _, sb := range []*options.Strike{gChain.AtTheMoney.Prev, gChain.AtTheMoney, gChain.AtTheMoney.Next} {
		for ss := gChain.AtTheMoney; ss != nil && ss.Price.Cmp(lo) >= 0; ss = ss.Prev {
			if sb == ss {
				continue
			}
			buy(sb.Put)
			sell(ss.Put)
			endSimulation(kStrategyBuyPutVertical)
		}
	}
}

func simulateBuyCombo(lo, hi decimal.Decimal) {
	for _, strike, _ := gChain.Strikes.Floor(lo); strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		buy(strike.Call)
		sell(strike.Put)
		if endSimulation(kStrategyBuyCombo) {
			break
		}
	}
}

func simulateSellCombo(lo, hi decimal.Decimal) {
	for _, strike, _ := gChain.Strikes.Floor(lo); strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		sell(strike.Call)
		buy(strike.Put)
		if endSimulation(kStrategySellCombo) {
			break
		}
	}
}
