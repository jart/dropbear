package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var (
	Equities                  = make(map[string]*Equity)
	ErrUnknown                = errors.New("unknown symbol")
	ErrRunning                = errors.New("cubby is running")
	ErrNoData                 = errors.New("no data for equity")
	ErrNotEquity              = errors.New("asset not an equity")
	ErrIsRunning              = errors.New("cubby is running")
	ErrIsWarmingUp            = errors.New("cubby is warming up")
	ErrNotTradable            = errors.New("asset is not tradable")
	ErrNotShortable           = errors.New("asset is not shortable")
	ErrZeroQuantity           = errors.New("quantity cannot be zero")
	ErrNegativePrice          = errors.New("price cannot be negative")
	ErrNotEasyToBorrow        = errors.New("asset is not easy to borrow")
	ErrFractionalShares       = errors.New("quantity cannot be fractional")
	ErrOrderOverlapsZero      = errors.New("order quantity would flip position")
	ErrUnsupportedTimeInForce = errors.New("unsupported time in force")
)

type Equity struct {
	Symbol     string // e.g. "GOOG", "BRK.B", etc.
	Asset      *alpaca.Asset
	Price      decimal.Decimal // current midpoint
	Quantity   decimal.Decimal // number of shares held (negative if short)
	Hold       decimal.Decimal // number of shares held up in limit orders
	EntryPrice decimal.Decimal // average entry price for current position
	OnBar      func(*ds.Bar)
	nextBar    *ds.Bar // next bar (for filling orders without lookahead bias)
}

// AddEquity creates and registers a new Equity with the given symbol.
func AddEquity(symbol string) (*Equity, error) {
	if Running {
		return nil, ErrIsRunning
	}
	e := Equities[symbol]
	if e != nil {
		return e, nil
	}
	asset := alpaca.GetAsset(symbol)
	if asset == nil {
		return nil, ErrUnknown
	}
	if asset.Class != alpaca.AssetClassUSEquity {
		return nil, ErrNotEquity
	}
	e = &Equity{
		Symbol: symbol,
		Asset:  asset,
		OnBar:  func(*ds.Bar) {},
	}
	Equities[symbol] = e
	return e, nil
}

func (e *Equity) String() string {
	return e.Symbol
}

// GetMaxOrderQuantity returns the maximum number of shares that can be ordered.
func (e *Equity) GetMaxOrderQuantity(price decimal.Decimal) decimal.Decimal {
	marginUsed := GetMarginUsed()
	equity := GetPortfolioValue().Mul(gPowerLevel)
	maxMarginAvailable := gMaxMarginAvailable.Mul(gPowerLevel)
	marginAvailable := equity.Sub(marginUsed).Min(maxMarginAvailable)
	lo := decimal.Zero
	hi := decimal.FromInt(100_000)
	for lo.Cmp(hi) < 0 {
		mid := lo.Add(hi.Sub(lo).Add(decimal.One).DivInt(2))
		marginNeeded := e.Asset.GetInitialMargin(mid, price)
		if marginNeeded.Cmp(marginAvailable) <= 0 {
			lo = mid
		} else {
			hi = mid.Sub(decimal.One)
		}
	}
	return lo.Mul(decimal.One.Sub(*flagBuffer)).Truncate()
}

// LimitOrder places a limit order for the given quantity of shares.
// For backtesting, simulates fill based on whether limit price intersects the bar's range.
func (e *Equity) LimitOrder(quantity, limitPrice decimal.Decimal, timeInForce alpaca.TimeInForce) (*Order, error) {
	if IsWarmingUp {
		return nil, ErrIsWarmingUp
	}
	if quantity.IsZero() {
		return nil, ErrZeroQuantity
	}
	if quantity.Truncate().Cmp(quantity) != 0 {
		return nil, ErrFractionalShares
	}
	if !e.Asset.Tradable.Load() {
		return nil, ErrNotTradable
	}
	if !limitPrice.IsPositive() {
		return nil, ErrNegativePrice
	}
	if timeInForce != alpaca.TimeInForceIOC {
		return nil, ErrUnsupportedTimeInForce
	}

	side := ds.SideBuy
	newQty := e.Quantity.Add(quantity)
	if quantity.IsNegative() {
		side = ds.SideSell
		if newQty.IsNegative() {
			if !e.Asset.Shortable.Load() {
				return nil, ErrNotShortable
			}
			if !e.Asset.EasyToBorrow.Load() {
				return nil, ErrNotEasyToBorrow
			}
		}
	}
	if e.Quantity.Mul(newQty).IsNegative() {
		return nil, ErrOrderOverlapsZero
	}

	// check margin for new positions
	if newQty.Abs().Cmp(e.Quantity.Abs()) > 0 {
		marginUsed := GetMarginUsed()
		equity := GetPortfolioValue().Mul(gPowerLevel)
		marginNeeded := e.Asset.GetInitialMargin(quantity, limitPrice)
		maxMarginAvailable := gMaxMarginAvailable.Mul(gPowerLevel)
		marginAvailable := equity.Sub(marginUsed).Min(maxMarginAvailable)
		if marginNeeded.Cmp(marginAvailable) > 0 {
			return nil, fmt.Errorf("need %s margin but only %s available", marginNeeded, marginAvailable)
		}
	}

	if Paper {
		return e.simulateLimitOrder(side, quantity, limitPrice)
	}
	return e.executeLimitOrder(side, quantity, limitPrice, timeInForce)
}

// updatePosition updates the equity position after a fill and returns entry price and realized P/L.
func (e *Equity) updatePosition(side ds.Side, filledQuantity, fillPrice decimal.Decimal) (entryPrice, realizedPL decimal.Decimal) {
	entryPrice = e.EntryPrice
	if side == ds.SideBuy {
		// Buying: compute weighted average entry price
		oldNotional := e.EntryPrice.Mul(e.Quantity)
		newNotional := fillPrice.Mul(filledQuantity)
		e.Quantity = e.Quantity.Add(filledQuantity)
		if !e.Quantity.IsZero() {
			e.EntryPrice = oldNotional.Add(newNotional).Div(e.Quantity)
		}
	} else {
		// Selling: calculate realized P/L and reduce position
		realizedPL = fillPrice.Sub(entryPrice).Mul(filledQuantity.Abs())
		e.Quantity = e.Quantity.Add(filledQuantity)
		if e.Quantity.IsZero() {
			e.EntryPrice = decimal.Zero
		}
	}
	return entryPrice, realizedPL
}

// simulateLimitOrder simulates an IOC limit order fill during backtesting.
// Uses nextBar to avoid lookahead bias - order fills on the bar AFTER the decision.
func (e *Equity) simulateLimitOrder(side ds.Side, quantity, limitPrice decimal.Decimal) (*Order, error) {
	if e.nextBar == nil {
		return nil, ErrNoData
	}

	orderID := uuid.New().String()
	order := &Order{
		OrderID:       orderID,
		ClientOrderID: orderID,
		Side:          side,
		Equity:        e,
		Quantity:      quantity,
		LimitPrice:    limitPrice,
	}

	// Use next bar for fill simulation (no lookahead bias)
	bar := e.nextBar
	if bar == nil {
		order.Status = alpaca.OrderStatusCanceled
		return order, nil
	}

	var fillPrice decimal.Decimal
	var fillRatio decimal.Decimal

	if side == ds.SideBuy {
		// BUY: need price to come down to our limit
		// limitPrice is max we're willing to pay
		if limitPrice.Cmp(bar.Low) < 0 {
			// Our limit is below the bar's low - no fill possible
			order.Status = alpaca.OrderStatusCanceled
			return order, nil
		}
		if limitPrice.Cmp(bar.High) >= 0 {
			// We're willing to pay more than bar's high - full fill at VWAP
			fillPrice = bar.VWAP
			fillRatio = decimal.One
		} else {
			// Partial fill: our limit is within the bar's range
			// Estimate fill ratio based on where our limit sits in the range
			barRange := bar.High.Sub(bar.Low)
			if barRange.IsPositive() {
				// How much of the bar traded at or below our limit?
				fillRatio = limitPrice.Sub(bar.Low).Div(barRange)
			} else {
				fillRatio = decimal.One
			}
			// Fill at our limit price (we got what we asked for)
			fillPrice = limitPrice
		}
	} else {
		// SELL: need price to come up to our limit
		// limitPrice is min we're willing to accept
		if limitPrice.Cmp(bar.High) > 0 {
			// Our limit is above the bar's high - no fill possible
			order.Status = alpaca.OrderStatusCanceled
			return order, nil
		}
		if limitPrice.Cmp(bar.Low) <= 0 {
			// We're willing to accept less than bar's low - full fill at VWAP
			fillPrice = bar.VWAP
			fillRatio = decimal.One
		} else {
			// Partial fill: our limit is within the bar's range
			barRange := bar.High.Sub(bar.Low)
			if barRange.IsPositive() {
				// How much of the bar traded at or above our limit?
				fillRatio = bar.High.Sub(limitPrice).Div(barRange)
			} else {
				fillRatio = decimal.One
			}
			// Fill at our limit price
			fillPrice = limitPrice
		}
	}

	// Calculate filled quantity based on fill ratio and available volume
	maxFillFromVolume := bar.Volume.Mul(fillRatio)
	filledQuantity := quantity.Abs().Min(maxFillFromVolume).Truncate()
	if side == ds.SideSell {
		filledQuantity = filledQuantity.Neg()
	}

	if filledQuantity.IsZero() {
		order.Status = alpaca.OrderStatusCanceled
		return order, nil
	}

	// Calculate slippage: difference between fill price and expected price (close)
	// For buys: positive slippage means we paid more than close
	// For sells: positive slippage means we received less than close
	var slippage decimal.Decimal
	if side == ds.SideBuy {
		slippage = fillPrice.Sub(e.Price).Mul(filledQuantity)
	} else {
		slippage = e.Price.Sub(fillPrice).Mul(filledQuantity.Abs())
	}
	gTotalSlippage = gTotalSlippage.Add(slippage)

	// Update position and cash
	notional := fillPrice.Mul(filledQuantity)
	fee := gFeeCalculator.GetFee(clocky.Now(), filledQuantity, true)
	Cash = Cash.Sub(fee).Sub(notional)
	entryPrice, realizedPL := e.updatePosition(side, filledQuantity, fillPrice)

	order.Status = alpaca.OrderStatusFilled
	order.FilledQuantity = filledQuantity
	order.FilledPrice = fillPrice
	order.TotalFees = fee
	order.EntryPrice = entryPrice
	order.RealizedPL = realizedPL

	if filledQuantity.Abs().Cmp(quantity.Abs()) < 0 {
		order.Status = alpaca.OrderStatusPartiallyFilled
	}

	return order, nil
}

const (
	kIOCWait  = 23 * clocky.Second
	kIOCSleep = 1 * clocky.Second
)

// executeLimitOrder executes a real limit order through the broker.
func (e *Equity) executeLimitOrder(side ds.Side, quantity, limitPrice decimal.Decimal, timeInForce alpaca.TimeInForce) (*Order, error) {
TryAgain:
	alpacaOrder, err := Client.LimitOrder(e.Symbol, side, quantity.Abs(), limitPrice, timeInForce, alpaca.OrderAlgorithmNone, alpaca.OrderDestinationNone, false)
	if err != nil {
		if err == ds.ErrTooManyRequests {
			clocky.Sleep(1 * clocky.Second)
			goto TryAgain
		}
		return nil, err
	}
	waitUntil := clocky.Now().Add(kIOCWait)
	for {
		if alpacaOrder.Status.IsFinal() {
			order := e.convertAlpacaOrder(side, quantity, limitPrice, alpacaOrder)
			order.log()
			return order, nil
		}
		if clocky.Now().After(waitUntil) {
		TryAgain2:
			err := Client.CancelOrder(alpacaOrder.ID)
			if err != nil {
				if err == ds.ErrTooManyRequests {
					clocky.Sleep(1 * clocky.Second)
					goto TryAgain2
				}
				return nil, fmt.Errorf("error cancelling order %s: %v", alpacaOrder.ID, err)
			}
		} else {
			clocky.Sleep(kIOCSleep)
		}
		alpacaOrder, err = Client.GetOrder(alpacaOrder.ID)
		if err != nil {
			return nil, fmt.Errorf("error fetching order %s: %v", alpacaOrder.ID, err)
		}
	}
}

func (e *Equity) convertAlpacaOrder(side ds.Side, quantity, limitPrice decimal.Decimal, alpacaOrder *alpaca.Order) *Order {
	filledQuantity := alpacaOrder.FilledQty
	if quantity.IsNegative() {
		filledQuantity = filledQuantity.Neg()
	}
	notional := alpacaOrder.FilledAvgPrice.Mul(filledQuantity)
	fee := gFeeCalculator.GetFee(clocky.Now(), filledQuantity, true)
	Cash = Cash.Sub(fee).Sub(notional)
	entryPrice, realizedPL := e.updatePosition(side, filledQuantity, alpacaOrder.FilledAvgPrice)
	return &Order{
		OrderID:        alpacaOrder.ID,
		ClientOrderID:  alpacaOrder.ClientOrderID,
		Equity:         e,
		Side:           side,
		Status:         alpacaOrder.Status,
		Quantity:       quantity,
		LimitPrice:     limitPrice,
		FilledPrice:    alpacaOrder.FilledAvgPrice,
		FilledQuantity: filledQuantity,
		TotalFees:      fee,
		EntryPrice:     entryPrice,
		RealizedPL:     realizedPL,
	}
}
