package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/ds/symbol"
	"fmt"
	"log"
)

var Equities = make(map[symbol.Symbol]*Equity)

type Equity struct {
	Symbol     symbol.Symbol // e.g. "GOOG", "BRK.B", etc.
	Asset      *alpaca.Asset
	Price      decimal.Decimal // current midpoint
	Quantity   decimal.Decimal // number of shares held (negative if short)
	EntryPrice decimal.Decimal // average entry price for current position
	OnBar      func(*ds.Bar)
	Orders     map[string]*Order
}

// AddEquity creates and registers a new Equity with the given symbol.
func AddEquity(sym symbol.Symbol) (*Equity, error) {
	if Running {
		return nil, ErrIsRunning
	}
	e := Equities[sym]
	if e != nil {
		return e, nil
	}
	asset := alpaca.GetAsset(sym)
	if asset == nil {
		return nil, ErrUnknown
	}
	if asset.Class != alpaca.AssetClassUSEquity {
		return nil, ErrNotEquity
	}
	e = &Equity{
		Symbol: sym,
		Asset:  asset,
		OnBar:  func(*ds.Bar) {},
		Orders: make(map[string]*Order),
	}
	Equities[sym] = e
	return e, nil
}

func (e *Equity) String() string {
	return e.Symbol.String()
}

// GetMaxOrderQuantity returns the maximum number of shares that can be ordered.
func (e *Equity) GetMaxOrderQuantity(price decimal.Decimal) decimal.Decimal {
	if !Paper {
		account, err := Client.GetAccount()
		if err != nil {
			log.Printf("error getting alpaca account info: %v", err)
		} else {
			return account.BuyingPower.Div(e.Price).Truncate()
		}
	}
	marginUsed := GetMarginUsed()
	equity := GetPortfolioValue().Mul(gPowerLevel)
	maxMarginAvailable := gMaxMarginAvailable.Mul(gPowerLevel)
	marginAvailable := equity.Sub(marginUsed).Sub(gMarginHold).Min(maxMarginAvailable)
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
	return lo.Mul(decimal.One.Sub(*FlagBuffer)).Truncate()
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
	if limitPrice.QuantizeTruncate(decimal.Cent).Cmp(limitPrice) != 0 {
		return nil, ErrPriceNotRounded
	}
	newQty := e.Quantity.Add(quantity)
	side := ds.SideBuy
	if quantity.IsNegative() {
		side = ds.SideSell
		quantity = quantity.Abs()
		if newQty.IsNegative() {
			if !e.Asset.Shortable.Load() {
				return nil, ErrNotShortable
			}
			if !e.Asset.EasyToBorrow.Load() {
				return nil, ErrNotEasyToBorrow
			}
		}
	}
	order := &Order{
		ClientOrderID: generateOrderID(),
		Type:          alpaca.OrderTypeLimit,
		Equity:        e,
		Side:          side,
		Status:        alpaca.OrderStatusNew,
		Quantity:      quantity,
		LimitPrice:    limitPrice,
		OrderedAt:     clocky.Now(),
	}
	if !Paper {
		return e.sendOrder(order)
	}
	return e.simulateOrder(order, newQty)
}

func (e *Equity) simulateOrder(order *Order, newQty decimal.Decimal) (*Order, error) {
	now := clocky.Now()
	time := now.ClockInt()
	if time < 06_30_00 || time >= 12_59_55 {
		return nil, ErrMarketNotOpen
	}
	price := order.LimitPrice.Min(e.Price)
	newMargin := e.Asset.GetInitialMargin(newQty, price)
	oldMargin := e.Asset.GetMaintenanceMargin(e.Quantity, price)
	if newMargin.Cmp(oldMargin) > 0 {
		marginNeeded := newMargin.Sub(oldMargin)
		marginUsed := GetMarginUsed()
		equity := GetPortfolioValue().Mul(gPowerLevel)
		maxMarginAvailable := gMaxMarginAvailable.Mul(gPowerLevel)
		marginAvailable := equity.Sub(marginUsed).Sub(gMarginHold).Min(maxMarginAvailable)
		if marginNeeded.Cmp(marginAvailable) > 0 {
			return nil, fmt.Errorf("need %s margin but only %s available", marginNeeded, marginAvailable)
		}
		gMarginHold = gMarginHold.Add(marginNeeded)
		order.MarginHeld = marginNeeded
	}
	order.OrderID = order.ClientOrderID
	order.logPlacedOrder()
	e.Orders[order.OrderID] = order
	Orders[order.OrderID] = order
	return order, nil
}

func (e *Equity) sendOrder(order *Order) (*Order, error) {
	var startTime, endTime clocky.Time
	now := clocky.Now()
	time := now.ClockInt()
	if time < 06_30_00 || time >= 12_59_55 {
		return nil, ErrMarketNotOpen
	}
	startTime = now
	endTime = now.Add(*FlagPatience)
	if endTime.ClockInt() >= 13_00_00 {
		// just rely on the default behavior of lasting until close
		// todo: will alpaca do the right thing if we overlap market close?
		startTime = 0
		endTime = 0
	}
	ordersByCID[order.ClientOrderID] = order
	alpacaOrder, err := Client.CreateOrder(&alpaca.OrderRequest{
		Symbol:        e.Symbol,
		Side:          order.Side,
		Qty:           order.Quantity,
		LimitPrice:    order.LimitPrice,
		ClientOrderID: order.ClientOrderID,
		Type:          alpaca.OrderTypeLimit,
		TimeInForce:   alpaca.TimeInForceDay,
		AdvancedInstructions: &alpaca.AdvancedInstructions{
			Algorithm:     alpaca.OrderAlgorithmVWAP,
			MaxPercentage: *FlagVWAP,
			StartTime:     startTime,
			EndTime:       endTime,
		},
	})
	if err != nil {
		return nil, err
	}
	order.OrderID = alpacaOrder.ID
	order.logPlacedOrder()
	Orders[order.OrderID] = order
	e.Orders[order.OrderID] = order
	order.sync(alpacaOrder)
	return order, nil
}
