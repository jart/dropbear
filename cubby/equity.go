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

type Equity struct {
	Symbol     string // e.g. "GOOG", "BRK.B", etc.
	Asset      *alpaca.Asset
	Price      decimal.Decimal // current midpoint
	LastPrice  decimal.Decimal // last traded price
	AskPrice   decimal.Decimal // current ask price
	AskSize    decimal.Decimal // current ask size in number of shares
	BidPrice   decimal.Decimal // current bid price
	BidSize    decimal.Decimal // current bid size in number of shares
	Quantity   decimal.Decimal // number of shares held (negative if short)
	Hold       decimal.Decimal // number of shares held up in limit orders
	EntryPrice decimal.Decimal // average entry price for current position
	OnBar      func(*alpaca.Bar)
	nextBar    *alpaca.Bar // next bar (for filling orders without lookahead bias)
}

// Equities holds all added equities by symbol.
var Equities = make(map[string]*Equity)

var (
	ErrUnknownAsset = errors.New("unknown asset")
	ErrRunning      = errors.New("cubby is running")
	ErrNotEquity    = errors.New("asset not an equity")
)

// AddEquity creates and registers a new Equity with the given symbol.
func AddEquity(symbol string) (*Equity, error) {
	if Running {
		return nil, fmt.Errorf("cannot add equity while running")
	}
	e := Equities[symbol]
	if e != nil {
		return e, nil
	}
	asset := alpaca.GetAsset(symbol)
	if asset == nil {
		return nil, ErrUnknownAsset
	}
	if asset.Class != alpaca.AssetClassUSEquity {
		return nil, ErrNotEquity
	}
	e = &Equity{
		Symbol: symbol,
		Asset:  asset,
		OnBar:  func(*alpaca.Bar) {},
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
	switch timeInForce {
	case alpaca.TimeInForceIOC:
		// supported
	default:
		return nil, fmt.Errorf("unsupported time in force: %s", timeInForce)
	}
	if quantity.IsZero() {
		return nil, fmt.Errorf("cannot place zero-quantity order")
	}
	if quantity.Truncate().Cmp(quantity) != 0 {
		return nil, fmt.Errorf("only whole-share orders are supported")
	}
	if !e.Asset.Tradable.Load() {
		return nil, fmt.Errorf("asset %s is not tradable", e.Symbol)
	}
	if !limitPrice.IsPositive() {
		return nil, fmt.Errorf("limit price must be positive")
	}

	side := ds.SideBuy
	newQty := e.Quantity.Add(quantity)
	if quantity.IsNegative() {
		side = ds.SideSell
		if newQty.IsNegative() {
			if !e.Asset.Shortable.Load() {
				return nil, fmt.Errorf("asset %s is not shortable", e.Symbol)
			}
			if !e.Asset.EasyToBorrow.Load() {
				return nil, fmt.Errorf("asset %s is not easy to borrow", e.Symbol)
			}
		}
	}
	if e.nextBar == nil {
		return nil, fmt.Errorf("cannot place order on %s that hasn't received data yet", e.Symbol)
	}
	if e.Quantity.Mul(newQty).IsNegative() {
		return nil, fmt.Errorf("order quantity %s would flip the %s %s position we hold", quantity, e.Quantity, e.Symbol)
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

// simulateLimitOrder simulates an IOC limit order fill during backtesting.
// Uses nextBar to avoid lookahead bias - order fills on the bar AFTER the decision.
func (e *Equity) simulateLimitOrder(side ds.Side, quantity, limitPrice decimal.Decimal) (*Order, error) {
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
	e.Quantity = e.Quantity.Add(filledQuantity)
	fee := gFeeCalculator.GetFee(clocky.Now(), filledQuantity, true)
	Cash = Cash.Sub(fee).Sub(notional)

	order.Status = alpaca.OrderStatusFilled
	order.FilledQuantity = filledQuantity
	order.FilledPrice = fillPrice
	order.TotalFees = fee

	if filledQuantity.Abs().Cmp(quantity.Abs()) < 0 {
		order.Status = alpaca.OrderStatusPartiallyFilled
	}

	return order, nil
}

// executeLimitOrder executes a real limit order through the broker.
func (e *Equity) executeLimitOrder(side ds.Side, quantity, limitPrice decimal.Decimal, timeInForce alpaca.TimeInForce) (*Order, error) {
	response, err := Client.LimitOrder(e.Symbol, side, quantity.Abs(), limitPrice, timeInForce, false)
	if err != nil {
		return nil, err
	}
	filledQuantity := response.FilledQty
	if quantity.IsNegative() {
		filledQuantity = filledQuantity.Neg()
	}
	notional := response.FilledAvgPrice.Mul(filledQuantity)
	e.Quantity = e.Quantity.Add(filledQuantity)
	fee := gFeeCalculator.GetFee(clocky.Now(), filledQuantity, true)
	Cash = Cash.Sub(fee).Sub(notional)
	return &Order{
		OrderID:        response.ID,
		ClientOrderID:  response.ClientOrderID,
		Equity:         e,
		Side:           side,
		Status:         response.Status,
		Quantity:       quantity,
		LimitPrice:     limitPrice,
		FilledPrice:    response.FilledAvgPrice,
		FilledQuantity: filledQuantity,
		TotalFees:      fee,
	}, nil
}
