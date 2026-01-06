package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/decimal"
	"dropbear/ds"
	"fmt"

	"github.com/google/uuid"
)

var Equities = make(map[string]*Equity)

type Equity struct {
	Symbol     string // e.g. "GOOG", "BRK.B", etc.
	Asset      *alpaca.Asset
	Price      decimal.Decimal // current midpoint
	Quantity   decimal.Decimal // number of shares held (negative if short)
	EntryPrice decimal.Decimal // average entry price for current position
	OnBar      func(*ds.Bar)
	Orders     map[string]*Order
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
		Orders: make(map[string]*Order),
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

// Order places a volume weighted limit order for the given quantity of shares.
func (e *Equity) Order(quantity, limitPrice decimal.Decimal) (*Order, error) {
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
	if Paper {
		return e.simulateOrder(side, quantity, limitPrice, newQty)
	}
	return e.executeOrder(side, quantity, limitPrice)
}

func (e *Equity) simulateOrder(side ds.Side, quantity, limitPrice, newQty decimal.Decimal) (*Order, error) {
	if newQty.Abs().Cmp(e.Quantity.Abs()) > 0 {
		marginUsed := GetMarginUsed()
		equity := GetPortfolioValue().Mul(gPowerLevel)
		marginNeeded := e.Asset.GetInitialMargin(quantity, limitPrice.Min(e.Price))
		maxMarginAvailable := gMaxMarginAvailable.Mul(gPowerLevel)
		marginAvailable := equity.Sub(marginUsed).Min(maxMarginAvailable)
		if marginNeeded.Cmp(marginAvailable) > 0 {
			return nil, fmt.Errorf("need %s margin but only %s available", marginNeeded, marginAvailable)
		}
	}
	order := &Order{
		OrderID:    uuid.New().String(),
		Side:       side,
		Status:     alpaca.OrderStatusNew,
		Equity:     e,
		Quantity:   quantity,
		LimitPrice: limitPrice,
	}
	e.Orders[order.OrderID] = order
	Orders[order.OrderID] = order
	return order, nil
}

func (e *Equity) executeOrder(side ds.Side, quantity, limitPrice decimal.Decimal) (*Order, error) {
	clientOrderID := uuid.New().String()
	order := &Order{
		ClientOrderID: clientOrderID,
		Equity:        e,
		Side:          side,
		Status:        alpaca.OrderStatusNew,
		Quantity:      quantity,
		LimitPrice:    limitPrice,
	}
	ordersByCID[order.ClientOrderID] = order
	alpacaOrder, err := Client.CreateOrder(&alpaca.OrderRequest{
		Side:        side,
		Symbol:      e.Symbol,
		LimitPrice:  limitPrice,
		Qty:         quantity.Abs(),
		Type:        alpaca.OrderTypeLimit,
		TimeInForce: alpaca.TimeInForceDay,
		AdvancedInstructions: &alpaca.AdvancedInstructions{
			Algorithm:     alpaca.OrderAlgorithmVWAP,
			MaxPercentage: *flagVWAP,
		},
	})
	if err != nil {
		order.Status = alpaca.OrderStatusRejected
		delete(ordersByCID, order.ClientOrderID)
		return nil, err
	}
	order.OrderID = alpacaOrder.ID
	order.Status = alpacaOrder.Status
	order.FilledPrice = alpacaOrder.FilledAvgPrice
	order.FilledQuantity = alpacaOrder.FilledQty
	if order.Quantity.IsNegative() {
		order.FilledQuantity = order.FilledQuantity.Neg()
	}
	Orders[order.OrderID] = order
	e.Orders[order.OrderID] = order
	return order, nil
}
