package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
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
	OnCandle   func(*indicators.Candle)
	isReady    bool
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
		Symbol:   symbol,
		Asset:    asset,
		OnCandle: func(*indicators.Candle) {},
	}
	Equities[symbol] = e
	return e, nil
}

func (e *Equity) String() string {
	return e.Symbol
}

// GetMaxOrderQuantity returns the maximum number of shares that can be ordered.
func (e *Equity) GetMaxOrderQuantity() decimal.Decimal {
	marginUsed := GetMarginUsed()
	equity := GetPortfolioValue().Mul(gPowerLevel)
	maxMarginAvailable := gMaxMarginAvailable.Mul(gPowerLevel)
	marginAvailable := equity.Sub(marginUsed).Min(maxMarginAvailable)
	lo := decimal.Zero
	hi := decimal.FromInt(100_000)
	for lo.Cmp(hi) < 0 {
		mid := lo.Add(hi.Sub(lo).Add(decimal.One).DivInt(2))
		marginNeeded := e.Asset.GetInitialMargin(mid, e.Price)
		if marginNeeded.Cmp(marginAvailable) <= 0 {
			lo = mid
		} else {
			hi = mid.Sub(decimal.One)
		}
	}
	return lo.Truncate()
}

// MarketOrder places a market order for the given quantity of shares.
func (e *Equity) MarketOrder(quantity decimal.Decimal) (*Order, error) {
	if quantity.IsZero() {
		return nil, fmt.Errorf("cannot place zero-quantity order")
	}
	if quantity.Truncate().Cmp(quantity) != 0 {
		return nil, fmt.Errorf("only whole-share orders are supported")
	}
	if !e.Asset.Tradable.Load() {
		return nil, fmt.Errorf("asset %s is not tradable", e.Symbol)
	}
	side := ds.SideBuy
	price := e.AskPrice
	newQty := e.Quantity.Add(quantity)
	if quantity.IsNegative() {
		side = ds.SideSell
		price = e.BidPrice
		if newQty.IsNegative() {
			if !e.Asset.Shortable.Load() {
				return nil, fmt.Errorf("asset %s is not shortable", e.Symbol)
			}
			if !e.Asset.EasyToBorrow.Load() {
				return nil, fmt.Errorf("asset %s is not easy to borrow", e.Symbol)
			}
		}
	}
	if !price.IsPositive() {
		return nil, fmt.Errorf("cannot place order on %s that hasn't received data yet", e.Symbol)
	}
	if e.Quantity.Mul(newQty).IsNegative() {
		return nil, fmt.Errorf("order quantity %s would flip the %s %s position we hold", quantity, e.Quantity, e.Symbol)
	}
	if newQty.Abs().Cmp(e.Quantity.Abs()) > 0 {
		marginUsed := GetMarginUsed()
		equity := GetPortfolioValue().Mul(gPowerLevel)
		marginNeeded := e.Asset.GetInitialMargin(quantity, price)
		maxMarginAvailable := gMaxMarginAvailable.Mul(gPowerLevel)
		marginAvailable := equity.Sub(marginUsed).Min(maxMarginAvailable)
		if marginNeeded.Cmp(marginAvailable) > 0 {
			return nil, fmt.Errorf("need %s margin but only %s available", marginNeeded, marginAvailable)
		}
	}
	if Paper {
		filledQuantity := quantity
		notional := price.Mul(filledQuantity)
		e.Quantity = e.Quantity.Add(filledQuantity)
		fee := gFeeCalculator.GetFee(clocky.Now(), filledQuantity, true)
		Cash = Cash.Sub(fee).Sub(notional)
		orderID := uuid.New().String()
		return &Order{
			OrderID:        orderID,
			ClientOrderID:  orderID,
			Side:           side,
			Equity:         e,
			Status:         alpaca.OrderStatusFilled,
			Quantity:       quantity,
			FilledQuantity: filledQuantity,
			FilledPrice:    price,
			TotalFees:      fee,
		}, nil
	} else {
		response, err := Client.MarketOrder(e.Symbol, side, quantity.Abs(), alpaca.TimeInForceDay)
		if err != nil {
			return nil, err
		}
		filledQuantity := response.FilledQty
		if quantity.IsNegative() {
			filledQuantity = filledQuantity.Neg()
		}
		notional := price.Mul(filledQuantity)
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
			FilledPrice:    response.FilledAvgPrice,
			FilledQuantity: filledQuantity,
			TotalFees:      fee,
		}, nil
	}
}
