package main

import (
	"dropbear/broker/databento"
	"dropbear/decimal"
	"fmt"
)

type Box struct {
	CallLeg1 *Leg
	CallLeg2 *Leg
	PutLeg1  *Leg
	PutLeg2  *Leg
}

func (b *Box) String() string {
	direction := "sell"
	if b.IsBuying() {
		direction = "buy"
	}
	return fmt.Sprintf("%s %sw box @ %s (market=%s profit=%s)\n\t%s\n\t%s\n\t%s\n\t%s",
		direction, b.Width(), b.LimitPrice(), b.MarketPrice(), b.LimitProfit(),
		b.CallLeg1, b.CallLeg2, b.PutLeg1, b.PutLeg2)
}

// Width returns the distance between the strikes of the box.
// This will be negative for a sell (credit) box and positive for a buy (debit) box.
func (b *Box) Width() decimal.Decimal {
	return b.CallLeg2.Option.Strike.Sub(b.CallLeg1.Option.Strike)
}

// IsBuying returns true if this is a buy box (net debit), false if it's a sell box (net credit).
func (b *Box) IsBuying() bool {
	return b.CallLeg1.Option.Strike.Cmp(b.CallLeg2.Option.Strike) < 0
}

func (b *Box) ApplyGreed() {
	b.CallLeg1.ApplyGreed()
	b.CallLeg2.ApplyGreed()
	b.PutLeg1.ApplyGreed()
	b.PutLeg2.ApplyGreed()
}

// LimitPrice returns the net credit (positive) or debit (negative) of the box based on leg limit prices.
func (b *Box) LimitPrice() decimal.Decimal {
	return b.CallLeg1.LimitPrice.
		Add(b.CallLeg2.LimitPrice).
		Add(b.PutLeg1.LimitPrice).
		Add(b.PutLeg2.LimitPrice)
}

// LimitProfit returns the minimum profit of the box based on leg limit prices.
func (b *Box) LimitProfit() decimal.Decimal {
	return b.Width().Add(b.LimitPrice())
}

// FillPrice returns the net credit (positive) or debit (negative) of the box based on leg fill prices.
func (b *Box) FillPrice() decimal.Decimal {
	return b.CallLeg2.FillPrice.
		Add(b.PutLeg1.FillPrice).
		Sub(b.CallLeg1.FillPrice).
		Sub(b.PutLeg2.FillPrice)
}

// FillProfit returns the minimum profit of the box after being filled.
func (b *Box) FillProfit() decimal.Decimal {
	return b.Width().Add(b.FillPrice())
}

// MarketPrice returns the net credit (positive) or debit (negative) of the box based on leg mid prices.
func (b *Box) MarketPrice() decimal.Decimal {
	return b.CallLeg1.Option.MarketPrice().
		Sub(b.CallLeg2.Option.MarketPrice()).
		Sub(b.PutLeg1.Option.MarketPrice()).
		Add(b.PutLeg2.Option.MarketPrice())
}

// Filled returns true if all legs of the box are filled.
func (b *Box) Filled() bool {
	return b.CallLeg1.Filled() &&
		b.CallLeg2.Filled() &&
		b.PutLeg1.Filled() &&
		b.PutLeg2.Filled()
}

// Order places orders for all legs of the box in parallel.
func (b *Box) Order(legUpdates chan<- LegUpdate) {
	boxes.Add(b)
	b.CallLeg1.Order(legUpdates)
	b.CallLeg2.Order(legUpdates)
	b.PutLeg1.Order(legUpdates)
	b.PutLeg2.Order(legUpdates)
}

// Check checks invariants.
func (b *Box) Check() {
	b.CallLeg1.Check()
	b.CallLeg2.Check()
	b.PutLeg1.Check()
	b.PutLeg2.Check()
	if b.CallLeg1.Option.Class != databento.InstrumentClassCall {
		panic("CallLeg1 must be a call")
	}
	if b.CallLeg2.Option.Class != databento.InstrumentClassCall {
		panic("CallLeg2 must be a call")
	}
	if b.PutLeg1.Option.Class != databento.InstrumentClassPut {
		panic("PutLeg1 must be a put")
	}
	if b.PutLeg2.Option.Class != databento.InstrumentClassPut {
		panic("PutLeg2 must be a put")
	}
	if b.CallLeg1.Option.Strike.Cmp(b.PutLeg1.Option.Strike) != 0 {
		panic("CallLeg1 and PutLeg1 must have the same strike")
	}
	if b.CallLeg2.Option.Strike.Cmp(b.PutLeg2.Option.Strike) != 0 {
		panic("CallLeg2 and PutLeg2 must have the same strike")
	}
	if b.CallLeg1.Option.Strike.Cmp(b.CallLeg2.Option.Strike) == 0 {
		panic("CallLeg1 and CallLeg2 must have different strikes")
	}
	if !b.CallLeg1.LimitPrice.IsNegative() {
		panic("CallLeg1 must be a buy (negative limit price)")
	}
	if !b.CallLeg2.LimitPrice.IsPositive() {
		panic("CallLeg2 must be a sell (positive limit price)")
	}
	if !b.PutLeg1.LimitPrice.IsPositive() {
		panic("PutLeg1 must be a sell (positive limit price)")
	}
	if !b.PutLeg2.LimitPrice.IsNegative() {
		panic("PutLeg2 must be a buy (negative limit price)")
	}
	limitPrice := b.LimitPrice()
	if b.IsBuying() {
		if limitPrice.IsPositive() {
			panic("Buy box must have negative limit price (net debit)")
		}
		if b.Width().IsNegative() {
			panic("Buy box must have positive width")
		}
	} else {
		if limitPrice.IsNegative() {
			panic("Sell box must have positive limit price (net credit)")
		}
		if b.Width().IsPositive() {
			panic("Sell box must have negative width")
		}
	}
}

func (b *Box) LogTickIfRelevant(option *Option) {
	b.CallLeg1.LogTickIfRelevant(option)
	b.CallLeg2.LogTickIfRelevant(option)
	b.PutLeg1.LogTickIfRelevant(option)
	b.PutLeg2.LogTickIfRelevant(option)
}
