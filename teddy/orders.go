package teddy

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/exchange/coinbase"
	"dropbear/loggy"
	"strings"
	"sync"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

type Orders struct {
	Exchange     *Exchange
	lock         sync.RWMutex
	ordersArray  []*Order
	ordersMap    map[string]*Order // by clientOrderID or orderID
	openOrders   *treeset.Set[*Order]
	OnOrderEvent func(*Order)
}

// Get retrieves an order by its OrderID or ClientOrderID.
func (os *Orders) Get(ID string) *Order {
	os.lock.RLock()
	defer os.lock.RUnlock()
	return os.ordersMap[ID]
}

// All retrieves all orders in insertion order.
func (os *Orders) All() []*Order {
	os.lock.RLock()
	defer os.lock.RUnlock()
	result := make([]*Order, 0, len(os.ordersArray))
	for _, order := range os.ordersArray {
		result = append(result, order)
	}
	return result
}

// Open retrieves all open orders sorted by client order id.
func (os *Orders) Open() []*Order {
	os.lock.RLock()
	defer os.lock.RUnlock()
	result := make([]*Order, 0)
	for it := os.openOrders.Iterator(); it.Next(); {
		result = append(result, it.Value())
	}
	return result
}

func newOrders(ex *Exchange) *Orders {
	os := &Orders{
		Exchange:     ex,
		OnOrderEvent: func(*Order) {},
		ordersArray:  make([]*Order, 0),
		ordersMap:    make(map[string]*Order),
		openOrders:   treeset.NewWith(compareOrdersByClientOrderID),
	}
	if Live {
		switch ex.Exchange {
		case ds.ExchangeCoinbase:
			go os.coinbaseOrderUpdateDaemon()
		}
	}
	return os
}

func (os *Orders) create(pair *Pair, otype ds.OrderType, side ds.Side, quantity, limitPrice, hold decimal.Decimal) *Order {
	order := &Order{
		Pair:          pair,
		CreatedTime:   clocky.Now(),
		State:         ds.OrderStateNew,
		ClientOrderID: GenerateOrderID(),
		onClose:       make(chan struct{}),
		Type:          otype,
		Hold:          hold,
		Side:          side,
		Quantity:      quantity,
		LimitPrice:    limitPrice,
	}
	os.lock.Lock()
	os.ordersMap[order.ClientOrderID] = order
	os.ordersArray = append(os.ordersArray, order)
	os.openOrders.Add(order)
	os.lock.Unlock()
	pair.Lock.Lock()
	pair.openOrders.Add(order)
	pair.Lock.Unlock()
	os.OnOrderEvent(order)
	return order
}

func (os *Orders) coinbaseOrderUpdateDaemon() {
	for orderUpdate := range coinbase.OrderUpdates() {
		os.onCoinbaseOrderUpdate(orderUpdate)
	}
}

func (os *Orders) onCoinbaseOrderUpdate(orderUpdate *coinbase.OrderUpdate) {
	now := clocky.Now()
	pair := os.Exchange.Pairs.Get(orderUpdate.ProductID)
	os.lock.Lock()
	order, exists := os.ordersMap[orderUpdate.OrderID]
	mustAddToPair := false
	if !exists {
		order, exists = os.ordersMap[orderUpdate.ClientOrderID]
		if !exists {
			order = &Order{
				CreatedTime:   now,
				Pair:          pair,
				OrderID:       orderUpdate.OrderID,
				ClientOrderID: orderUpdate.ClientOrderID,
				Type:          ds.MustParseOrderType(orderUpdate.OrderType),
				Side:          ds.MustParseSide(orderUpdate.OrderSide),
				onClose:       make(chan struct{}),
			}
			mustAddToPair = true
			os.ordersMap[orderUpdate.OrderID] = order
			os.ordersArray = append(os.ordersArray, order)
			os.openOrders.Add(order)
			if orderUpdate.ClientOrderID != "" {
				os.ordersMap[orderUpdate.ClientOrderID] = order
			}
		} else {
			order.OrderID = orderUpdate.OrderID
			os.ordersMap[orderUpdate.OrderID] = order
		}
	}
	os.lock.Unlock()
	createdTime := clocky.MustParseTime(orderUpdate.CreationTime)
	baseHolding := os.Exchange.Holdings.Get(order.Pair.BaseCurrency)
	quoteHolding := os.Exchange.Holdings.Get(order.Pair.QuoteCurrency)
	order.Lock.Lock()
	oldFee := order.Fee
	oldHold := order.Hold
	oldFill := order.Filled
	oldState := order.State
	oldValue := order.FillValue
	order.CreatedTime = createdTime
	order.Filled = decimal.Parse(orderUpdate.CumulativeQuantity)
	order.Quantity = decimal.Parse(orderUpdate.LeavesQuantity).Add(order.Filled)
	order.LimitPrice = decimal.Parse(orderUpdate.LimitPrice)
	order.FillPrice = decimal.Parse(orderUpdate.AvgPrice)
	order.FillValue = decimal.Parse(orderUpdate.FilledValue)
	order.Hold = decimal.Parse(orderUpdate.OutstandingHoldAmount)
	order.Fee = decimal.Parse(orderUpdate.TotalFees)
	order.Side = ds.MustParseSide(orderUpdate.OrderSide)
	newState := ds.NewOrderStateForCoinbase(orderUpdate.Status, order.Filled)
	valueDelta := order.FillValue.Sub(oldValue)
	holdDelta := order.Hold.Sub(oldHold)
	fillDelta := order.Filled.Sub(oldFill)
	feeDelta := order.Fee.Sub(oldFee)
	order.State = newState
	if fillDelta.IsPositive() {
		order.LastFillTime = now
	}
	order.Lock.Unlock()
	if feeDelta.IsNegative() {
		loggy.Fatalf("fee delta went negative: %s -> %s", oldFee, order.Fee)
	}
	if fillDelta.IsNegative() {
		loggy.Fatalf("fill delta went negative: %s -> %s", oldFill, order.Filled)
	}
	if valueDelta.IsNegative() {
		loggy.Fatalf("value delta went negative: %s -> %s", oldValue, order.FillValue)
	}
	switch order.Side {
	case ds.SideBuy:
		if fillDelta.IsPositive() {
			baseHolding.Lock.Lock()
			baseHolding.Quantity = baseHolding.Quantity.Add(fillDelta)
			baseHolding.Available = baseHolding.Available.Add(fillDelta)
			baseHolding.Volume = baseHolding.Volume.Add(fillDelta)
			baseHolding.BuyVolume = baseHolding.BuyVolume.Add(fillDelta)
			baseHolding.Lots.Add(now, fillDelta, valueDelta)
			baseHolding.Lock.Unlock()
		}
		if feeDelta.IsPositive() || valueDelta.IsPositive() || !holdDelta.IsZero() {
			quoteHolding.Lock.Lock()
			quoteHolding.Quantity = quoteHolding.Quantity.Sub(feeDelta)
			quoteHolding.Available = quoteHolding.Available.Sub(feeDelta)
			quoteHolding.Quantity = quoteHolding.Quantity.Sub(valueDelta)
			quoteHolding.Available = quoteHolding.Available.Sub(holdDelta)
			quoteHolding.Volume = quoteHolding.Volume.Add(valueDelta)
			quoteHolding.SellVolume = quoteHolding.SellVolume.Add(valueDelta)
			quoteHolding.Lock.Unlock()
		}
	case ds.SideSell:
		if fillDelta.IsPositive() || !holdDelta.IsZero() {
			baseHolding.Lock.Lock()
			baseHolding.Quantity = baseHolding.Quantity.Sub(fillDelta)
			baseHolding.Available = baseHolding.Available.Sub(holdDelta)
			baseHolding.Volume = baseHolding.Volume.Add(fillDelta)
			baseHolding.SellVolume = baseHolding.SellVolume.Add(fillDelta)
			baseHolding.Lots.Consume(fillDelta, now, decimal.Zero)
			baseHolding.Lock.Unlock()
		}
		if fillDelta.IsPositive() {
			quoteHolding.Lock.Lock()
			quoteHolding.Quantity = quoteHolding.Quantity.Sub(feeDelta)
			quoteHolding.Quantity = quoteHolding.Quantity.Add(valueDelta)
			quoteHolding.Available = quoteHolding.Available.Add(valueDelta)
			quoteHolding.Volume = quoteHolding.Volume.Add(valueDelta)
			quoteHolding.BuyVolume = quoteHolding.BuyVolume.Add(valueDelta)
			quoteHolding.Lock.Unlock()
		}
	}
	if feeDelta.IsPositive() {
		os.Exchange.Lock.Lock()
		os.Exchange.Fees = os.Exchange.Fees.Add(feeDelta)
		os.Exchange.Lock.Unlock()
	}
	if mustAddToPair && !newState.IsFinal() {
		order.Pair.Lock.Lock()
		order.Pair.openOrders.Add(order)
		order.Pair.Lock.Unlock()
	}
	if !mustAddToPair && newState.IsFinal() && !oldState.IsFinal() {
		order.Pair.Lock.Lock()
		if order.Pair.openOrders.Contains(order) {
			order.Pair.openOrders.Remove(order)
		}
		order.Pair.Lock.Unlock()
	}
	if !oldState.IsFinal() && newState.IsFinal() {
		close(order.onClose)
		os.lock.Lock()
		if !os.openOrders.Contains(order) {
			os.openOrders.Remove(order)
		}
		os.lock.Unlock()
	}
	os.OnOrderEvent(order)
}

func compareOrdersByClientOrderID(a, b *Order) int {
	return strings.Compare(a.ClientOrderID, b.ClientOrderID)
}
