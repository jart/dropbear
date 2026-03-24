package main

import "dropbear/decimal"

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
	kStrategyBuyCallVertical:  true,
	kStrategyBuyPutVertical:   true,
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
	for strike := gChain.AtTheMoney; strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		sell(gChain.AtTheMoney.Prev.Call)
		buy(strike.Call)
		endSimulation(kStrategySellCallVertical)
	}
}

func simulateSellPutVerticals(lo decimal.Decimal) {
	for strike := gChain.AtTheMoney; strike != nil && strike.Price.Cmp(lo) >= 0; strike = strike.Prev {
		sell(gChain.AtTheMoney.Next.Put)
		buy(strike.Put)
		endSimulation(kStrategySellPutVertical)
	}
}

func simulateBuyCallVerticals(hi decimal.Decimal) {
	for strike := gChain.AtTheMoney; strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		buy(gChain.AtTheMoney.Prev.Call)
		sell(strike.Call)
		endSimulation(kStrategyBuyCallVertical)
	}
}

func simulateBuyPutVerticals(lo decimal.Decimal) {
	for strike := gChain.AtTheMoney; strike != nil && strike.Price.Cmp(lo) >= 0; strike = strike.Prev {
		buy(gChain.AtTheMoney.Next.Put)
		sell(strike.Put)
		endSimulation(kStrategyBuyPutVertical)
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
