package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"log"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

type Exchange struct {
	Lock          sync.RWMutex
	Exchange      ds.Exchange
	Holdings      *Holdings
	Orders        *Orders
	Equities      *EquityRegistry
	FeeCalculator *AlpacaEliteFees // fee calculator for simulated fills
	OnReady       func()

	// Account balances (like Alpaca dashboard)
	// Buying Power
	RegTBuyingPower       decimal.Decimal // 2x overnight buying power
	DayTradingBuyingPower decimal.Decimal // 4x intraday buying power (PDT accounts)

	// Margin
	InitialMargin     decimal.Decimal // margin requirement to open positions
	MaintenanceMargin decimal.Decimal // margin requirement to hold positions

	// Cash
	Cash decimal.Decimal // actual cash (can be negative when using margin)

	// Positions
	Equity          decimal.Decimal // total account value (cash + positions)
	LongMarketValue decimal.Decimal // value of long positions

	// Fees
	Fees        decimal.Decimal // total fees paid
	AccruedFees decimal.Decimal // fees not yet settled

	// Day trading
	DayTradeCount int // PDT rule tracking
}

func newExchange(exchange ds.Exchange) *Exchange {
	if gRunning {
		loggy.Fatalf("cannot create new exchange %s while cubby is running", exchange)
	}
	ex := &Exchange{
		Exchange:      exchange,
		FeeCalculator: NewAlpacaEliteFees(),
		OnReady:       func() {},
	}
	ex.Holdings = newHoldings(ex)
	ex.Orders = newOrders(ex)
	ex.Equities = newEquityRegistry(ex)
	if Live {
		ex.fetchAlpacaAccount()
	}
	return ex
}

func (ex *Exchange) fetchAlpacaAccount() {
	account, err := AlpacaClient.GetAccount()
	if err != nil {
		loggy.Fatalf("alpaca: error fetching account: %v", err)
	}
	ex.Lock.Lock()
	ex.Cash.Store(decimal.Parse(account.Cash))
	ex.RegTBuyingPower.Store(decimal.Parse(account.RegTBuyingPower))
	ex.DayTradingBuyingPower.Store(decimal.Parse(account.DayTradingBuyingPower))
	ex.Equity.Store(decimal.Parse(account.Equity))
	ex.LongMarketValue.Store(decimal.Parse(account.LongMarketValue))
	ex.InitialMargin.Store(decimal.Parse(account.InitialMargin))
	ex.MaintenanceMargin.Store(decimal.Parse(account.MaintenanceMargin))
	ex.AccruedFees.Store(decimal.Parse(account.AccruedFees))
	ex.Lock.Unlock()
}

func (ex *Exchange) String() string {
	return ex.Exchange.String()
}

func (ex *Exchange) Now() clocky.Time {
	return clocky.Now()
}

func (ex *Exchange) run() {
	// Live trading daemons would go here
}

func compareExchanges(a, b *Exchange) int {
	if a.Exchange < b.Exchange {
		return -1
	}
	if a.Exchange > b.Exchange {
		return +1
	}
	return 0
}

// exchanges is a registry of all exchanges.
type exchanges struct {
	OnReady       func()
	lock          sync.RWMutex
	exchangeMap   map[ds.Exchange]*Exchange
	exchangeArray []*Exchange
	unready       *treeset.Set[*Exchange]
}

var Exchanges = &exchanges{
	OnReady:       func() {},
	exchangeMap:   make(map[ds.Exchange]*Exchange),
	exchangeArray: make([]*Exchange, 0),
	unready:       treeset.NewWith(compareExchanges),
}

func (es *exchanges) Get(exchange ds.Exchange) *Exchange {
	es.lock.RLock()
	ex, ok := es.exchangeMap[exchange]
	es.lock.RUnlock()
	if !ok {
		es.lock.Lock()
		ex, ok = es.exchangeMap[exchange]
		if !ok {
			ex = newExchange(exchange)
			es.unready.Add(ex)
			es.exchangeMap[exchange] = ex
			es.exchangeArray = append(es.exchangeArray, ex)
		}
		es.lock.Unlock()
	}
	return ex
}

func (es *exchanges) All() []*Exchange {
	es.lock.RLock()
	defer es.lock.RUnlock()
	result := make([]*Exchange, 0, len(es.exchangeArray))
	for _, exchange := range es.exchangeArray {
		result = append(result, exchange)
	}
	return result
}

func (es *exchanges) markReady(exchange *Exchange) {
	es.lock.Lock()
	es.unready.Remove(exchange)
	isReady := es.unready.Empty()
	es.lock.Unlock()
	if isReady {
		if *flagVerbose {
			log.Printf("[cubby] all exchanges ready")
		}
		es.OnReady()
	}
}

// Holdings is a registry of all holdings for an exchange.
type Holdings struct {
	lock          sync.RWMutex
	exchange      *Exchange
	holdingsMap   map[string]*Holding
	holdingsArray []*Holding
}

func newHoldings(exchange *Exchange) *Holdings {
	return &Holdings{
		exchange:      exchange,
		holdingsMap:   make(map[string]*Holding),
		holdingsArray: make([]*Holding, 0),
	}
}

func (hs *Holdings) Lookup(symbol string) *Holding {
	hs.lock.RLock()
	ho := hs.holdingsMap[symbol]
	hs.lock.RUnlock()
	return ho
}

func (hs *Holdings) Get(symbol string) *Holding {
	ho := hs.Lookup(symbol)
	if ho == nil {
		hs.lock.Lock()
		ho = hs.holdingsMap[symbol]
		if ho == nil {
			ho = newHolding(hs.exchange, symbol)
			hs.holdingsMap[symbol] = ho
			hs.holdingsArray = append(hs.holdingsArray, ho)
		}
		hs.lock.Unlock()
	}
	return ho
}

func (hs *Holdings) All() []*Holding {
	hs.lock.RLock()
	defer hs.lock.RUnlock()
	result := make([]*Holding, 0, len(hs.holdingsArray))
	for _, holding := range hs.holdingsArray {
		result = append(result, holding)
	}
	return result
}

func (hs *Holdings) GetEquityUSD() decimal.Decimal {
	// Start with cash balance
	total := hs.exchange.Cash.Load()
	// Add value of all stock positions
	for _, holding := range hs.All() {
		price := holding.Exchange.Equities.GetPriceUSD(holding.Symbol)
		holding.Lock.RLock()
		value := holding.Quantity.Load().Mul(price)
		holding.Lock.RUnlock()
		total = total.Add(value)
	}
	return total
}

func (hs *Holdings) GetInvestedUSD() decimal.Decimal {
	total := decimal.Zero
	for _, holding := range hs.All() {
		price := holding.Exchange.Equities.GetPriceUSD(holding.Symbol)
		holding.Lock.RLock()
		value := holding.Quantity.Load().Mul(price)
		holding.Lock.RUnlock()
		total = total.Add(value)
	}
	return total
}

// EquityRegistry is a registry of all tradeable equities for an exchange.
type EquityRegistry struct {
	lock        sync.RWMutex
	exchange    *Exchange
	equityMap   map[string]*Equity
	unready     *treeset.Set[*Equity]
	equityArray []*Equity
}

func newEquityRegistry(exchange *Exchange) *EquityRegistry {
	return &EquityRegistry{
		exchange:    exchange,
		equityMap:   make(map[string]*Equity),
		equityArray: make([]*Equity, 0),
		unready:     treeset.NewWith(compareEquities),
	}
}

func (er *EquityRegistry) Lookup(symbol string) *Equity {
	er.lock.RLock()
	eq := er.equityMap[symbol]
	er.lock.RUnlock()
	return eq
}

func (er *EquityRegistry) Get(symbol string) *Equity {
	eq := er.Lookup(symbol)
	if eq == nil {
		er.lock.Lock()
		eq = er.equityMap[symbol]
		if eq == nil {
			eq = newEquity(er.exchange, symbol)
			er.unready.Add(eq)
			er.equityMap[symbol] = eq
			er.equityArray = append(er.equityArray, eq)
		}
		er.lock.Unlock()
	}
	return eq
}

func (er *EquityRegistry) All() []*Equity {
	er.lock.RLock()
	defer er.lock.RUnlock()
	result := make([]*Equity, 0, len(er.equityArray))
	for _, eq := range er.equityArray {
		result = append(result, eq)
	}
	return result
}

func (er *EquityRegistry) GetPriceUSD(symbol string) decimal.Decimal {
	if symbol == "USD" {
		return decimal.One
	}
	er.lock.RLock()
	eq := er.equityMap[symbol]
	er.lock.RUnlock()
	if eq == nil {
		loggy.Fatalf("don't know how to determine USD value of %s", symbol)
	}
	return eq.LastPrice.Load()
}

func (er *EquityRegistry) markReady(eq *Equity) {
	er.lock.Lock()
	er.unready.Remove(eq)
	isReady := er.unready.Empty()
	er.lock.Unlock()
	if isReady {
		if *flagVerbose {
			log.Printf("[cubby] %s exchange ready", er.exchange)
		}
		er.exchange.OnReady()
		Exchanges.markReady(er.exchange)
	}
}

// Orders is a registry of all orders for an exchange.
type Orders struct {
	Exchange     *Exchange
	lock         sync.RWMutex
	ordersArray  []*Order
	ordersMap    map[string]*Order
	openOrders   *treeset.Set[*Order]
	OnOrderEvent atomic.Pointer[func(*Order)]
}

func newOrders(ex *Exchange) *Orders {
	return &Orders{
		Exchange:    ex,
		ordersArray: make([]*Order, 0),
		ordersMap:   make(map[string]*Order),
		openOrders:  treeset.NewWith(compareOrdersByClientOrderID),
	}
}

func (os *Orders) Get(ID string) *Order {
	os.lock.RLock()
	defer os.lock.RUnlock()
	return os.ordersMap[ID]
}

func (os *Orders) All() []*Order {
	os.lock.RLock()
	defer os.lock.RUnlock()
	result := make([]*Order, 0, len(os.ordersArray))
	for _, order := range os.ordersArray {
		result = append(result, order)
	}
	return result
}

func (os *Orders) Open() []*Order {
	os.lock.RLock()
	defer os.lock.RUnlock()
	result := make([]*Order, 0)
	for it := os.openOrders.Iterator(); it.Next(); {
		result = append(result, it.Value())
	}
	return result
}

func (os *Orders) Add(order *Order) {
	os.ordersMap[order.ClientOrderID] = order
	os.ordersArray = append(os.ordersArray, order)
	os.openOrders.Add(order)
	if onOrderEvent := os.OnOrderEvent.Load(); onOrderEvent != nil {
		(*onOrderEvent)(order)
	}
}

func compareOrdersByClientOrderID(a, b *Order) int {
	return strings.Compare(a.ClientOrderID, b.ClientOrderID)
}
