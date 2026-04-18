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
	Created   clocky.Time
	OrderID   schwab.OrderID
	Legs      []*Leg
	Price     decimal.Decimal
	Sent      bool
	Canceling bool
	Making    bool
}

func (order *Order) String() string {
	return fmt.Sprintf("#%d", order.ID)
}

// MidPrice computes the price of the order using mid prices.
func (order *Order) MidPrice() decimal.Decimal {
	var price decimal.Decimal
	for _, leg := range order.Legs {
		if leg.Quantity.IsPositive() {
			price = price.Sub(leg.Security.MidPrice()) // buying costs money
		} else {
			price = price.Add(leg.Security.MidPrice()) // selling receives money
		}
	}
	return price
}

// NaturalPrice computes the price to cross the spread.
func (order *Order) NaturalPrice() decimal.Decimal {
	var price decimal.Decimal
	for _, leg := range order.Legs {
		if leg.Quantity.IsPositive() {
			price = price.Sub(leg.Security.GetAsk()) // buying costs money
		} else {
			price = price.Add(leg.Security.GetBid()) // selling receives money
		}
	}
	return price
}

// MakerPrice computes the price to place a maker order.
func (order *Order) MakerPrice() decimal.Decimal {
	var price decimal.Decimal
	for _, leg := range order.Legs {
		if leg.Quantity.IsPositive() {
			price = price.Sub(leg.Security.GetBid()) // buying costs money
		} else {
			price = price.Add(leg.Security.GetAsk()) // selling receives money
		}
	}
	return price
}

func (order *Order) Options() []*options.Option {
	var opts []*options.Option
	for _, leg := range order.Legs {
		if opt, ok := leg.Security.(*options.Option); ok {
			opts = append(opts, opt)
		}
	}
	return opts
}

// Width returns the distance between the lowest and highest strikes.
// Returns zero for single-leg orders.
func (order *Order) Width() decimal.Decimal {
	opts := order.Options()
	if len(opts) < 2 {
		return decimal.Zero
	}
	lo := opts[0].Strike.Price
	hi := lo
	for _, leg := range opts[1:] {
		sp := leg.Strike.Price
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
	if len(order.Legs) == 0 {
		return errors.New("cannot send order with no legs")
	}
	if order.Vertical() && !order.Price.IsZero() {
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
	seen := map[options.Security]bool{}
	for _, leg := range order.Legs {
		if seen[leg.Security] {
			return errors.New("duplicate security in order")
		}
		seen[leg.Security] = true
	}
	underlyingSymbol := order.Legs[0].Security.GetSymbol()
	for _, leg := range order.Legs {
		if underlyingSymbol != leg.Security.GetSymbol() {
			return errors.New("all legs must have same underlying")
		}
	}
	if order.Price.Cmp(order.Price.QuantizeTruncate(order.priceTick())) != 0 {
		return errors.New("order price must be quantized properly")
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
	opts := order.Options()
	return len(opts) == 2 && opts[0].Class == opts[1].Class
}

func (order *Order) EstimateFee(marketable bool) decimal.Decimal {
	fee := decimal.Zero
	if order.hasEquityLeg() {
		fee = fee.Add(kCatFeePerTrade)
		fee = fee.Add(kBrokerFeePerTrade)
	}
	for _, leg := range order.Legs {
		fee = fee.Add(leg.EstimateFee(marketable))
	}
	return fee
}

func (order *Order) hasEquityLeg() bool {
	for _, leg := range order.Legs {
		if _, ok := leg.Security.(*options.Equity); ok {
			return true
		}
	}
	return false
}

func (order *Order) Ticks() (decimal.Decimal, decimal.Decimal) {
	tick, bigTick := decimal.Max, decimal.Max
	for _, leg := range order.Legs {
		t, bigT := leg.Security.Ticks()
		tick = tick.Min(t)
		bigTick = bigTick.Min(bigT)
	}
	if len(order.Legs) > 1 {
		bigTick = tick // spreads always quantize on minimum tick size
	}
	return tick, bigTick
}

func (order *Order) priceTick() decimal.Decimal {
	tick, bigTick := order.Ticks()
	if order.Price.Abs().Cmp(decimal.Three) >= 0 {
		return bigTick
	}
	return tick
}
