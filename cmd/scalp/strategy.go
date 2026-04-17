package main

import (
	"dropbear/decimal"
	"dropbear/options"
)

const (
	kStrategyBuyPut                = "buy put"
	kStrategyBuyCall               = "buy call"
	kStrategySellPut               = "naked sell put"
	kStrategySellCall              = "naked sell call"
	kStrategyBuyCombo              = "combo buy"
	kStrategySellCombo             = "combo sell"
	kStrategySellPutVertical       = "vertical sell put"
	kStrategySellCallVertical      = "vertical sell call"
	kStrategyBuyPutVertical        = "vertical buy put"
	kStrategyBuyCallVertical       = "vertical buy call"
	kStrategyLiquidatePut          = "liquidate put"
	kStrategyLiquidateCall         = "liquidate call"
	kStrategyLiquidatePutVertical  = "liquidate vertical put"
	kStrategyLiquidateCallVertical = "liquidate vertical call"
	kStrategyLiquidatePair         = "liquidate pair"
	kStrategyBuyStrangle           = "strangle buy"
)

var kStrategies = []string{
	kStrategyBuyCall,
	kStrategyBuyPut,
	kStrategySellCall,
	kStrategySellPut,
	kStrategyBuyCombo,
	kStrategySellCombo,
	kStrategyBuyStrangle,
	kStrategySellCallVertical,
	kStrategySellPutVertical,
	kStrategyBuyCallVertical,
	kStrategyBuyPutVertical,
	kStrategyLiquidateCall,
	kStrategyLiquidatePut,
	kStrategyLiquidateCallVertical,
	kStrategyLiquidatePutVertical,
	kStrategyLiquidatePair,
}

var kStrategyDefault = map[string]bool{
	kStrategySellCallVertical: true,
	kStrategySellPutVertical:  true,
}

var kStrategyDefaultEOD = map[string]bool{
	kStrategyLiquidateCall: true,
	kStrategyLiquidatePut:  true,
}

func (t *Trader) buyCall() {
	if !t.Config.Strategies[kStrategyBuyCall] {
		return
	}
	for strike := t.Chain.AtTheMoney.Prev; strike != nil && strike.Call.Ask.Cmp(minTick(t.Symbol)) >= 0; strike = strike.Next {
		if t.prune() {
			continue
		}
		t.buy(strike.Call)
		t.end(kStrategyBuyCall)
	}
}

func (t *Trader) buyPut() {
	if !t.Config.Strategies[kStrategyBuyPut] {
		return
	}
	for strike := t.Chain.AtTheMoney.Next; strike != nil && strike.Put.Ask.Cmp(minTick(t.Symbol)) >= 0; strike = strike.Prev {
		if t.prune() {
			continue
		}
		t.buy(strike.Put)
		t.end(kStrategyBuyPut)
	}
}

func (t *Trader) sellCall() {
	if !t.Config.Strategies[kStrategySellCall] {
		return
	}
	for strike := t.Chain.AtTheMoney.Prev; strike != nil && strike.Call.Ask.Cmp(minTick(t.Symbol)) >= 0; strike = strike.Next {
		if t.prune() {
			continue
		}
		t.sell(strike.Call)
		t.end(kStrategySellCall)
	}
}

func (t *Trader) sellPut() {
	if !t.Config.Strategies[kStrategySellPut] {
		return
	}
	for strike := t.Chain.AtTheMoney.Next; strike != nil && strike.Put.Ask.Cmp(minTick(t.Symbol)) >= 0; strike = strike.Prev {
		if t.prune() {
			continue
		}
		t.sell(strike.Put)
		t.end(kStrategySellPut)
	}
}

func (t *Trader) buyStrangle(lo, hi decimal.Decimal) {
	if !t.Config.Strategies[kStrategyBuyStrangle] {
		return
	}
	for _, s1, _ := t.Chain.Strikes.Floor(lo); s1 != nil && s1.Price.Cmp(hi) <= 0; s1 = s1.Next {
		for _, s2, _ := t.Chain.Strikes.Floor(lo); s2 != nil && s2.Price.Cmp(hi) <= 0; s2 = s2.Next {
			if t.prune() {
				continue
			}
			t.buy(s1.Call)
			t.buy(s2.Put)
			t.end(kStrategyBuyStrangle)
		}
	}
}

func (t *Trader) sellCallVertical(lo, hi decimal.Decimal) {
	if !t.Config.Strategies[kStrategySellCallVertical] {
		return
	}
	for _, ss, _ := t.Chain.Strikes.Ceiling(lo); ss != nil && ss.Price.Cmp(hi) <= 0; ss = ss.Next {
		for sb := ss.Next; sb != nil && sb.Price.Cmp(hi) <= 0; sb = sb.Next {
			if t.prune() {
				continue
			}
			t.sell(ss.Call)
			t.buy(sb.Call)
			t.end(kStrategySellCallVertical)
		}
	}
}

func (t *Trader) sellPutVertical(lo, hi decimal.Decimal) {
	if !t.Config.Strategies[kStrategySellPutVertical] {
		return
	}
	for _, ss, _ := t.Chain.Strikes.Ceiling(hi); ss != nil && ss.Price.Cmp(lo) >= 0; ss = ss.Prev {
		for sb := ss.Prev; sb != nil && sb.Price.Cmp(lo) >= 0; sb = sb.Prev {
			if t.prune() {
				continue
			}
			t.sell(ss.Put)
			t.buy(sb.Put)
			t.end(kStrategySellPutVertical)
		}
	}
}

func (t *Trader) buyCallVertical(lo, hi decimal.Decimal) {
	if !t.Config.Strategies[kStrategyBuyCallVertical] {
		return
	}
	for _, sb, _ := t.Chain.Strikes.Ceiling(lo); sb != nil && sb.Price.Cmp(hi) <= 0; sb = sb.Next {
		for ss := sb.Next; ss != nil && ss.Price.Cmp(hi) <= 0; ss = ss.Next {
			if t.prune() {
				continue
			}
			t.buy(sb.Call)
			t.sell(ss.Call)
			t.end(kStrategyBuyCallVertical)
		}
	}
}

func (t *Trader) buyPutVertical(lo, hi decimal.Decimal) {
	if !t.Config.Strategies[kStrategyBuyPutVertical] {
		return
	}
	for _, sb, _ := t.Chain.Strikes.Ceiling(hi); sb != nil && sb.Price.Cmp(lo) >= 0; sb = sb.Prev {
		for ss := sb.Prev; ss != nil && ss.Price.Cmp(lo) >= 0; ss = ss.Prev {
			if t.prune() {
				continue
			}
			t.buy(sb.Put)
			t.sell(ss.Put)
			t.end(kStrategyBuyPutVertical)
		}
	}
}

func (t *Trader) buyCombo(lo, hi decimal.Decimal) {
	if !t.Config.Strategies[kStrategyBuyCombo] {
		return
	}
	for _, strike, _ := t.Chain.Strikes.Floor(lo); strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		if t.prune() {
			continue
		}
		t.buy(strike.Call)
		t.sell(strike.Put)
		if t.end(kStrategyBuyCombo) {
			break
		}
	}
}

func (t *Trader) sellCombo(lo, hi decimal.Decimal) {
	if !t.Config.Strategies[kStrategySellCombo] {
		return
	}
	for _, strike, _ := t.Chain.Strikes.Floor(lo); strike != nil && strike.Price.Cmp(hi) <= 0; strike = strike.Next {
		if t.prune() {
			continue
		}
		t.sell(strike.Call)
		t.buy(strike.Put)
		if t.end(kStrategySellCombo) {
			break
		}
	}
}

func (t *Trader) liquidateCall() {
	if !t.Config.Strategies[kStrategyLiquidateCall] {
		return
	}
	for x, xh := range t.Holdings.Positions {
		if x.Class != 'C' || x.IntrinsicValue(t.Chain.Price).IsZero() {
			continue
		}
		qty := xh.Quantity.Abs()
		if xh.Quantity.IsPositive() {
			t.sellN(x, qty)
		} else {
			t.buyN(x, qty)
		}
		t.end(kStrategyLiquidateCall)
	}
}

func (t *Trader) liquidatePut() {
	if !t.Config.Strategies[kStrategyLiquidatePut] {
		return
	}
	for x, xh := range t.Holdings.Positions {
		if x.Class != 'P' || x.IntrinsicValue(t.Chain.Price).IsZero() {
			continue
		}
		qty := xh.Quantity.Abs()
		if xh.Quantity.IsPositive() {
			t.sellN(x, qty)
		} else {
			t.buyN(x, qty)
		}
		t.end(kStrategyLiquidatePut)
	}
}

func (t *Trader) liquidateCallVertical() {
	if !t.Config.Strategies[kStrategyLiquidateCallVertical] {
		return
	}
	for s, sh := range t.Holdings.Positions {
		if s.Class != 'C' || sh.Quantity.IsPositive() {
			continue // need a short call
		}
		for l, lh := range t.Holdings.Positions {
			if s == l {
				continue
			}
			if l.Class != 'C' || lh.Quantity.IsNegative() {
				continue // need a long call
			}
			if !t.isSpreadProfitableToClose(s, sh, l, lh) {
				continue
			}
			qty := sh.Quantity.Abs().Min(lh.Quantity.Abs())
			t.buyN(s, qty)
			t.sellN(l, qty)
			t.end(kStrategyLiquidateCallVertical)
		}
	}
}

func (t *Trader) liquidatePutVertical() {
	if !t.Config.Strategies[kStrategyLiquidatePutVertical] {
		return
	}
	for s, sh := range t.Holdings.Positions {
		if s.Class != 'P' || sh.Quantity.IsPositive() {
			continue // need a short put
		}
		for l, lh := range t.Holdings.Positions {
			if s == l {
				continue
			}
			if l.Class != 'P' || lh.Quantity.IsNegative() {
				continue // need a long put
			}
			if !t.isSpreadProfitableToClose(s, sh, l, lh) {
				continue
			}
			qty := sh.Quantity.Abs().Min(lh.Quantity.Abs())
			t.buyN(s, qty)
			t.sellN(l, qty)
			t.end(kStrategyLiquidatePutVertical)
		}
	}
}

func (t *Trader) liquidatePair() {
	if !t.Config.Strategies[kStrategyLiquidatePair] {
		return
	}
	for x, xh := range t.Holdings.Positions {
		for y, yh := range t.Holdings.Positions {
			if x == y {
				continue
			}
			if x.OSI() >= y.OSI() {
				continue
			}
			qty := xh.Quantity.Abs().Min(yh.Quantity.Abs())
			if xh.Quantity.IsPositive() {
				t.sellN(x, qty)
			} else {
				t.buyN(x, qty)
			}
			if yh.Quantity.IsPositive() {
				t.sellN(y, qty)
			} else {
				t.buyN(y, qty)
			}
			t.end(kStrategyLiquidatePair)
		}
	}
}

func (t *Trader) isSpreadProfitableToClose(short *options.Option, sh *Holding, long *options.Option, lh *Holding) bool {
	openCredit := sh.AverageCost.Sub(lh.AverageCost)
	closeCost := short.MidPrice().Sub(long.MidPrice())
	return closeCost.Mul(t.Config.Demand).Cmp(openCredit) < 0
}
