package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/symbol"
	"fmt"
)

var Equities = make(map[symbol.Symbol]*Equity)

type Equity struct {
	Symbol     symbol.Symbol // e.g. "GOOG", "BRK.B", etc.
	Asset      *alpaca.Asset
	Price      decimal.Decimal
	Quantity   decimal.Decimal // number of shares held (negative if short)
	EntryPrice decimal.Decimal // average entry price for current position (never negative)
	Orders     map[string]*Order
	OnBar      func(*ds.Bar)
}

// AddEquity creates and registers a new Equity with the given symbol.
func AddEquity(sym symbol.Symbol) (*Equity, error) {
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
	marginUsed := GetMarginUsed()
	marginAvailable := gDayTradingBuyingPower.Sub(marginUsed).Sub(gMarginHold)
	oldMargin := e.Asset.GetMaintenanceMargin(e.Quantity, price)
	lo := decimal.Zero
	hi := decimal.FromInt(100_000)
	for lo.Cmp(hi) < 0 {
		mid := lo.Add(hi.Sub(lo).Add(decimal.One).Half())
		newQty := e.Quantity.Add(mid)
		newMargin := e.Asset.GetInitialMargin(newQty, price)
		marginNeeded := newMargin.Sub(oldMargin)
		if marginNeeded.Cmp(marginAvailable) <= 0 {
			lo = mid
		} else {
			hi = mid.Sub(decimal.One)
		}
	}
	return lo.Mul(decimal.One.Sub(*FlagBuffer)).Truncate()
}

// Order places a volume weighted limit order for the given quantity of shares.
// If you place your order from 6:00:00 to 6:27:58 a.m. PT, it'll be a LOO order.
// If you place your order from 12:45:00 to 12:49:58 p.m. PT, it'll be a LOC order.
func (e *Equity) Order(quantity, limitPrice decimal.Decimal) (*Order, error) {

	// validate order
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

	// check buying power
	marginNeeded := decimal.Zero
	price := limitPrice.Min(e.Price)
	newMargin := e.Asset.GetInitialMargin(newQty, price)
	oldMargin := e.Asset.GetMaintenanceMargin(e.Quantity, price)
	if newMargin.Cmp(oldMargin) > 0 {
		marginNeeded = newMargin.Sub(oldMargin)
		marginUsed := GetMarginUsed()
		marginAvailable := gDayTradingBuyingPower.Sub(marginUsed).Sub(gMarginHold)
		if marginNeeded.Cmp(marginAvailable) > 0 {
			return nil, fmt.Errorf("need %s margin but only %s available", marginNeeded, marginAvailable)
		}
	}

	// check that new position won't cause maintenance margin to exceed portfolio value
	// this prevents margin calls even when DTBP (based on previous day) allows the trade
	newMaintMargin := e.Asset.GetMaintenanceMargin(newQty, price)
	oldMaintMargin := e.Asset.GetMaintenanceMargin(e.Quantity, price)
	maintMarginDelta := newMaintMargin.Sub(oldMaintMargin)
	currentMarginUsed := GetMarginUsed()
	totalMaintMargin := currentMarginUsed.Add(maintMarginDelta)
	portfolioValue := GetPortfolioValue()
	// Only check buys (positive delta) - sells should always be allowed to reduce exposure
	if maintMarginDelta.IsPositive() && totalMaintMargin.Cmp(portfolioValue) > 0 {
		return nil, fmt.Errorf("would cause margin call: maint margin %s > portfolio %s",
			totalMaintMargin, portfolioValue)
	}

	// check time
	now := clocky.Now()
	year, month, day := now.Date()
	timeInForce := alpaca.TimeInForceDay
	openTime := cboe.GetOpenTime(year, month, day)
	closeTime := cboe.GetCloseTime(year, month, day)
	time := now.ClockInt()
	if now.Before(openTime) || !now.Before(closeTime) {
		// OPG orders submitted after 9:28am but before 7:00pm ET will be rejected
		// https://docs.alpaca.markets/docs/orders-at-alpaca
		if time >= 6_27_59 {
			return nil, ErrMissedLOODeadline
		} else if time >= 6_00_00 {
			// this is a sane enough time to place a LOO order
			timeInForce = alpaca.TimeInForceOPG
		} else {
			return nil, ErrMarketNotOpen
		}
	} else if now.After(openTime) && now.Before(closeTime) {
		// Alpaca requires LOC orders be placed at least 10 minutes before close.
		if time >= 12_45_00 {
			timeInForce = alpaca.TimeInForceCLS
		}
	}

	// create order
	order := &Order{
		ClientOrderID: generateOrderID(),
		Type:          alpaca.OrderTypeLimit,
		Equity:        e,
		Side:          side,
		Status:        alpaca.OrderStatusNew,
		TimeInForce:   timeInForce,
		MarginHeld:    marginNeeded,
		Quantity:      quantity,
		LimitPrice:    limitPrice,
		OrderedAt:     now,
	}
	gMarginHold = gMarginHold.Add(marginNeeded)

	// dispatch order
	if !Paper {
		return e.sendOrder(order, now)
	}
	return e.simulateOrder(order)
}

func (e *Equity) simulateOrder(order *Order) (*Order, error) {
	order.OrderID = order.ClientOrderID
	order.logPlacedOrder()
	e.Orders[order.OrderID] = order
	Orders[order.OrderID] = order
	return order, nil
}

func (e *Equity) sendOrder(order *Order, now clocky.Time) (*Order, error) {
	var advancedInstructions *alpaca.AdvancedInstructions
	if order.TimeInForce == alpaca.TimeInForceDay {
		advancedInstructions = &alpaca.AdvancedInstructions{
			Algorithm:     alpaca.OrderAlgorithmVWAP,
			MaxPercentage: *FlagVWAP,
			EndTime:       now.Add(*FlagPatience),
		}
	}
	ordersByCID[order.ClientOrderID] = order
	alpacaOrder, err := Client.CreateOrder(&alpaca.CreateOrderRequest{
		Symbol:               e.Symbol.String(),
		Side:                 order.Side,
		Qty:                  order.Quantity,
		LimitPrice:           order.LimitPrice,
		ClientOrderID:        order.ClientOrderID,
		Type:                 alpaca.OrderTypeLimit,
		TimeInForce:          order.TimeInForce,
		AdvancedInstructions: advancedInstructions,
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
