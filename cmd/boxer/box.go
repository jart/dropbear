package main

import (
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"fmt"
)

type Box struct {
	BuyCall  *Leg
	SellCall *Leg
	SellPut  *Leg
	BuyPut   *Leg
	Closing  bool
	Created  clocky.Time
}

func (b *Box) String() string {
	direction := "sell"
	if b.IsBuying() {
		direction = "buy"
	}
	return fmt.Sprintf("%s %sw box @ %s (market=%s profit=%s->%s close=%s spx=%s es=%s)\n\t%s\n\t%s\n\t%s\n\t%s",
		direction, b.Width(), b.LimitPrice(), b.MarketPrice(), b.LimitProfit(), b.FillProfit(), b.ClosingProfit(),
		gSPXPrice, gES.Price, b.BuyCall, b.SellCall, b.SellPut, b.BuyPut)
}

// Width returns the distance between the strikes of the box.
// This will be negative for a sell (credit) box and positive for a buy (debit) box.
func (b *Box) Width() decimal.Decimal {
	return b.SellCall.Option.Strike.Sub(b.BuyCall.Option.Strike)
}

// IsBuying returns true if this is a buy box (net debit), false if it's a sell box (net credit).
func (b *Box) IsBuying() bool {
	return b.BuyCall.Option.Strike.Cmp(b.SellCall.Option.Strike) < 0
}

func (b *Box) ChooseLimitPrices() {
	b.BuyCall.ChooseLimitPrice()
	b.SellCall.ChooseLimitPrice()
	b.SellPut.ChooseLimitPrice()
	b.BuyPut.ChooseLimitPrice()
}

func (b *Box) ApplyGreed() {
	b.BuyCall.ApplyGreed()
	b.SellCall.ApplyGreed()
	b.SellPut.ApplyGreed()
	b.BuyPut.ApplyGreed()
}

// LimitPrice returns the net credit (positive) or debit (negative) of the box based on leg limit prices.
func (b *Box) LimitPrice() decimal.Decimal {
	return b.BuyCall.LimitPrice.
		Add(b.SellCall.LimitPrice).
		Add(b.SellPut.LimitPrice).
		Add(b.BuyPut.LimitPrice)
}

// LimitProfit returns the minimum profit of the box based on leg limit prices.
func (b *Box) LimitProfit() decimal.Decimal {
	return b.Width().Add(b.LimitPrice())
}

// FillPrice returns the net credit (positive) or debit (negative) of the box based on leg fill prices.
// If a leg isn't filled yet, then we fall back to the limit price for that leg.
func (b *Box) FillPrice() decimal.Decimal {
	return b.SellCall.EffectivePrice().
		Add(b.SellPut.EffectivePrice()).
		Sub(b.BuyCall.EffectivePrice()).
		Sub(b.BuyPut.EffectivePrice())
}

// FillProfit returns the minimum profit of the box after being filled.
func (b *Box) FillProfit() decimal.Decimal {
	return b.Width().Add(b.FillPrice())
}

// MarketPrice returns the net credit (positive) or debit (negative) of the box based on leg mid prices.
func (b *Box) MarketPrice() decimal.Decimal {
	return b.BuyCall.MarketPrice().
		Sub(b.SellCall.MarketPrice()).
		Sub(b.SellPut.MarketPrice()).
		Add(b.BuyPut.MarketPrice())
}

// FairPrice returns the net credit (positive) or debit (negative) of the box based on leg fair prices.
func (b *Box) FairPrice() decimal.Decimal {
	return b.BuyCall.FairPrice().
		Sub(b.SellCall.FairPrice()).
		Sub(b.SellPut.FairPrice()).
		Add(b.BuyPut.FairPrice())
}

// Filled returns true if all legs of the box are filled.
func (b *Box) Filled() bool {
	return b.BuyCall.Filled() &&
		b.SellCall.Filled() &&
		b.SellPut.Filled() &&
		b.BuyPut.Filled()
}

// Complete returns true if box is in terminal state.
func (b *Box) Complete() bool {
	if b.Filled() {
		return true
	}
	if b.PartiallyFilled() {
		return false
	}
	return b.BuyCall.Complete() && b.SellCall.Complete() && b.SellPut.Complete() && b.BuyPut.Complete()
}

// PartiallyFilled returns true if some but not all legs are filled.
func (b *Box) PartiallyFilled() bool {
	f1 := b.BuyCall.Filled()
	f2 := b.SellCall.Filled()
	f3 := b.SellPut.Filled()
	f4 := b.BuyPut.Filled()
	return (f1 || f2 || f3 || f4) && !(f1 && f2 && f3 && f4)
}

// Order places orders for all legs of the box in parallel.
func (b *Box) Order() {
	gPendingBoxes.Add(b)
	b.BuyCall.Order()
	b.SellCall.Order()
	b.SellPut.Order()
	b.BuyPut.Order()
}

// ClosingProfit returns the hypothetical profit if the box were to be closed at the market price.
func (b *Box) ClosingProfit() decimal.Decimal {
	return b.BuyCall.Profit().
		Add(b.SellCall.Profit()).
		Add(b.SellPut.Profit()).
		Add(b.BuyPut.Profit())
}

// Close aborts a box by closing completed legs and canceling the others.
func (b *Box) Close() {
	if b.Closing {
		panic("cannot close box that is already closing")
	}
	if b.BuyCall.Filled() {
		b.BuyCall.Close()
	} else {
		b.BuyCall.Cancel()
	}
	if b.SellCall.Filled() {
		b.SellCall.Close()
	} else {
		b.SellCall.Cancel()
	}
	if b.SellPut.Filled() {
		b.SellPut.Close()
	} else {
		b.SellPut.Cancel()
	}
	if b.BuyPut.Filled() {
		b.BuyPut.Close()
	} else {
		b.BuyPut.Cancel()
	}
	b.Closing = true
}

// Check checks invariants.
func (b *Box) Check() {
	b.BuyCall.Check()
	b.SellCall.Check()
	b.SellPut.Check()
	b.BuyPut.Check()
	if b.BuyCall.Option.Class != databento.InstrumentClassCall {
		panic("BuyCall must be a call")
	}
	if b.SellCall.Option.Class != databento.InstrumentClassCall {
		panic("SellCall must be a call")
	}
	if b.SellPut.Option.Class != databento.InstrumentClassPut {
		panic("SellPut must be a put")
	}
	if b.BuyPut.Option.Class != databento.InstrumentClassPut {
		panic("BuyPut must be a put")
	}
	if b.BuyCall.Option.Strike.Cmp(b.SellPut.Option.Strike) != 0 {
		panic("BuyCall and SellPut must have the same strike")
	}
	if b.SellCall.Option.Strike.Cmp(b.BuyPut.Option.Strike) != 0 {
		panic("SellCall and BuyPut must have the same strike")
	}
	if b.BuyCall.Option.Strike.Cmp(b.SellCall.Option.Strike) == 0 {
		panic("BuyCall and SellCall must have different strikes")
	}
	if !b.BuyCall.LimitPrice.IsNegative() {
		panic("BuyCall must be a buy (negative limit price)")
	}
	if !b.SellCall.LimitPrice.IsPositive() {
		panic("SellCall must be a sell (positive limit price)")
	}
	if !b.SellPut.LimitPrice.IsPositive() {
		panic("SellPut must be a sell (positive limit price)")
	}
	if !b.BuyPut.LimitPrice.IsNegative() {
		panic("BuyPut must be a buy (negative limit price)")
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
