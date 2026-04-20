package main

import (
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/options"
	"dropbear/symbol"
	"errors"
	"fmt"
)

type Order struct {
	Trader        *Trader
	ID            int
	Created       clocky.Time
	OrderIDSchwab schwab.OrderID
	OrderIDAlpaca string
	ClientOrderID string
	Security      options.Security
	Quantity      decimal.Decimal // negative for sell, positive for buy, never zero
	Price         decimal.Decimal // always positive, or zero for market orders
	Unfilled      decimal.Decimal // always positive, or zero if fully filled
	Sent          bool
	Making        bool
	Filled        bool
	Canceling     bool
}

func (order *Order) String() string {
	return fmt.Sprintf("#%d", order.ID)
}

func (order *Order) MidPrice() decimal.Decimal {
	return order.Security.MidPrice()
}

func (order *Order) Send() error {
	if order.Sent {
		return errors.New("order already sent")
	}
	if order.Price.IsNegative() {
		return errors.New("order has negative price")
	}
	if order.Quantity.IsZero() {
		return errors.New("order has zero quantity")
	}
	if order.Price.Cmp(order.Price.QuantizeTruncate(order.priceTick())) != 0 {
		return errors.New("order price must be quantized properly")
	}
	if *dryFlag {
		return errors.New("won't send order in dry run mode")
	}
	order.Sent = true
	order.Unfilled = order.Quantity.Abs()
	if *liveFlag {
		if order.Trader.Config.Schwab {
			order.Trader.sendLiveOrderSchwab(order)
		} else {
			order.Trader.sendLiveOrderAlpaca(order)
		}
	} else {
		order.Trader.simulateOrder(order)
	}
	return nil
}

func (order *Order) Cancel() error {
	if !order.Sent {
		return errors.New("order not sent")
	}
	if order.Filled {
		return errors.New("order already filled")
	}
	if order.Canceling {
		return errors.New("order already canceling")
	}
	if *liveFlag && order.OrderIDSchwab == 0 && order.OrderIDAlpaca == "" {
		return errors.New("cannot cancel order that was never acknowledged by broker")
	}
	if *dryFlag {
		panic("not possible")
	}
	order.Canceling = true
	if *liveFlag {
		if order.Trader.Config.Schwab {
			go order.Trader.cancelOrderSchwab(order)
		} else {
			go order.Trader.cancelOrderAlpaca(order)
		}
	} else {
		order.Trader.simulateCancelOrder(order)
	}
	return nil
}

func (order *Order) EstimateFee(qty decimal.Decimal, first, marketable bool) decimal.Decimal {
	switch s := order.Security.(type) {
	case *options.Option:
		fee := decimal.Zero
		fee = fee.Add(kOptionsFeeORF.Mul(qty))
		fee = fee.Add(kOptionsFeeOCC.Mul(qty))
		if order.Quantity.IsNegative() {
			fee = fee.Add(kOptionsFeeTAF.Mul(qty))
		}
		if order.Trader.Config.Schwab {
			switch s.Symbol {
			case symbol.SPXW, symbol.RUTW, symbol.NDX:
				fee = kOptionsFeeSchwabCBOE.Mul(qty) // basket estimate
			default:
				fee = fee.Add(kOptionsFeeSchwab.Mul(qty))
			}
		}
		return fee
	case *options.Equity:
		fee := decimal.Zero
		if first {
			fee = fee.Add(kCatFeePerTrade)
			if !order.Trader.Config.Schwab {
				fee = fee.Add(kBrokerFeePerTrade)
			}
		}
		if order.Quantity.IsNegative() {
			fee = fee.Add(kTafFeePerShare.Mul(qty))
		}
		if !order.Trader.Config.Schwab {
			if marketable {
				fee = fee.Add(kExchangeTakerFeePerShare.Mul(qty))
			} else {
				fee = fee.Add(kExchangeMakerFeePerShare.Mul(qty))
			}
		}
		return fee
	default:
		panic("unknown security type")
	}
}

func (order *Order) priceTick() decimal.Decimal {
	tick, bigTick := order.Security.Ticks()
	if order.Price.Abs().Cmp(decimal.Three) >= 0 {
		return bigTick
	}
	return tick
}
