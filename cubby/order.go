package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/cboe"
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
	MarginHeld     decimal.Decimal // margin reserved for this order
	OrderedAt      clocky.Time     // local creation time
	TimeInForce    alpaca.TimeInForce
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

	// check order time
	year, month, day := now.Date()
	openTime := cboe.GetOpenTime(year, month, day)
	closeTime := cboe.GetCloseTime(year, month, day)
	if now.Before(openTime) {
		return
	}
	if o.TimeInForce == alpaca.TimeInForceOPG {
		// OPG orders only fill at market open
		if now != openTime {
			return
		}
	} else if o.TimeInForce == alpaca.TimeInForceCLS {
		// CLS orders only fill at market close
		if now != closeTime {
			return
		}
	} else if !now.Before(closeTime) || now.Sub(o.OrderedAt) > *FlagPatience {
		o.setStatus(alpaca.OrderStatusExpired)
		return
	}

	// check if bar crosses limit price
	var mayPass bool
	var barPrice decimal.Decimal
	if o.TimeInForce == alpaca.TimeInForceOPG || o.TimeInForce == alpaca.TimeInForceCLS {
		barPrice = bar.Open
	} else {
		barPrice = bar.VWAP
	}
	if o.Side == ds.SideBuy {
		mayPass = barPrice.Cmp(o.LimitPrice) <= 0
	} else {
		mayPass = barPrice.Cmp(o.LimitPrice) >= 0
	}
	if !mayPass {
		if o.TimeInForce == alpaca.TimeInForceOPG || o.TimeInForce == alpaca.TimeInForceCLS {
			o.setStatus(alpaca.OrderStatusCanceled)
		}
		return
	}

	// calculate how many shares we can take
	var availableQuantity decimal.Decimal
	if o.TimeInForce == alpaca.TimeInForceOPG || o.TimeInForce == alpaca.TimeInForceCLS {
		availableQuantity = o.Quantity
	} else {
		availableQuantity = bar.Volume.Mul(*FlagVWAP).Truncate()
		if !availableQuantity.IsPositive() {
			return
		}
	}

	// check if fill would cause margin call (prices may have moved since order placed)
	if o.Side == ds.SideBuy {
		unfilledQuantity := o.Quantity.Sub(o.FilledQuantity)
		fillQuantity := unfilledQuantity.Min(availableQuantity)
		newQty := o.Equity.Quantity.Add(fillQuantity)
		newMaintMargin := o.Equity.Asset.GetMaintenanceMargin(newQty, barPrice)
		oldMaintMargin := o.Equity.Asset.GetMaintenanceMargin(o.Equity.Quantity, barPrice)
		maintMarginDelta := newMaintMargin.Sub(oldMaintMargin)
		totalMaintMargin := GetMarginUsed().Add(maintMarginDelta)
		portfolioValue := GetPortfolioValue()
		if totalMaintMargin.Cmp(portfolioValue) > 0 {
			// cancel order - filling it would cause margin call
			o.setStatus(alpaca.OrderStatusCanceled)
			return
		}
	}

	// calculate remaining quantity to fill
	unfilledQuantity := o.Quantity.Sub(o.FilledQuantity)
	fillQuantity := unfilledQuantity.Min(availableQuantity)
	if !fillQuantity.IsPositive() {
		panic("broken program logic")
	}

	// calculate weighted average fill price if we already have partial fills
	// OPG orders fill at the opening auction price (bar.Open), not VWAP
	fillPrice := bar.VWAP
	if o.TimeInForce == alpaca.TimeInForceOPG || o.TimeInForce == alpaca.TimeInForceCLS {
		fillPrice = bar.Open
	}
	oldValue := o.FilledPrice.Mul(o.FilledQuantity)
	newValue := fillPrice.Mul(fillQuantity)
	o.FilledQuantity = o.FilledQuantity.Add(fillQuantity)
	o.FilledPrice = oldValue.Add(newValue).Div(o.FilledQuantity)

	// calculate and add fees (limit orders are maker orders)
	fee := gFeeCalculator.GetFee(clocky.Now(), fillQuantity, false)
	o.TotalFees = o.TotalFees.Add(fee)

	// update cash and position
	notional := fillPrice.Mul(fillQuantity)
	if o.Side == ds.SideBuy {
		Cash = Cash.Sub(notional).Sub(fee)
		// update average entry price (weighted average)
		oldValue := o.Equity.EntryPrice.Mul(o.Equity.Quantity)
		newValue := fillPrice.Mul(fillQuantity)
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

	// assert margin constraints after fill
	assertMarginConstraints(o)
}

// assertMarginConstraints verifies we haven't exceeded margin limits after a fill.
// This catches bugs in the margin checking logic.
func assertMarginConstraints(o *Order) {
	marginUsed := GetMarginUsed()
	marginHold := gMarginHold
	totalMargin := marginUsed.Add(marginHold)
	portfolioValue := GetPortfolioValue()

	// Check 1: Total margin (used + held) should not exceed DTBP
	// This would mean we placed orders we couldn't afford
	if totalMargin.Cmp(gDayTradingBuyingPower) > 0 {
		log.Printf("MARGIN VIOLATION: total margin $%s (used $%s + hold $%s) exceeds DTBP $%s after %s %s %s",
			totalMargin.FormatThousand(2),
			marginUsed.FormatThousand(2),
			marginHold.FormatThousand(2),
			gDayTradingBuyingPower.FormatThousand(2),
			o.Side, o.FilledQuantity, o.Equity.Symbol)
	}

	// Check 2: Maintenance margin should not exceed portfolio value (margin call)
	// This shouldn't happen intraday if we're checking initial margin on entry
	if marginUsed.Cmp(portfolioValue) > 0 {
		log.Printf("MARGIN CALL: maintenance margin $%s exceeds portfolio value $%s after %s %s %s",
			marginUsed.FormatThousand(2),
			portfolioValue.FormatThousand(2),
			o.Side, o.FilledQuantity, o.Equity.Symbol)
	}
}

func (o *Order) setStatus(newStatus alpaca.OrderStatus) {
	oldStatus := o.Status
	o.Status = newStatus
	if !oldStatus.IsFinal() && newStatus.IsFinal() {
		delete(Orders, o.OrderID)
		delete(o.Equity.Orders, o.OrderID)
		gMarginHold = gMarginHold.Sub(o.MarginHeld)
		if Verbose || o.FilledQuantity.IsPositive() {
			log.Printf("%s %s %s %s out of %s %s at %s notional %s fee %s id %s", o.Status, o.TimeInForce, o.Side, o.FilledQuantity, o.Quantity, o.Equity.Symbol, o.FilledPrice, o.FilledPrice.Mul(o.FilledQuantity), o.TotalFees, o.OrderID)
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
