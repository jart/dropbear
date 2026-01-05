package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"log"
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
	EntryPrice     decimal.Decimal // entry price at time of trade (for P/L)
	RealizedPL     decimal.Decimal // profit/loss on sells
	Reason         string          // optional reason for the trade (e.g. "TRAIL_STOP", "EOD")
}

func (o *Order) Wait() {
	// todo: limit order support
}

// Log logs the order details if verbose logging is enabled.
func (o *Order) Log() {
	if !*FlagVerbose {
		return
	}
	if o.FilledQuantity.IsZero() {
		log.Printf("\033[33m%s\033[0m %s order not filled (limit $%s, status %s)",
			o.Side, o.Equity.Symbol, o.LimitPrice.Format(2), o.Status)
		return
	}
	h, m, s := clocky.Now().Clock()
	if o.Side == ds.SideBuy {
		log.Printf("\033[32mBUY\033[0m %02d:%02d:%02d %s %s @ $%s (fee $%s)",
			h, m, s,
			o.FilledQuantity.Abs(), o.Equity.Symbol,
			o.FilledPrice.Format(2), o.TotalFees.Format(2))
	} else {
		plColor := "\033[32m" // green for profit
		if o.RealizedPL.IsNegative() {
			plColor = "\033[31m" // red for loss
		}
		reason := ""
		if o.Reason != "" {
			reason = " [" + o.Reason + "]"
		}
		log.Printf("\033[31mSELL\033[0m %02d:%02d:%02d %s %s @ $%s (fee $%s, P/L %s$%s\033[0m)%s",
			h, m, s,
			o.FilledQuantity.Abs(), o.Equity.Symbol,
			o.FilledPrice.Format(2), o.TotalFees.Format(2),
			plColor, o.RealizedPL.Format(2), reason)
	}
}
