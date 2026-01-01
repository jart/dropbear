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

type Broker struct {
	Lock          sync.RWMutex
	Broker        ds.Broker
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

func newBroker(broker ds.Broker) *Broker {
	if gRunning {
		loggy.Fatalf("cannot create new broker %s while cubby is running", broker)
	}
	b := &Broker{
		Broker:           broker,
		FeeCalculator:    NewAlpacaEliteFees(),
		MarginInterest:   NewMarginInterest(100), // 100 bps spread over RFR (Alpaca ~5.75%)
		OnReady:          func() {},
		PatternDayTrader: true, // default to PDT mode
		pdtMinEquity:     decimal.FromInt(25000),
	}
	b.Holdings = newHoldings(b)
	b.Orders = newOrders(b)
	b.Equities = newEquityRegistry(b)
	if Live {
		b.fetchAlpacaAccount()
	}
	return b
}

// CalculateTotalMaintenanceMargin calculates the total maintenance margin
// required for all current positions.
func (b *Broker) CalculateTotalMaintenanceMargin() decimal.Decimal {
	var total decimal.Decimal
	for _, holding := range b.Holdings.All() {
		holding.Lock.RLock()
		qty := holding.Quantity.Load()
		holding.Lock.RUnlock()
		if qty.IsZero() {
			continue
		}
		price := b.Equities.GetPriceUSD(holding.Symbol)
		margin := MaintenanceMargin(holding.Symbol, qty, price)
		total = total.Add(margin)
	}
	return total
}

// LockDTBP should be called at end of day to lock the DTBP for the next trading day.
// Formula: Day Trade Buying Power = (EOD Equity − Maintenance Margin) × 4
func (b *Broker) LockDTBP() {
	b.Lock.Lock()
	defer b.Lock.Unlock()

	equity := b.Holdings.GetEquityUSD()
	maintMargin := b.CalculateTotalMaintenanceMargin()
	b.LastEquity = equity.Sub(maintMargin).Max(decimal.Zero)

	if *flagVerbose {
		log.Printf("[dtbp] EOD locked - Equity: $%s, Maintenance: $%s, Available Next Day: $%s",
			equity.Format(2), maintMargin.Format(2), b.LastEquity.Format(2))
	}
}

// InitDTBP should be called at start of day to initialize day trading buying power.
// PDT accounts with $25k+ equity get 4x leverage; otherwise 2x.
func (b *Broker) InitDTBP() {
	b.Lock.Lock()
	defer b.Lock.Unlock()

	equity := b.Holdings.GetEquityUSD()

	// If LastEquity was never set (first day), use current equity
	if b.LastEquity.IsZero() {
		b.LastEquity = equity
	}

	// PDT status requires $25k minimum equity
	if b.PatternDayTrader && equity.Cmp(b.pdtMinEquity) >= 0 {
		b.BodDTBP = b.LastEquity.MulInt(4)
		b.IsDayTradingTime = true
	} else {
		b.BodDTBP = b.LastEquity.MulInt(2)
		b.IsDayTradingTime = false
	}

	// Also set the current DayTradingBuyingPower to BodDTBP
	b.DayTradingBuyingPower.Store(b.BodDTBP)
	b.RegTBuyingPower.Store(b.LastEquity.MulInt(2))

	if *flagVerbose {
		log.Printf("[dtbp] BOD init - LastEquity: $%s, BodDTBP: $%s, PDT: %v, IsDayTradingTime: %v",
			b.LastEquity.Format(2), b.BodDTBP.Format(2), b.PatternDayTrader, b.IsDayTradingTime)
	}
}

// EndDayTradingTime should be called 10 minutes before market close.
// After this, only 2x leverage is available for new positions.
func (b *Broker) EndDayTradingTime() {
	b.Lock.Lock()
	defer b.Lock.Unlock()
	b.IsDayTradingTime = false

	if *flagVerbose {
		log.Printf("[dtbp] Day trading time ended - switching to 2x leverage only")
	}
}

// GetAvailableBuyingPower returns the buying power available for new positions,
// accounting for whether we're in day trading time or not.
func (b *Broker) GetAvailableBuyingPower() decimal.Decimal {
	b.Lock.RLock()
	defer b.Lock.RUnlock()

	if b.IsDayTradingTime {
		return b.DayTradingBuyingPower.Load()
	}
	return b.RegTBuyingPower.Load()
}

// GetMarginUsedByOpenOrders calculates the margin reserved by all open orders
// that would increase position size (opening orders).
func (b *Broker) GetMarginUsedByOpenOrders() decimal.Decimal {
	var total decimal.Decimal
	for _, order := range b.Orders.Open() {
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
func (b *Broker) GetNetAvailableBuyingPower() decimal.Decimal {
	available := b.GetAvailableBuyingPower()
	reserved := b.GetMarginUsedByOpenOrders()
	return available.Sub(reserved).Max(decimal.Zero)
}

// GetLeverageMultiplier returns the current leverage multiplier (4x or 2x).
func (b *Broker) GetLeverageMultiplier() int {
	b.Lock.RLock()
	defer b.Lock.RUnlock()

	if b.IsDayTradingTime && b.PatternDayTrader {
		return 4
	}
	return 2
}

// CheckMarginCall checks if account equity is below maintenance margin and
// triggers auto-liquidation if needed. Returns true if margin call was triggered.
func (b *Broker) CheckMarginCall() bool {
	equity := b.Holdings.GetEquityUSD()
	maintMargin := b.CalculateTotalMaintenanceMargin()

	// No margin call if equity is above maintenance
	if equity.Cmp(maintMargin) > 0 {
		if b.MarginCallTriggered {
			// Margin call was resolved
			b.Lock.Lock()
			b.MarginCallTriggered = false
			b.Lock.Unlock()
			if *flagVerbose {
				log.Printf("[margin] Margin call resolved - Equity: $%s, Maintenance: $%s",
					equity.Format(2), maintMargin.Format(2))
			}
		}
		return false
	}

	// Margin call triggered - need to liquidate
	b.Lock.Lock()
	if !b.MarginCallTriggered {
		b.MarginCallTriggered = true
		b.MarginCallTime = clocky.Now()
		b.MarginCallCount++
		log.Printf("[MARGIN CALL] Equity $%s < Maintenance $%s - AUTO-LIQUIDATING",
			equity.Format(2), maintMargin.Format(2))
	}
	b.Lock.Unlock()

	// Calculate deficit: how much we need to sell to restore margin + 10% buffer
	buffer := equity.MulInt(10).DivInt(100) // 10% buffer
	deficit := maintMargin.Sub(equity).Add(buffer)

	if deficit.IsPositive() {
		b.AutoLiquidate(deficit)
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
func (b *Broker) AutoLiquidate(deficit decimal.Decimal) {
	if !deficit.IsPositive() {
		return
	}

	// Collect all non-zero positions with P&L
	var positions []*positionWithPnL
	for _, holding := range b.Holdings.All() {
		holding.Lock.RLock()
		qty := holding.Quantity.Load()
		holding.Lock.RUnlock()

		if qty.IsZero() {
			continue
		}

		eq := b.Equities.Lookup(holding.Symbol)
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
		liquidateQty := liquidateValue.Div(price).Truncate()

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
			liquidated = b.forceSellLong(pos.equity, pos.holding, liquidateQty)
		} else {
			// Liquidate short position (cover)
			liquidated = b.forceCoverShort(pos.equity, pos.holding, liquidateQty)
		}

		remaining = remaining.Sub(liquidated)
		b.Lock.Lock()
		b.LiquidatedValue = b.LiquidatedValue.Add(liquidated)
		b.Lock.Unlock()

		log.Printf("[MARGIN CALL] Liquidated %s %s @ $%s = $%s (P&L: $%s)",
			liquidateQty.String(), pos.holding.Symbol, price.Format(2),
			liquidated.Format(2), pos.pnl.Format(2))
	}
}

// forceSellLong executes a forced liquidation sell for a long position.
// Returns the notional value of the liquidation.
func (b *Broker) forceSellLong(eq *Equity, holding *Holding, qty decimal.Decimal) decimal.Decimal {
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
	fee := eq.Broker.FeeCalculator.Calculate(clocky.Now(), qty.Int(), true)

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
	b.Lock.Lock()
	add(&b.Cash, proceeds)
	add(&b.DayTradingBuyingPower, proceeds)
	b.Fees = b.Fees.Add(fee)
	b.Lock.Unlock()

	return notional
}

// forceCoverShort executes a forced cover for a short position.
// Returns the notional value of the cover.
func (b *Broker) forceCoverShort(eq *Equity, holding *Holding, qty decimal.Decimal) decimal.Decimal {
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
	fee := eq.Broker.FeeCalculator.Calculate(clocky.Now(), qty.Int(), true)

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
	b.Lock.Lock()
	sub(&b.Cash, cost)
	b.Fees = b.Fees.Add(fee)
	b.Lock.Unlock()

	return notional
}

func (b *Broker) fetchAlpacaAccount() {
	account, err := AlpacaClient.GetAccount()
	if err != nil {
		loggy.Fatalf("alpaca: error fetching account: %v", err)
	}
	b.Lock.Lock()
	b.Cash.Store(decimal.Parse(account.Cash))
	b.RegTBuyingPower.Store(decimal.Parse(account.RegTBuyingPower))
	b.DayTradingBuyingPower.Store(decimal.Parse(account.DayTradingBuyingPower))
	b.Equity.Store(decimal.Parse(account.Equity))
	b.LongMarketValue.Store(decimal.Parse(account.LongMarketValue))
	b.InitialMargin.Store(decimal.Parse(account.InitialMargin))
	b.MaintenanceMargin.Store(decimal.Parse(account.MaintenanceMargin))
	b.AccruedFees.Store(decimal.Parse(account.AccruedFees))
	b.Lock.Unlock()
}

func (b *Broker) String() string {
	return b.Broker.String()
}

func (b *Broker) Now() clocky.Time {
	return clocky.Now()
}

func (b *Broker) run() {
	// Live trading daemons would go here
}

func compareBrokers(a, b *Broker) int {
	if a.Broker < b.Broker {
		return -1
	}
	if a.Broker > b.Broker {
		return +1
	}
	return 0
}

// brokers is a registry of all brokers.
type brokers struct {
	OnReady     func()
	lock        sync.RWMutex
	brokerMap   map[ds.Broker]*Broker
	brokerArray []*Broker
	unready     *treeset.Set[*Broker]
}

var Brokers = &brokers{
	OnReady:     func() {},
	brokerMap:   make(map[ds.Broker]*Broker),
	brokerArray: make([]*Broker, 0),
	unready:     treeset.NewWith(compareBrokers),
}

func (bs *brokers) Get(broker ds.Broker) *Broker {
	bs.lock.RLock()
	b, ok := bs.brokerMap[broker]
	bs.lock.RUnlock()
	if !ok {
		bs.lock.Lock()
		b, ok = bs.brokerMap[broker]
		if !ok {
			b = newBroker(broker)
			bs.unready.Add(b)
			bs.brokerMap[broker] = b
			bs.brokerArray = append(bs.brokerArray, b)
		}
		bs.lock.Unlock()
	}
	return b
}

func (bs *brokers) All() []*Broker {
	bs.lock.RLock()
	defer bs.lock.RUnlock()
	result := make([]*Broker, 0, len(bs.brokerArray))
	for _, broker := range bs.brokerArray {
		result = append(result, broker)
	}
	return result
}

func (bs *brokers) markReady(broker *Broker) {
	bs.lock.Lock()
	bs.unready.Remove(broker)
	isReady := bs.unready.Empty()
	bs.lock.Unlock()
	if isReady {
		if *flagVerbose {
			log.Printf("[cubby] all brokers ready")
		}
		bs.OnReady()
	}
}

// Holdings is a registry of all holdings for a broker.
type Holdings struct {
	lock          sync.RWMutex
	broker        *Broker
	holdingsMap   map[string]*Holding
	holdingsArray []*Holding
}

func newHoldings(broker *Broker) *Holdings {
	return &Holdings{
		broker:        broker,
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
			ho = newHolding(hs.broker, symbol)
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
	total := hs.broker.Cash.Load()
	// Add value of all stock positions
	for _, holding := range hs.All() {
		price := holding.Broker.Equities.GetPriceUSD(holding.Symbol)
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
		price := holding.Broker.Equities.GetPriceUSD(holding.Symbol)
		holding.Lock.RLock()
		value := holding.Quantity.Load().Mul(price)
		holding.Lock.RUnlock()
		total = total.Add(value)
	}
	return total
}

// EquityRegistry is a registry of all tradeable equities for a broker.
type EquityRegistry struct {
	lock        sync.RWMutex
	broker      *Broker
	equityMap   map[string]*Equity
	unready     *treeset.Set[*Equity]
	equityArray []*Equity
}

func newEquityRegistry(broker *Broker) *EquityRegistry {
	return &EquityRegistry{
		broker:      broker,
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
			eq = newEquity(er.broker, symbol)
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
			log.Printf("[cubby] %s broker ready", er.broker)
		}
		er.broker.OnReady()
		Brokers.markReady(er.broker)
	}
}

// Orders is a registry of all orders for a broker.
type Orders struct {
	Broker       *Broker
	lock         sync.RWMutex
	ordersArray  []*Order
	ordersMap    map[string]*Order
	openOrders   *treeset.Set[*Order]
	OnOrderEvent atomic.Pointer[func(*Order)]
}

func newOrders(b *Broker) *Orders {
	return &Orders{
		Broker:      b,
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
