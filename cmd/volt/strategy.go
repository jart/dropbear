package main

import (
	"dropbear/decimal"
	"dropbear/options"
)

const (
	kStrategyBoxArbitrage = "box arbitrage"
)

var kStrategies = []string{
	kStrategyBoxArbitrage,
}

var kStrategyDefault = map[string]bool{
	kStrategyBoxArbitrage: true,
}

var kStrategyDefaultEOD = map[string]bool{}

func (t *Trader) arbitrageBox(s *options.Strike) {
	price := t.Chain.Price
	if price.IsZero() || s == nil {
		return
	}

	window := price.Mul(t.Config.Berth)
	lo, hi := price.Sub(window), price.Add(window)
	if s.Price.Cmp(lo) < 0 || s.Price.Cmp(hi) > 0 {
		return
	}

	threshold := t.Config.MinProfit
	if threshold.IsZero() {
		threshold = decimal.FromInt(5)
	}

	// Compare the updated strike 's' against every other strike in the window.
	for _, it, _ := t.Chain.Strikes.Ceiling(lo); it != nil && it.Price.Cmp(hi) <= 0; it = it.Next {
		if it == s {
			continue
		}
		if len(t.PendingOrders) >= t.Config.MaxPending {
			break
		}

		s1, s2 := s, it
		if s1.Price.Cmp(s2.Price) > 0 {
			s1, s2 = s2, s1
		}

		if s1.Call == nil || s1.Put == nil || s2.Call == nil || s2.Put == nil {
			continue
		}

		// 1. Determine direction based on modes (No Allocation)
		direction := 0
		if s1.Call.Mode == options.ModeLong && s1.Put.Mode == options.ModeShort &&
			s2.Call.Mode == options.ModeShort && s2.Put.Mode == options.ModeLong {
			direction = 1 // Long Box
		} else if s1.Call.Mode == options.ModeShort && s1.Put.Mode == options.ModeLong &&
			s2.Call.Mode == options.ModeLong && s2.Put.Mode == options.ModeShort {
			direction = -1 // Short Box
		}
		if direction == 0 {
			continue
		}

		// 2. Check liquidity (No Allocation)
		var size uint32
		if direction == 1 {
			size = min4(s1.Call.AskSize, s1.Put.BidSize, s2.Call.BidSize, s2.Put.AskSize)
		} else {
			size = min4(s1.Call.BidSize, s1.Put.AskSize, s2.Call.AskSize, s2.Put.BidSize)
		}
		if size == 0 {
			continue
		}

		// 3. Compute price (No Allocation)
		orderPrice := t.computeBoxPrice(s1, s2, direction)
		if orderPrice.IsZero() {
			continue
		}

		// 4. Calculate Net Value
		width := s2.Price.Sub(s1.Price).MulInt(kMultiplier)
		settlement := width.MulInt(direction)
		netValue := orderPrice.MulInt(kMultiplier).Add(settlement).MulInt64(int64(size))

		// 5. Submit Order Immediately (Allocates only on success)
		// Virtual Liquidity in Order.Send() will decrement sizes for subsequent iterations.
		if netValue.Cmp(threshold) > 0 {
			legs := t.makeBoxLegs(s1, s2, direction, size)
			t.submitOrder(kStrategyBoxArbitrage, legs)
		}
	}
}

func (t *Trader) computeBoxPrice(s1, s2 *options.Strike, direction int) decimal.Decimal {
	spread := t.Config.Spread
	var p decimal.Decimal
	if direction == 1 {
		// Buy Call s1, Sell Put s1, Sell Call s2, Buy Put s2
		p = p.Sub(midSpread(s1.Call, 1, spread))
		p = p.Add(midSpread(s1.Put, -1, spread))
		p = p.Add(midSpread(s2.Call, -1, spread))
		p = p.Sub(midSpread(s2.Put, 1, spread))
	} else {
		// Sell Call s1, Buy Put s1, Buy Call s2, Sell Put s2
		p = p.Add(midSpread(s1.Call, -1, spread))
		p = p.Sub(midSpread(s1.Put, 1, spread))
		p = p.Sub(midSpread(s2.Call, 1, spread))
		p = p.Add(midSpread(s2.Put, -1, spread))
	}
	tick, bigTick := getTicks(t.Symbol)
	if p.Abs().Cmp(kThree) >= 0 {
		tick = bigTick
	}
	return p.QuantizeCeil(tick)
}

func midSpread(o *options.Option, action int, spread decimal.Decimal) decimal.Decimal {
	mid := o.MidPrice()
	hlf := o.Ask.Sub(o.Bid).DivInt(2)
	if action > 0 {
		return mid.Add(hlf.Mul(spread))
	}
	return mid.Sub(hlf.Mul(spread))
}

func (t *Trader) makeBoxLegs(s1, s2 *options.Strike, direction int, size uint32) []*Leg {
	qty := decimal.FromInt64(int64(size))
	if direction == 1 {
		return []*Leg{
			{Option: s1.Call, Quantity: qty},
			{Option: s1.Put, Quantity: qty.Neg()},
			{Option: s2.Call, Quantity: qty.Neg()},
			{Option: s2.Put, Quantity: qty},
		}
	}
	return []*Leg{
		{Option: s1.Call, Quantity: qty.Neg()},
		{Option: s1.Put, Quantity: qty},
		{Option: s2.Call, Quantity: qty},
		{Option: s2.Put, Quantity: qty.Neg()},
	}
}

func min4(a, b, c, d uint32) uint32 {
	return min(d, min(c, min(b, a)))
}
