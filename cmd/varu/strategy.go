package main

import (
	"dropbear/decimal"
	"dropbear/ds/options"
)

const (
	kStrategyBuyCall          = "buy call"
	kStrategyBuyPut           = "buy put"
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
	kStrategyBuyCombo:         true,
	kStrategySellCombo:        true,
	kStrategySellCallVertical: true,
	kStrategySellPutVertical:  true,
	kStrategyBuyCallVertical:  false,
	kStrategyBuyPutVertical:   false,
	kStrategySellCondor:       false,
	kStrategyBuyCondor:        false,
}

func simulateBuyCalls(hi decimal.Decimal) {
	for strike := gChain.AtTheMoney; strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		buy(strike.Call)
		endSimulation(kStrategyBuyCall)
	}
}

func simulateBuyPuts(lo decimal.Decimal) {
	for strike := gChain.AtTheMoney; strike != nil && strike.Price.Cmp(lo) >= 0; strike = strike.Prev {
		buy(strike.Put)
		endSimulation(kStrategyBuyPut)
	}
}

func simulateSellCallVerticals(hi decimal.Decimal) {
	for _, ss := range []*options.Strike{gChain.AtTheMoney.Prev, gChain.AtTheMoney, gChain.AtTheMoney.Next} {
		for sb := gChain.AtTheMoney; sb != nil && sb.Price.Cmp(hi) <= 0; sb = sb.Next {
			sell(ss.Call)
			buy(sb.Call)
			endSimulation(kStrategySellCallVertical)
		}
	}
}

func simulateSellPutVerticals(lo decimal.Decimal) {
	for _, ss := range []*options.Strike{gChain.AtTheMoney.Prev, gChain.AtTheMoney, gChain.AtTheMoney.Next} {
		for sb := gChain.AtTheMoney; sb != nil && sb.Price.Cmp(lo) >= 0; sb = sb.Prev {
			sell(ss.Put)
			buy(sb.Put)
			endSimulation(kStrategySellPutVertical)
		}
	}
}

func simulateBuyCallVerticals(hi decimal.Decimal) {
	for _, sb := range []*options.Strike{gChain.AtTheMoney.Prev, gChain.AtTheMoney, gChain.AtTheMoney.Next} {
		for ss := gChain.AtTheMoney; ss != nil && ss.Price.Cmp(hi) <= 0; ss = ss.Next {
			buy(sb.Call)
			sell(ss.Call)
			endSimulation(kStrategyBuyCallVertical)
		}
	}
}

func simulateBuyPutVerticals(lo decimal.Decimal) {
	for _, sb := range []*options.Strike{gChain.AtTheMoney.Prev, gChain.AtTheMoney, gChain.AtTheMoney.Next} {
		for ss := gChain.AtTheMoney; ss != nil && ss.Price.Cmp(lo) >= 0; ss = ss.Prev {
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
