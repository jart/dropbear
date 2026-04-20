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
	Trader    *Trader
	ID        int
	Created   clocky.Time
	OrderID   schwab.OrderID
	Security  options.Security
	Quantity  decimal.Decimal // negative for sell, positive for buy, never zero
	Price     decimal.Decimal // always positive, or zero for market orders
	Unfilled  decimal.Decimal
	Sent      bool
	Making    bool
	Filled    bool
	Canceling bool
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
		order.Trader.sendLiveOrder(order)
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

func (order *Order) EstimateFee(first, marketable bool) decimal.Decimal {
	qty := order.Quantity.Abs()
	switch s := order.Security.(type) {
	case *options.Option:
		switch s.Symbol {
		case symbol.SPXW, symbol.RUTW, symbol.NDX:
			return kFeePerOptionsContractSPXW.Mul(qty)
		default:
			return kFeePerOptionsContract.Mul(qty)
		}
	case *options.Equity:
		fee := decimal.Zero
		if first {
			fee = fee.Add(kCatFeePerTrade)
			fee = fee.Add(kBrokerFeePerTrade)
		}
		fee = fee.Add(kTafFeePerShare.Mul(qty))
		if marketable {
			fee = fee.Add(kExchangeTakerFeePerShare.Mul(qty))
		} else {
			fee = fee.Add(kExchangeMakerFeePerShare.Mul(qty))
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
