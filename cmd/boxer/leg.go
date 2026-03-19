package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/decimal"
	"fmt"
	"log"
)

type Leg struct {
	Box            *Box            // the box this leg belongs to
	Name           string          // arbitrary human friendly name for logs, e.g. "#1", "#2", "#3", "#4"
	Option         *Option         // the option instrument for this leg
	LimitPrice     decimal.Decimal // our limit order price (negative if buying, e.g. -0.15 means we get a $15 debit or in otherwords are paying $15 for the leg)
	OldMarketPrice decimal.Decimal // the market price of the leg at the time we last updated the limit price
	OldFairPrice   decimal.Decimal // the fair price of the leg at the time we last updated the limit price
	Greed          decimal.Decimal // the amount of greed applied to the limit price (positive means more greedy, negative means more generous)
	OrderID        int64           // schwab order ID, or 0 if not yet placed, or canceled
	FillPrice      decimal.Decimal // the fill price of the leg (always positive, zero if not yet filled)
	RouteName      string          // pfof processor name that's handling order
}

type LegUpdate struct {
	Leg     *Leg
	OrderID int64
}

func (l *Leg) String() string {
	kind := "bear"
	if l.IsBull() {
		kind = "bull"
	}
	return fmt.Sprintf("%s %s %s %s @ %s (greed=%s bid=%s ask=%s market=%s->%s fair=%s->%s iv=%.3f δ=%.3f γ=%s θ=%s ν=%s)",
		l.Name, l.Instruction(), kind, l.Option, l.LimitPrice, l.Greed, l.Option.Bid, l.Option.Ask,
		l.OldMarketPrice, l.MarketPrice(), l.OldFairPrice, l.FairPrice(), l.Option.IV, l.Option.Delta,
		l.Option.Gamma().Format(3), l.Option.Theta().Format(3), l.Option.Vega().Format(3))
}

func (l *Leg) IsBull() bool {
	return (l.Option.Class == databento.InstrumentClassCall && l.IsBuying()) ||
		(l.Option.Class == databento.InstrumentClassPut && !l.IsBuying())
}

func (l *Leg) IsSafe() bool {
	// selling a call is pretty safe if the strike is above the current price
	if l.Option.Class == databento.InstrumentClassCall && !l.IsBuying() && l.Option.Strike.Cmp(es.Price.Add(*safetyFlag)) >= 0 {
		return true
	}
	// selling a put is pretty safe if the strike is below the current price
	if l.Option.Class == databento.InstrumentClassPut && !l.IsBuying() && l.Option.Strike.Cmp(es.Price.Sub(*safetyFlag)) <= 0 {
		return true
	}
	return false
}

func (l *Leg) IsBuying() bool {
	return l.LimitPrice.IsNegative()
}

func (l *Leg) Filled() bool {
	return !l.FillPrice.IsZero()
}

func (l *Leg) FairPrice() decimal.Decimal {
	return l.Option.FairPrice()
}

func (l *Leg) MarketPrice() decimal.Decimal {
	return l.Option.MarketPrice()
}

func (l *Leg) ApplyGreed() {
	l.OldFairPrice = l.FairPrice()
	l.OldMarketPrice = l.MarketPrice()
	// imbalance > 0 means too many unfilled bulls (long delta exposure)
	// imbalance < 0 means too many unfilled bears (short delta exposure)
	imbalance := unfilledBulls.Size() - unfilledBears.Size()
	// bull legs: greed when imbalance positive, generous when negative
	// bear legs: greed when imbalance negative, generous when positive
	ticks := imbalance
	if !l.IsBull() {
		ticks = -ticks
	}
	ticks = max(-3, min(3, ticks))
	if ticks > 0 && !l.IsSafe() {
		// this leg worsens our exposure; demand a better price
		// skip greed on safe legs (far OTM) since they carry little risk
		l.LimitPrice, l.Greed = applyGreed(l.LimitPrice, ticks)
	} else if ticks < 0 {
		// this leg reduces our exposure; accept a worse price to fill faster
		l.LimitPrice, l.Greed = applyGenerosity(l.LimitPrice, -ticks)
	}
}

func (l *Leg) Instruction() schwab.Instruction {
	if l.IsBuying() {
		return schwab.InstructionBuyToOpen
	}
	return schwab.InstructionSellToOpen
}

func (l *Leg) Order(legUpdates chan<- LegUpdate) {
	l.Check()
	if l.IsBuying() {
		restrictedToBuying.Add(l.Option.ID)
	} else {
		restrictedToSelling.Add(l.Option.ID)
	}
	if l.IsBull() {
		unfilledBulls.Add(l)
	} else {
		unfilledBears.Add(l)
	}
	go l.doOrder(legUpdates)
}

func (l *Leg) doOrder(legUpdates chan<- LegUpdate) {
	orderID, err := schwabClient.CreateOrder(&schwab.Order{
		OrderType:         schwab.OrderTypeLimit,
		Price:             l.LimitPrice.Abs(),
		Duration:          schwab.DurationDay,
		Session:           schwab.SessionNormal,
		OrderStrategyType: schwab.OrderStrategyTypeSingle,
		OrderLegCollection: []schwab.OrderLeg{{
			Instruction: l.Instruction(),
			Quantity:    decimal.One,
			Instrument: schwab.Instrument{
				Symbol: l.Option.OSI(),
				Type:   schwab.AssetTypeOption,
			},
		}},
	})
	if err != nil {
		log.Fatalf("order FAILED %s %s %s %s @ %s: %v",
			l.Name, l.Instruction(), l.Option.Class, l.Option.Strike, l.LimitPrice.Abs(), err)
	}
	log.Printf("order SENT %s %s %s %s @ %s (id=%d)",
		l.Name, l.Instruction(), l.Option.Class, l.Option.Strike, l.LimitPrice.Abs(), orderID)
	legUpdates <- LegUpdate{Leg: l, OrderID: orderID}
}

// Check checks invariants.
func (l *Leg) Check() {
	if l.Option == nil {
		panic("Leg must have an option")
	}
	if l.LimitPrice.IsZero() {
		panic("Leg must have a nonzero limit price")
	}
	if l.LimitPrice.Cmp(quantizeTruncateSPX(l.LimitPrice)) != 0 {
		panic("Leg limit price must be quantized to SPX tick size")
	}
	if l.IsBuying() {
		if restrictedToSelling.Contains(l.Option.ID) {
			panic("Leg is buying but option is restricted to selling only")
		}
	} else {
		if restrictedToBuying.Contains(l.Option.ID) {
			panic("Leg is selling but option is restricted to buying only")
		}
	}
}

func (l *Leg) UpdateOrderID(newID int64) {
	oldID := l.OrderID
	l.OrderID = newID
	log.Printf("leg %s order ID updated in thinkorswim %d -> %d", l.Name, oldID, newID)
}
