package main

import (
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/options"
	"errors"
	"fmt"
)

type Order struct {
	Trader    *Trader
	ID        int
	OrderID   schwab.OrderID
	Legs      []*Leg
	Strategy  string
	Price     decimal.Decimal
	Risk      decimal.Decimal
	Payoff    decimal.Decimal
	Score     decimal.Decimal
	Created   clocky.Time
	Sent      bool
	Canceling bool
}

func (order *Order) String() string {
	return fmt.Sprintf("#%d %s", order.ID, order.Strategy)
}

// MidPrice computes the price of the order using mid prices.
func (order *Order) MidPrice() decimal.Decimal {
	var price decimal.Decimal
	for _, leg := range order.Legs {
		if leg.Quantity.IsPositive() {
			price = price.Sub(leg.Option.MidPrice()) // buying costs money
		} else {
			price = price.Add(leg.Option.MidPrice()) // selling receives money
		}
	}
	return price
}

// NaturalPrice computes the price to cross the spread.
func (order *Order) NaturalPrice() decimal.Decimal {
	var price decimal.Decimal
	for _, leg := range order.Legs {
		if leg.Quantity.IsPositive() {
			price = price.Sub(leg.Option.Ask) // buying costs money
		} else {
			price = price.Add(leg.Option.Bid) // selling receives money
		}
	}
	return price
}

// MakerPrice computes the price to place a maker order.
func (order *Order) MakerPrice() decimal.Decimal {
	var price decimal.Decimal
	for _, leg := range order.Legs {
		if leg.Quantity.IsPositive() {
			price = price.Sub(leg.Option.Bid) // buying costs money
		} else {
			price = price.Add(leg.Option.Ask) // selling receives money
		}
	}
	return price
}

// Width returns the distance between the lowest and highest strikes.
// Returns zero for single-leg orders.
func (order *Order) Width() decimal.Decimal {
	if len(order.Legs) < 2 {
		return decimal.Zero
	}
	lo := order.Legs[0].Option.Strike.Price
	hi := lo
	for _, leg := range order.Legs[1:] {
		sp := leg.Option.Strike.Price
		lo = lo.Min(sp)
		hi = hi.Max(sp)
	}
	return hi.Sub(lo)
}

// Send submits order to market.
func (order *Order) Send() error {
	if order.Sent {
		return errors.New("order already sent")
	}
	if order.Price.IsZero() {
		return errors.New("cannot send order with zero price")
	}
	if len(order.Legs) == 0 {
		return errors.New("cannot send order with no legs")
	}
	if order.Vertical() {
		width := order.Width()
		if order.Price.Abs().Cmp(width) >= 0 {
			return fmt.Errorf("vertical order price must be beneath spread width: price=%s width=%s mid=%s natural=%s maker=%s",
				order.Price, width, order.MidPrice(), order.NaturalPrice(), order.MakerPrice())
		}
	}
	for _, leg := range order.Legs {
		if leg.Quantity.IsZero() {
			return errors.New("leg has zero quantity")
		}
	}
	seen := map[*options.Option]bool{}
	for _, leg := range order.Legs {
		if seen[leg.Option] {
			return errors.New("duplicate option in order")
		}
		seen[leg.Option] = true
	}
	underlyingSymbol := order.Legs[0].Option.Symbol
	for _, leg := range order.Legs {
		if underlyingSymbol != leg.Option.Symbol {
			return errors.New("all legs must have same underlying")
		}
	}
	tick, bigTick := getTicks(underlyingSymbol)
	if len(order.Legs) == 1 && order.Price.Abs().Cmp(kThree) >= 0 {
		tick = bigTick // spreads always quantize on minimum tick size
	}
	if order.Price.Cmp(order.Price.QuantizeTruncate(tick)) != 0 {
		return errors.New("order price must be quantized properly")
	}

	// Decrement virtual liquidity so we don't send multiple orders for same volume.
	// We do this even in dry run mode to ensure simulation accuracy.
	for _, leg := range order.Legs {
		qty := uint32(leg.Quantity.Abs().Int64())
		if leg.Quantity.IsPositive() {
			if leg.Option.AskSize > qty {
				leg.Option.AskSize -= qty
			} else {
				leg.Option.AskSize = 0
			}
		} else {
			if leg.Option.BidSize > qty {
				leg.Option.BidSize -= qty
			} else {
				leg.Option.BidSize = 0
			}
		}
	}

	if *dryFlag {
		return errors.New("won't send order in dry run mode")
	}
	order.Sent = true
	if *liveFlag {
		order.Trader.sendLiveOrder(order)
	} else {
		order.Trader.simulateOrder(order)
	}
	return nil
}

// Cancel attempts to withdraw sent order from market.
func (order *Order) Cancel() error {
	if !order.Sent {
		return errors.New("order not sent")
	}
	if order.HasFill() {
		return errors.New("order already filled")
	}
	if order.Canceling {
		return errors.New("order already canceling")
	}
	if *liveFlag && order.OrderID == 0 {
		return errors.New("cannot cancel order that was never acknowledged by broker")
	}
	if *dryFlag {
		panic("not possible")
	}
	order.Canceling = true
	if *liveFlag {
		go order.Trader.schwabCancelOrder(order)
	} else {
		order.Trader.simulateCancelOrder(order)
	}
	return nil
}

func (order *Order) Filled() bool {
	for _, leg := range order.Legs {
		if !leg.Filled {
			return false
		}
	}
	return true
}

func (order *Order) HasFill() bool {
	for _, leg := range order.Legs {
		if leg.Filled {
			return true
		}
	}
	return false
}

func (order *Order) Vertical() bool {
	return len(order.Legs) == 2 && order.Legs[0].Option.Class == order.Legs[1].Option.Class
}

func compareOrdersByScore(a, b *Order) int {
	res := a.Score.Cmp(b.Score)
	if res != 0 {
		return -res // highest score first
	}
	return a.ID - b.ID
}
