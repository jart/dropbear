package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"fmt"
	"log"

	"github.com/google/uuid"
)

var (
	Orders      = make(map[string]*Order)
	ordersByCID = make(map[string]*Order)
)

type Order struct {
	OrderID        string
	ClientOrderID  string
	Equity         *Equity
	Type           alpaca.OrderType
	Status         alpaca.OrderStatus
	Side           ds.Side
	Quantity       decimal.Decimal // always positive
	LimitPrice     decimal.Decimal // always positive
	FilledPrice    decimal.Decimal // average fill price, or zero if unfilled
	FilledQuantity decimal.Decimal // positive, or zero if unfilled
	TotalFees      decimal.Decimal // positive, or zero if unfilled
	OrderedAt      clocky.Time     // local creation time
}

func (o *Order) Cancel() error {
	if o.Status.IsFinal() {
		return nil
	}
	if Paper {
		o.setStatus(alpaca.OrderStatusCanceled)
	} else {
		err := Client.CancelOrder(o.OrderID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *Order) simulateFill(now clocky.Time, bar *ds.Bar) {
	if !Paper {
		panic("broken program logic")
	}
	if o.Status.IsFinal() {
		panic("broken program logic")
	}

	// check if order is too old
	if now.Sub(o.OrderedAt) > *FlagPatience {
		o.setStatus(alpaca.OrderStatusExpired)
		return
	}
	time := now.ClockInt()
	if time < 6_30_00 || time > 13_00_00 {
		o.setStatus(alpaca.OrderStatusExpired)
		return
	}

	// check if volume weighted average price crosses limit price
	if o.Side == ds.SideBuy {
		if bar.VWAP.Cmp(o.LimitPrice) > 0 {
			return
		}
	} else {
		if bar.VWAP.Cmp(o.LimitPrice) < 0 {
			return
		}
	}

	// calculate how many shares we can take
	availableQuantity := bar.Volume.Mul(*FlagVWAP).Truncate()
	if !availableQuantity.IsPositive() {
		return
	}

	// calculate remaining quantity to fill
	unfilledQuantity := o.Quantity.Sub(o.FilledQuantity)
	fillQuantity := unfilledQuantity.Min(availableQuantity)
	if !fillQuantity.IsPositive() {
		panic("broken program logic")
	}

	// calculate weighted average fill price if we already have partial fills
	oldValue := o.FilledPrice.Mul(o.FilledQuantity)
	newValue := bar.VWAP.Mul(fillQuantity)
	o.FilledQuantity = o.FilledQuantity.Add(fillQuantity)
	o.FilledPrice = oldValue.Add(newValue).Div(o.FilledQuantity)

	// calculate and add fees (limit orders are maker orders)
	fee := gFeeCalculator.GetFee(clocky.Now(), fillQuantity, false)
	o.TotalFees = o.TotalFees.Add(fee)

	// update cash and position
	notional := bar.VWAP.Mul(fillQuantity)
	if o.Side == ds.SideBuy {
		Cash = Cash.Sub(notional).Sub(fee)
		// update average entry price (weighted average)
		oldValue := o.Equity.EntryPrice.Mul(o.Equity.Quantity)
		newValue := bar.VWAP.Mul(fillQuantity)
		o.Equity.Quantity = o.Equity.Quantity.Add(fillQuantity)
		o.Equity.EntryPrice = oldValue.Add(newValue).Div(o.Equity.Quantity)
	} else {
		Cash = Cash.Add(notional).Sub(fee)
		o.Equity.Quantity = o.Equity.Quantity.Sub(fillQuantity)
		// reset entry price when position is fully closed
		if o.Equity.Quantity.IsZero() {
			o.Equity.EntryPrice = decimal.Zero
		}
	}

	// update order status
	if o.FilledQuantity.Cmp(o.Quantity) == 0 {
		o.setStatus(alpaca.OrderStatusFilled)
	} else {
		o.setStatus(alpaca.OrderStatusPartiallyFilled)
	}
}

func (o *Order) setStatus(newStatus alpaca.OrderStatus) {
	oldStatus := o.Status
	o.Status = newStatus
	if !oldStatus.IsFinal() && newStatus.IsFinal() {
		delete(Orders, o.OrderID)
		delete(o.Equity.Orders, o.OrderID)
		if Verbose || o.FilledQuantity.IsPositive() {
			log.Printf("%s %s %s out of %s %s at %s notional %s fee %s id %s", o.Status, o.Side, o.FilledQuantity, o.Quantity, o.Equity.Symbol, o.FilledPrice, o.FilledPrice.Mul(o.FilledQuantity), o.TotalFees, o.OrderID)
		}
	}
}

func (o *Order) sync(alpacaOrder *alpaca.Order) {
	if alpacaOrder.FilledQty.Cmp(o.FilledQuantity) > 0 {
		o.FilledQuantity = alpacaOrder.FilledQty
		o.FilledPrice = alpacaOrder.FilledAvgPrice
	}
	o.setStatus(alpacaOrder.Status)
}

func (o *Order) logPlacedOrder() {
	if Verbose {
		log.Printf("placed order to %s %s shares of %s at limit $%s notional $%s id %s",
			o.Side, o.Quantity, o.Equity.Symbol, o.LimitPrice, o.Quantity.Mul(o.LimitPrice), o.OrderID)
	}
}

var simulatedOrderCount int

func generateOrderID() string {
	if Live {
		return uuid.New().String()
	}
	simulatedOrderCount++
	return fmt.Sprintf("%d", simulatedOrderCount)
}
