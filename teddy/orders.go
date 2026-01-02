package teddy

import (
	"dropbear/broker/coinbase"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

type Orders struct {
	Broker       *Broker
	lock         sync.RWMutex
	ordersArray  []*Order
	ordersMap    map[string]*Order // by clientOrderID or orderID
	openOrders   *treeset.Set[*Order]
	OnOrderEvent atomic.Pointer[func(*Order)]
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

func newOrders(ex *Broker) *Orders {
	return &Orders{
		Broker:      ex,
		ordersArray: make([]*Order, 0),
		ordersMap:   make(map[string]*Order),
		openOrders:  treeset.NewWith(compareOrdersByClientOrderID),
	}
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
	func() {
		pair.Lock.Lock()
		defer pair.Lock.Unlock()
		pair.openOrders.Add(order)
	}()
	if onOrderEvent := os.OnOrderEvent.Load(); onOrderEvent != nil {
		(*onOrderEvent)(order)
	}
	return order
}

func (os *Orders) coinbaseOrderUpdateDaemon() {
	for orderUpdate := range coinbase.OrderUpdates() {
		os.onCoinbaseOrderUpdate(orderUpdate)
	}
}

func (os *Orders) onCoinbaseOrderUpdate(orderUpdate *coinbase.OrderUpdate) {
	// find the order in our map
	os.lock.Lock()
	order, exists := os.ordersMap[orderUpdate.OrderID]
	if !exists {
		order, exists = os.ordersMap[orderUpdate.ClientOrderID]
		if !exists {
			// order not in our map - ignore (could be from another session)
			os.lock.Unlock()
			return
		}
		order.OrderID = orderUpdate.OrderID
		os.ordersMap[orderUpdate.OrderID] = order
	}
	os.lock.Unlock()

	// parse coinbase values
	cbFilled := decimal.Parse(orderUpdate.CumulativeQuantity)
	cbValue := decimal.Parse(orderUpdate.FilledValue)
	cbFee := decimal.Parse(orderUpdate.TotalFees)
	cbQuantity := decimal.Parse(orderUpdate.LeavesQuantity).Add(cbFilled)
	cbState := ds.NewOrderStateForCoinbase(orderUpdate.Status, cbFilled)

	// compute deltas from our tracked state
	order.Lock.Lock()
	oldFilled := order.Filled.Load()
	oldValue := order.Notional.Load()
	oldState := order.State.Load()
	oldFees := order.lastFees.Load()
	feeDelta := cbFee.Sub(oldFees)
	fillDelta := cbFilled.Sub(oldFilled)
	valueDelta := cbValue.Sub(oldValue)
	feeRate := decimal.Zero
	if valueDelta.IsPositive() {
		feeRate = feeDelta.Div(valueDelta)
	}
	order.lastFees.Store(cbFee)
	order.Lock.Unlock()

	// ignore stale out-of-order updates
	if fillDelta.IsNegative() || valueDelta.IsNegative() {
		return
	}

	// update order metadata that coinbase controls
	order.Lock.Lock()
	order.CreatedTime.Store(clocky.MustParseTime(orderUpdate.CreationTime))
	order.Quantity.Store(cbQuantity) // coinbase may adjust quantity for market orders
	order.LimitPrice.Store(decimal.Parse(orderUpdate.LimitPrice))
	if !oldState.IsFinal() && !cbState.IsFinal() {
		order.State.Store(cbState)
	}
	order.Lock.Unlock()

	// process fill
	if fillDelta.IsPositive() && valueDelta.IsPositive() {
		_, _ = order.fill(fillDelta, valueDelta, feeRate, true)
	} else if cbState.IsFinal() && !oldState.IsFinal() {
		// order ended without new fills (cancelled, expired, etc.)
		order.kill(cbState)
	}
}

func compareOrdersByClientOrderID(a, b *Order) int {
	return strings.Compare(a.ClientOrderID, b.ClientOrderID)
}
