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
	OldIV          decimal.Decimal // the IV of the leg at the time we last updated the limit price
	Greed          decimal.Decimal // the amount of greed applied to the limit price (positive means more greedy, negative means more generous)
	OrderID        schwab.OrderID  // schwab order id or zero if we haven't ordered anything yet
	FillPrice      decimal.Decimal // the fill price of the leg (always positive, zero if not yet filled)
	RouteName      string          // pfof processor name that's handling order
	Canceling      bool            // whether we are currently trying to cancel this leg
	Canceled       bool            // whether this leg has been canceled
	Closing        bool            // whether this leg is a sell (true for sell call or buy put, false for buy call or sell put)
	ClosePrice     decimal.Decimal // the fill price of the leg (always positive, zero if not yet closed)
}

type LegUpdate struct {
	Leg     *Leg
	OrderID schwab.OrderID
}

func (l *Leg) String() string {
	kind := "bear"
	if l.IsBull() {
		kind = "bull"
	}
	return fmt.Sprintf("%s %s %s %s %s @ %s (greed=%s bid=%s ask=%s profit=%s market=%s->%s fair=%s->%s iv=%s->%s δ=%s γ=%s θ=%s ν=%s route=%s)",
		l.Name, l.DescribeState(), l.Instruction(), kind, l.Option, l.LimitPrice, l.Greed, l.Option.Bid, l.Option.Ask, l.Profit(),
		l.OldMarketPrice, l.MarketPrice(), l.OldFairPrice, l.FairPrice(), l.OldIV.Format(3), l.Option.IV.Format(3),
		l.Option.Delta.Format(3), l.Option.Gamma().Format(3), l.Option.Theta().Format(3), l.Option.Vega().Format(3),
		l.RouteName)
}

func (l *Leg) IsBull() bool {
	return (l.Option.Class == databento.InstrumentClassCall && l.IsBuying()) ||
		(l.Option.Class == databento.InstrumentClassPut && !l.IsBuying())
}

func (l *Leg) IsSafe() bool {
	// selling a call is pretty safe if the strike is above the current price
	if l.Option.Class == databento.InstrumentClassCall && !l.IsBuying() && l.Option.Strike.Cmp(gSPXPrice.Add(*safetyFlag)) >= 0 {
		return true
	}
	// selling a put is pretty safe if the strike is below the current price
	if l.Option.Class == databento.InstrumentClassPut && !l.IsBuying() && l.Option.Strike.Cmp(gSPXPrice.Sub(*safetyFlag)) <= 0 {
		return true
	}
	return false
}

func (l *Leg) IsBuying() bool {
	return l.LimitPrice.IsNegative()
}

func (l *Leg) Filled() bool {
	return !l.FillPrice.IsZero() && !l.Closed()
}

func (l *Leg) Closed() bool {
	return !l.ClosePrice.IsZero()
}

func (l *Leg) Complete() bool {
	return l.Canceled || l.Closed() || l.Filled()
}

// EffectivePrice returns the fill price if filled, otherwise the absolute limit price.
func (l *Leg) EffectivePrice() decimal.Decimal {
	if !l.FillPrice.IsZero() {
		return l.FillPrice
	}
	return l.LimitPrice.Abs()
}

func (l *Leg) FairPrice() decimal.Decimal {
	return l.Option.FairPrice()
}

func (l *Leg) MarketPrice() decimal.Decimal {
	return l.Option.MarketPrice()
}

func (l *Leg) ChooseLimitPrice() {
	switch l {
	case l.Box.BuyCall:
		l.LimitPrice = quantizeTruncateSPX(l.Option.Ask.Min(l.Option.FairPrice())).Neg()
	case l.Box.SellCall:
		l.LimitPrice = quantizeAwaySPX(l.Option.Bid.Max(l.Option.FairPrice()))
	case l.Box.SellPut:
		l.LimitPrice = quantizeAwaySPX(l.Option.Bid.Max(l.Option.FairPrice()))
	case l.Box.BuyPut:
		l.LimitPrice = quantizeTruncateSPX(l.Option.Ask.Min(l.Option.FairPrice())).Neg()
	default:
		panic("unknown leg")
	}
}

func (l *Leg) ApplyGreed() {
	l.OldIV = l.Option.IV
	l.OldFairPrice = l.FairPrice()
	l.OldMarketPrice = l.MarketPrice()
	// imbalance > 0 means too many unfilled bulls (long delta exposure)
	// imbalance < 0 means too many unfilled bears (short delta exposure)
	imbalance := gUnfilledBulls.Size() - gUnfilledBears.Size()
	// bull legs: greed when imbalance positive, generous when negative
	// bear legs: greed when imbalance negative, generous when positive
	ticks := imbalance
	if !l.IsBull() {
		ticks = -ticks
	}
	ticks = max(-3, min(3, ticks*2))
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
	if l.Closing {
		if l.IsBuying() {
			return schwab.InstructionBuyToClose
		}
		return schwab.InstructionSellToClose
	}
	if l.IsBuying() {
		return schwab.InstructionBuyToOpen
	}
	return schwab.InstructionSellToOpen
}

func (l *Leg) Order() {
	l.Check()
	if l.IsBuying() {
		gRestrictedToBuying.Add(l.Option.ID)
	} else {
		gRestrictedToSelling.Add(l.Option.ID)
	}
	if l.IsBull() {
		gUnfilledBulls.Add(l)
	} else {
		gUnfilledBears.Add(l)
	}
	gPendingLegs.Add(l)
	go l.doOrder()
}

func (l *Leg) doOrder() {
	orderID, err := gSchwabClient.CreateOrder(&schwab.Order{
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
	gLegUpdates <- LegUpdate{l, orderID}
}

func (l *Leg) Update(limitPrice decimal.Decimal) {
	if limitPrice.IsNegative() != l.LimitPrice.IsNegative() {
		panic("cannot change sign of limit price")
	}
	if l.OrderID == 0 {
		panic("cannot update leg that has not been ordered")
	}
	go l.doUpdate(l.OrderID, limitPrice)
}

func (l *Leg) doUpdate(orderID schwab.OrderID, limitPrice decimal.Decimal) {
	newOrderID, err := gSchwabClient.ReplaceOrder(orderID, &schwab.Order{
		OrderType:         schwab.OrderTypeLimit,
		Price:             limitPrice.Abs(),
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
		log.Fatalf("replace order FAILED %s %s %s %s @ %s: %v",
			l.Name, l.Instruction(), l.Option.Class, l.Option.Strike, l.LimitPrice.Abs(), err)
	}
	log.Printf("replace order SENT %s %s %s %s @ %s (id %d->%d)",
		l.Name, l.Instruction(), l.Option.Class, l.Option.Strike, l.LimitPrice.Abs(), orderID, newOrderID)
	// websocket event handlers will update l.OrderID and l.LimitPrice
}

// Profit returns profit of leg.
// If leg was never filled, this returns zero.
// If leg was filled, then this returns the hypothetical profit if it were to be closed.
// If leg was closed, then this returns the executed profit of the round trip.
func (l *Leg) Profit() decimal.Decimal {
	if l.FillPrice.IsZero() {
		return decimal.Zero
	}
	// determine the closing price (actual or hypothetical)
	closingPrice := l.ClosePrice
	if closingPrice.IsZero() {
		closingPrice = l.getClosingLimitPrice().Abs()
	}
	// determine whether this leg originally bought or sold
	// we can't use IsBuying() because Close() flips the LimitPrice sign
	switch l {
	case l.Box.BuyCall, l.Box.BuyPut:
		// we bought at FillPrice, would sell at closingPrice
		return closingPrice.Sub(l.FillPrice)
	case l.Box.SellCall, l.Box.SellPut:
		// we sold at FillPrice, would buy back at closingPrice
		return l.FillPrice.Sub(closingPrice)
	default:
		panic("unknown leg in Profit()")
	}
}

// Close closes a filled leg by sending the opposite order (e.g. sell to close if we bought to open).
func (l *Leg) Close() {
	if !l.Filled() {
		panic("cannot close leg that isn't filled")
	}
	if l.Closed() {
		panic("leg is already closed")
	}
	if l.Closing {
		panic("leg is already closing")
	}
	l.Closing = true
	l.OrderID = 0
	l.LimitPrice = l.getClosingLimitPrice()
	gPendingLegs.Add(l)
	go l.doOrder()
}

func (l *Leg) getClosingLimitPrice() decimal.Decimal {
	// To close, we do the opposite of the opening trade.
	// Be generous on quantization to fill quickly (sell low, buy high).
	switch l {
	case l.Box.BuyCall, l.Box.BuyPut:
		// we bought to open, so we sell to close
		return quantizeTruncateSPX(l.Option.Bid.Max(l.Option.FairPrice()))
	case l.Box.SellCall, l.Box.SellPut:
		// we sold to open, so we buy to close
		return quantizeAwaySPX(l.Option.Ask.Min(l.Option.FairPrice())).Neg()
	default:
		panic("unknown leg in getClosingLimitPrice()")
	}
}

// Cancel cancels this leg if it's not already filled or canceled.
func (l *Leg) Cancel() {
	if l.Canceling {
		panic("cannot cancel leg that is already canceling")
	}
	if l.Complete() {
		panic("cannot cancel leg that is already complete")
	}
	l.Canceling = true
	if l.OrderID == 0 {
		log.Printf("warning: canceling leg before receiving order id: %s", l)
		return
	}
	go l.doCancel(l.OrderID)
}

func (l *Leg) doCancel(orderID schwab.OrderID) {
	err := gSchwabClient.CancelOrder(orderID)
	if err != nil {
		log.Fatalf("cancel order FAILED %s %s %s %s @ %s: %v",
			l.Name, l.Instruction(), l.Option.Class, l.Option.Strike, l.LimitPrice.Abs(), err)
	}
	log.Printf("cancel order SENT %s %s %s %s @ %s (id %d)",
		l.Name, l.Instruction(), l.Option.Class, l.Option.Strike, l.LimitPrice.Abs(), orderID)
	// websocket event handlers will update object fields
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
		if gRestrictedToSelling.Contains(l.Option.ID) {
			panic("Leg is buying but option is restricted to selling only")
		}
	} else {
		if gRestrictedToBuying.Contains(l.Option.ID) {
			panic("Leg is selling but option is restricted to buying only")
		}
	}
	if l.ClosePrice.IsNegative() {
		panic("Leg close price cannot be negative")
	}
	if l.FillPrice.IsNegative() {
		panic("Leg fill price cannot be negative")
	}
	if l.Closing && !l.Filled() {
		panic("Leg is closing but wasn't filled")
	}
	if l.Closed() && !l.Filled() {
		panic("Leg was closed but wasn't filled")
	}
	if l.Canceling && l.OrderID == 0 {
		panic("Leg is canceling but never had an order ID")
	}
	if l.Canceled && l.OrderID == 0 {
		panic("Leg was canceled but never had an order ID")
	}
	if l.Canceled && l.Closed() {
		panic("Leg cannot be both canceled and closed")
	}
}

func (l *Leg) DescribeState() string {
	if l.ClosePrice.IsPositive() {
		return "closed"
	}
	if l.FillPrice.IsPositive() {
		if l.Canceled {
			return "filled-close-canceled"
		}
		if l.Canceling {
			return "filled-close-canceling"
		}
		if l.Closing {
			return "filled-closing"
		}
		return "filled"
	}
	if l.Canceling {
		return "canceling"
	}
	if l.Canceled {
		return "canceled"
	}
	if l.OrderID != 0 {
		return "ordered"
	}
	return "pending"
}
