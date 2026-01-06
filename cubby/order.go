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
	if o.FilledQuantity.IsZero() {
		log.Printf("%s%s%s %s order not filled (limit $%s, status %s)",
			kColorNeutral, o.Side, kColorReset, o.Equity.Symbol, o.LimitPrice.Format(2), o.Status)
		return
	}
	h, m, s := clocky.Now().Clock()
	if o.Side == ds.SideBuy {
		log.Printf("%sBUY%s %02d:%02d:%02d %s %s @ $%s (fee $%s)",
			kColorBuy, kColorReset,
			h, m, s,
			o.FilledQuantity.Abs(), o.Equity.Symbol,
			o.FilledPrice.Format(2), o.TotalFees.Format(2))
	} else {
		plColor := kColorProfit
		if o.RealizedPL.IsNegative() {
			plColor = kColorLoss
		}
		log.Printf("%sSELL%s %02d:%02d:%02d %s %s @ $%s (fee $%s, P/L %s$%s%s)",
			kColorSell, kColorReset,
			h, m, s,
			o.FilledQuantity.Abs(), o.Equity.Symbol,
			o.FilledPrice.Format(2), o.TotalFees.Format(2),
			plColor, o.RealizedPL.Format(2), kColorReset)
	}
}
