package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"log"
	"sort"
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

	// Margin Interest (charged on negative cash at EOD)
	MarginInterest *MarginInterest

	// Day trading
	DayTradeCount int // PDT rule tracking

	// DTBP (Day Trading Buying Power) lifecycle
	// See AlpacaMargin.cs for reference implementation
	LastEquity       decimal.Decimal // Equity at EOD (locked, determines next day's DTBP)
	BodDTBP          decimal.Decimal // Beginning-of-day DTBP (set from LastEquity)
	IsDayTradingTime bool            // true from market open until 10 min before close
	PatternDayTrader bool            // PDT flag - enables 4x intraday leverage

	// PDT threshold
	pdtMinEquity decimal.Decimal // $25,000 minimum for PDT status

	// Margin call state
	MarginCallTriggered bool            // true if margin call is in progress
	MarginCallTime      clocky.Time     // when margin call was triggered
	LiquidatedValue     decimal.Decimal // total value liquidated due to margin calls
	MarginCallCount     int             // number of margin calls triggered
}

func newExchange(exchange ds.Exchange) *Exchange {
	if gRunning {
		loggy.Fatalf("cannot create new exchange %s while cubby is running", exchange)
	}
	ex := &Exchange{
		Exchange:         exchange,
		FeeCalculator:    NewAlpacaEliteFees(),
		MarginInterest:   NewMarginInterest(100), // 100 bps spread over RFR (Alpaca ~5.75%)
		OnReady:          func() {},
		PatternDayTrader: true, // default to PDT mode
		pdtMinEquity:     decimal.FromInt(25000),
	}
	ex.Holdings = newHoldings(ex)
	ex.Orders = newOrders(ex)
	ex.Equities = newEquityRegistry(ex)
	if Live {
		ex.fetchAlpacaAccount()
	}
	return ex
}

// CalculateTotalMaintenanceMargin calculates the total maintenance margin
// required for all current positions.
func (ex *Exchange) CalculateTotalMaintenanceMargin() decimal.Decimal {
	var total decimal.Decimal
	for _, holding := range ex.Holdings.All() {
		holding.Lock.RLock()
		qty := holding.Quantity.Load()
		holding.Lock.RUnlock()
		if qty.IsZero() {
			continue
		}
		price := ex.Equities.GetPriceUSD(holding.Symbol)
		margin := MaintenanceMargin(holding.Symbol, qty, price)
		total = total.Add(margin)
	}
	return total
}

// LockDTBP should be called at end of day to lock the DTBP for the next trading day.
// Formula: Day Trade Buying Power = (EOD Equity − Maintenance Margin) × 4
func (ex *Exchange) LockDTBP() {
	ex.Lock.Lock()
	defer ex.Lock.Unlock()

	equity := ex.Holdings.GetEquityUSD()
	maintMargin := ex.CalculateTotalMaintenanceMargin()
	ex.LastEquity = equity.Sub(maintMargin).Max(decimal.Zero)

	if *flagVerbose {
		log.Printf("[dtbp] EOD locked - Equity: $%s, Maintenance: $%s, Available Next Day: $%s",
			equity.Format(2), maintMargin.Format(2), ex.LastEquity.Format(2))
	}
}

// InitDTBP should be called at start of day to initialize day trading buying power.
// PDT accounts with $25k+ equity get 4x leverage; otherwise 2x.
func (ex *Exchange) InitDTBP() {
	ex.Lock.Lock()
	defer ex.Lock.Unlock()

	equity := ex.Holdings.GetEquityUSD()

	// If LastEquity was never set (first day), use current equity
	if ex.LastEquity.IsZero() {
		ex.LastEquity = equity
	}

	// PDT status requires $25k minimum equity
	if ex.PatternDayTrader && equity.Cmp(ex.pdtMinEquity) >= 0 {
		ex.BodDTBP = ex.LastEquity.MulInt(4)
		ex.IsDayTradingTime = true
	} else {
		ex.BodDTBP = ex.LastEquity.MulInt(2)
		ex.IsDayTradingTime = false
	}

	// Also set the current DayTradingBuyingPower to BodDTBP
	ex.DayTradingBuyingPower.Store(ex.BodDTBP)
	ex.RegTBuyingPower.Store(ex.LastEquity.MulInt(2))

	if *flagVerbose {
		log.Printf("[dtbp] BOD init - LastEquity: $%s, BodDTBP: $%s, PDT: %v, IsDayTradingTime: %v",
			ex.LastEquity.Format(2), ex.BodDTBP.Format(2), ex.PatternDayTrader, ex.IsDayTradingTime)
	}
}

// EndDayTradingTime should be called 10 minutes before market close.
// After this, only 2x leverage is available for new positions.
func (ex *Exchange) EndDayTradingTime() {
	ex.Lock.Lock()
	defer ex.Lock.Unlock()
	ex.IsDayTradingTime = false

	if *flagVerbose {
		log.Printf("[dtbp] Day trading time ended - switching to 2x leverage only")
	}
}

// GetAvailableBuyingPower returns the buying power available for new positions,
// accounting for whether we're in day trading time or not.
func (ex *Exchange) GetAvailableBuyingPower() decimal.Decimal {
	ex.Lock.RLock()
	defer ex.Lock.RUnlock()

	if ex.IsDayTradingTime {
		return ex.DayTradingBuyingPower.Load()
	}
	return ex.RegTBuyingPower.Load()
}

// GetMarginUsedByOpenOrders calculates the margin reserved by all open orders
// that would increase position size (opening orders).
func (ex *Exchange) GetMarginUsedByOpenOrders() decimal.Decimal {
	var total decimal.Decimal
	for _, order := range ex.Orders.Open() {
		order.Lock.RLock()
		if order.State.Load().IsFinal() {
			order.Lock.RUnlock()
			continue
		}
		orderType := order.Type
		side := order.Side
		isShortSale := order.IsShortSale
		isCover := order.IsCover
		qty := order.Quantity.Load()
		limitPrice := order.LimitPrice.Load()
		eq := order.Equity
		order.Lock.RUnlock()

		// Skip if this is a closing order (sell existing long or cover existing short)
		if !isOpeningOrder(side, isShortSale, isCover, eq) {
			continue
		}

		// Calculate margin required for this order
		var price decimal.Decimal
		switch orderType {
		case ds.OrderTypeLimit, ds.OrderTypeLOC, ds.OrderTypeLOO:
			price = limitPrice
		default:
			// For market orders, use last price as estimate
			price = eq.LastPrice.Load()
		}

		if !price.IsPositive() {
			continue
		}

		margin := InitialMargin(eq.Symbol, qty, price)
		total = total.Add(margin)
	}
	return total
}

// isOpeningOrder returns true if this order would increase position size
// (i.e., open a new position or add to existing position in same direction).
func isOpeningOrder(side ds.Side, isShortSale, isCover bool, eq *Equity) bool {
	// Cover orders close short positions
	if isCover {
		return false
	}

	// Short sales always open/add to short position
	if isShortSale {
		return true
	}

	// For regular orders, check if it matches current position direction
	holding := eq.Shares
	holding.Lock.RLock()
	qty := holding.Quantity.Load()
	holding.Lock.RUnlock()

	switch side {
	case ds.SideBuy:
		// Buy is opening if no position or already long
		return qty.IsZero() || qty.IsPositive()
	case ds.SideSell:
		// Regular sell is closing if we have a long position
		return qty.IsZero() || qty.IsNegative()
	}
	return true
}

// GetNetAvailableBuyingPower returns buying power minus margin reserved by open orders.
func (ex *Exchange) GetNetAvailableBuyingPower() decimal.Decimal {
	available := ex.GetAvailableBuyingPower()
	reserved := ex.GetMarginUsedByOpenOrders()
	return available.Sub(reserved).Max(decimal.Zero)
}

// GetLeverageMultiplier returns the current leverage multiplier (4x or 2x).
func (ex *Exchange) GetLeverageMultiplier() int {
	ex.Lock.RLock()
	defer ex.Lock.RUnlock()

	if ex.IsDayTradingTime && ex.PatternDayTrader {
		return 4
	}
	return 2
}

// CheckMarginCall checks if account equity is below maintenance margin and
// triggers auto-liquidation if needed. Returns true if margin call was triggered.
func (ex *Exchange) CheckMarginCall() bool {
	equity := ex.Holdings.GetEquityUSD()
	maintMargin := ex.CalculateTotalMaintenanceMargin()

	// No margin call if equity is above maintenance
	if equity.Cmp(maintMargin) > 0 {
		if ex.MarginCallTriggered {
			// Margin call was resolved
			ex.Lock.Lock()
			ex.MarginCallTriggered = false
			ex.Lock.Unlock()
			if *flagVerbose {
				log.Printf("[margin] Margin call resolved - Equity: $%s, Maintenance: $%s",
					equity.Format(2), maintMargin.Format(2))
			}
		}
		return false
	}

	// Margin call triggered - need to liquidate
	ex.Lock.Lock()
	if !ex.MarginCallTriggered {
		ex.MarginCallTriggered = true
		ex.MarginCallTime = clocky.Now()
		ex.MarginCallCount++
		log.Printf("[MARGIN CALL] Equity $%s < Maintenance $%s - AUTO-LIQUIDATING",
			equity.Format(2), maintMargin.Format(2))
	}
	ex.Lock.Unlock()

	// Calculate deficit: how much we need to sell to restore margin + 10% buffer
	buffer := equity.MulInt(10).DivIntEven(100) // 10% buffer
	deficit := maintMargin.Sub(equity).Add(buffer)

	if deficit.IsPositive() {
		ex.AutoLiquidate(deficit)
	}

	return true
}

// positionWithPnL represents a position with its unrealized P&L for sorting.
type positionWithPnL struct {
	holding     *Holding
	equity      *Equity
	quantity    decimal.Decimal
	marketValue decimal.Decimal
	costBasis   decimal.Decimal
	pnl         decimal.Decimal
}

// AutoLiquidate sells positions to cover the deficit amount.
// Positions are sold in order of largest unrealized loss first.
func (ex *Exchange) AutoLiquidate(deficit decimal.Decimal) {
	if !deficit.IsPositive() {
		return
	}

	// Collect all non-zero positions with P&L
	var positions []*positionWithPnL
	for _, holding := range ex.Holdings.All() {
		holding.Lock.RLock()
		qty := holding.Quantity.Load()
		holding.Lock.RUnlock()

		if qty.IsZero() {
			continue
		}

		eq := ex.Equities.Lookup(holding.Symbol)
		if eq == nil {
			continue
		}

		price := eq.LastPrice.Load()
		if !price.IsPositive() {
			continue
		}

		marketValue := qty.Abs().Mul(price)
		var costBasis decimal.Decimal
		if qty.IsPositive() {
			costBasis = holding.Lots.Cost
		} else {
			costBasis = holding.ShortLots.Cost
		}

		var pnl decimal.Decimal
		if qty.IsPositive() {
			// Long position: profit = market value - cost basis
			pnl = marketValue.Sub(costBasis)
		} else {
			// Short position: profit = cost basis (proceeds) - market value (cost to cover)
			pnl = costBasis.Sub(marketValue)
		}

		positions = append(positions, &positionWithPnL{
			holding:     holding,
			equity:      eq,
			quantity:    qty,
			marketValue: marketValue,
			costBasis:   costBasis,
			pnl:         pnl,
		})
	}

	// Sort by P&L ascending (losers first)
	sort.Slice(positions, func(i, j int) bool {
		return positions[i].pnl.Cmp(positions[j].pnl) < 0
	})

	// Liquidate positions until deficit is covered
	remaining := deficit
	for _, pos := range positions {
		if !remaining.IsPositive() {
			break
		}

		// Determine how much to liquidate from this position
		liquidateValue := remaining.Min(pos.marketValue)
		price := pos.equity.LastPrice.Load()
		liquidateQty := liquidateValue.DivEven(price).Truncate()

		if liquidateQty.IsZero() {
			continue
		}

		// Cap at actual position size
		if liquidateQty.Cmp(pos.quantity.Abs()) > 0 {
			liquidateQty = pos.quantity.Abs()
		}

		// Execute force sell/cover
		var liquidated decimal.Decimal
		if pos.quantity.IsPositive() {
			// Liquidate long position
			liquidated = ex.forceSellLong(pos.equity, pos.holding, liquidateQty)
		} else {
			// Liquidate short position (cover)
			liquidated = ex.forceCoverShort(pos.equity, pos.holding, liquidateQty)
		}

		remaining = remaining.Sub(liquidated)
		ex.Lock.Lock()
		ex.LiquidatedValue = ex.LiquidatedValue.Add(liquidated)
		ex.Lock.Unlock()

		log.Printf("[MARGIN CALL] Liquidated %s %s @ $%s = $%s (P&L: $%s)",
			liquidateQty.String(), pos.holding.Symbol, price.Format(2),
			liquidated.Format(2), pos.pnl.Format(2))
	}
}

// forceSellLong executes a forced liquidation sell for a long position.
// Returns the notional value of the liquidation.
func (ex *Exchange) forceSellLong(eq *Equity, holding *Holding, qty decimal.Decimal) decimal.Decimal {
	holding.Lock.Lock()

	// Check current position - it may have changed since AutoLiquidate snapshot
	currentQty := holding.Quantity.Load()
	if !currentQty.IsPositive() {
		holding.Lock.Unlock()
		return decimal.Zero // Position already closed or is short
	}
	if qty.Cmp(currentQty) > 0 {
		qty = currentQty // Cap at available shares
	}

	price := eq.LastPrice.Load()
	notional := qty.Mul(price)
	// Forced liquidations use market orders
	fee := eq.Exchange.FeeCalculator.Calculate(clocky.Now(), qty.Int(), true)

	sub(&holding.Quantity, qty)
	sub(&holding.Available, qty.Min(holding.Available.Load()))
	holding.Volume += qty.Float64()
	holding.SellVolume += qty.Float64()
	costBasis := holding.Lots.Consume(qty, decimal.Zero)
	profit := notional.Sub(fee).Sub(costBasis)
	if profit.IsPositive() {
		holding.WinCount++
	} else if profit.IsNegative() {
		holding.LossCount++
	}
	holding.Lock.Unlock()

	// Add proceeds to cash
	proceeds := notional.Sub(fee)
	ex.Lock.Lock()
	add(&ex.Cash, proceeds)
	add(&ex.DayTradingBuyingPower, proceeds)
	ex.Fees = ex.Fees.Add(fee)
	ex.Lock.Unlock()

	return notional
}

// forceCoverShort executes a forced cover for a short position.
// Returns the notional value of the cover.
func (ex *Exchange) forceCoverShort(eq *Equity, holding *Holding, qty decimal.Decimal) decimal.Decimal {
	holding.Lock.Lock()

	// Check current position - it may have changed since AutoLiquidate snapshot
	currentQty := holding.Quantity.Load()
	if !currentQty.IsNegative() {
		holding.Lock.Unlock()
		return decimal.Zero // Position already closed or is long
	}
	absQty := currentQty.Neg() // Convert to positive for comparison
	if qty.Cmp(absQty) > 0 {
		qty = absQty // Cap at actual short position size
	}

	price := eq.LastPrice.Load()
	notional := qty.Mul(price)
	// Forced covers use market orders
	fee := eq.Exchange.FeeCalculator.Calculate(clocky.Now(), qty.Int(), true)

	add(&holding.Quantity, qty) // -100 + 50 = -50
	holding.Volume += qty.Float64()
	holding.BuyVolume += qty.Float64()
	shortProceeds := holding.ShortLots.Consume(qty, decimal.Zero)
	profit := shortProceeds.Sub(notional.Add(fee))
	if profit.IsPositive() {
		holding.WinCount++
	} else if profit.IsNegative() {
		holding.LossCount++
	}
	holding.Lock.Unlock()

	// Pay cash to cover
	cost := notional.Add(fee)
	ex.Lock.Lock()
	sub(&ex.Cash, cost)
	ex.Fees = ex.Fees.Add(fee)
	ex.Lock.Unlock()

	return notional
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
