package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"log"
)

var (
	Orders      = make(map[string]*Order)
	ordersByCID = make(map[string]*Order)
)

type Order struct {
	OrderID        string
	ClientOrderID  string
	Equity         *Equity
	Side           ds.Side
	Status         alpaca.OrderStatus
	Quantity       decimal.Decimal
	LimitPrice     decimal.Decimal
	FilledPrice    decimal.Decimal
	FilledQuantity decimal.Decimal
	TotalFees      decimal.Decimal
}

func (o *Order) Cancel() error {
	if o.Status.IsFinal() {
		return nil
	}
	if !Paper || *FlagVerbose {
		log.Printf("%s cancelling order %s %s %s @ $%s", o.OrderID, o.Side, o.Equity.Symbol, o.Quantity.Format(0), o.LimitPrice.Format(2))
	}
	if Paper {
		o.Status = alpaca.OrderStatusCanceled
		delete(o.Equity.Orders, o.OrderID)
		delete(Orders, o.OrderID)
		o.log()
	} else {
		err := Client.CancelOrder(o.OrderID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *Order) simulateFill(bar *ds.Bar) {
	if o.Status.IsFinal() {
		panic("can't fill a filled order")
	}

	// check if VWAP crosses limit price
	// for BUY: we fill if bar.VWAP <= LimitPrice (price is low enough to buy)
	// for SELL: we fill if bar.VWAP >= LimitPrice (price is high enough to sell)
	if o.Side == ds.SideBuy {
		if bar.VWAP.Cmp(o.LimitPrice) > 0 {
			return
		}
	} else {
		if bar.VWAP.Cmp(o.LimitPrice) < 0 {
			return
		}
	}

	// calculate how many shares we can take (flagVWAP % of bar volume)
	maxShares := bar.Volume.Mul(*flagVWAP).Truncate()
	if maxShares.IsZero() {
		return
	}

	// calculate remaining quantity to fill
	remaining := o.Quantity.Sub(o.FilledQuantity).Abs()
	fillQty := remaining.Min(maxShares)
	if fillQty.IsZero() {
		return
	}

	// calculate weighted average fill price if we already have partial fills
	oldValue := o.FilledPrice.Mul(o.FilledQuantity.Abs())
	newValue := bar.VWAP.Mul(fillQty)
	newFilledQty := o.FilledQuantity.Abs().Add(fillQty)
	o.FilledPrice = oldValue.Add(newValue).Div(newFilledQty)

	// update filled quantity (preserving sign)
	if o.Side == ds.SideBuy {
		o.FilledQuantity = newFilledQty
	} else {
		o.FilledQuantity = newFilledQty.Neg()
	}

	// calculate and add fees (limit orders are maker orders)
	fee := gFeeCalculator.GetFee(clocky.Now(), fillQty, false)
	o.TotalFees = o.TotalFees.Add(fee)
	Cash = Cash.Sub(fee)

	// update cash and position
	tradeValue := bar.VWAP.Mul(fillQty)
	if o.Side == ds.SideBuy {
		Cash = Cash.Sub(tradeValue)
		o.Equity.Quantity = o.Equity.Quantity.Add(fillQty)
	} else {
		Cash = Cash.Add(tradeValue)
		o.Equity.Quantity = o.Equity.Quantity.Sub(fillQty)
	}

	// update order status
	if newFilledQty.Cmp(o.Quantity.Abs()) >= 0 {
		o.Status = alpaca.OrderStatusFilled
		delete(o.Equity.Orders, o.OrderID)
		delete(Orders, o.OrderID)
		o.log()
	} else {
		o.Status = alpaca.OrderStatusPartiallyFilled
	}
}

const (
	kColorNeutral = "\033[33m" // yellow
	kColorBuy     = "\033[32m" // green
	kColorSell    = "\033[31m" // red
	kColorProfit  = "\033[32m" // green
	kColorLoss    = "\033[31m" // red
	kColorReset   = "\033[0m"
)

func (o *Order) log() {
	if !Paper || *FlagVerbose {
		if o.FilledQuantity.IsZero() {
			log.Printf("%s %s%s%s %s order not filled (limit $%s, status %s)",
				o.OrderID, kColorNeutral, o.Side, kColorReset, o.Equity.Symbol, o.LimitPrice.Format(2), o.Status)
			return
		}
		h, m, s := clocky.Now().Clock()
		if o.Side == ds.SideBuy {
			log.Printf("%s %sBUY%s %02d:%02d:%02d %s %s @ $%s (fee $%s)",
				o.OrderID, kColorBuy, kColorReset,
				h, m, s,
				o.FilledQuantity.Abs(), o.Equity.Symbol,
				o.FilledPrice.Format(2), o.TotalFees.Format(2))
		} else {
			log.Printf("%s %sSELL%s %02d:%02d:%02d %s %s @ $%s (fee $%s, P/L)",
				o.OrderID, kColorSell, kColorReset,
				h, m, s,
				o.FilledQuantity.Abs(), o.Equity.Symbol,
				o.FilledPrice.Format(2), o.TotalFees.Format(2))
		}
	}
}
