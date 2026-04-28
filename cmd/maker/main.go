// symmetrical market making strategy
package main

import (
	"bytes"
	"io"
	"dropbear/broker/alpaca"
	"dropbear/broker/alpaca/sip"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"dropbear/netty"
	"dropbear/symbol"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"
)

var (
	flagCooldown  = clocky.DurationFlag("cooldown", "30m", "cooldown period after order rejection")
	flagBucket    = flag.String("bucket", "dropbear-sip", "google cloud storage bucket for recording market data")
	flagLive      = flag.Bool("live", false, "enables live trading and network access")
	flagExtended  = flag.Bool("extended", false, "enables extended hours trading")
	flagOvernight = flag.Bool("overnight", false, "enables overnight hours trading")
	flagData      = flag.String("data", "", "path of sip data file for backtest")
	flagLatency   = clocky.DurationFlag("latency", "7ms", "simulated order latency for backtest")
)

const (
	kHeartbeatInterval = clocky.Second
	kBalanceInterval   = 5 * clocky.Second
)

var (
	gBroker         Broker
	gOrders         map[string]*State
	gSymbols        map[symbol.Symbol]*State
	gIgnoreSymbols  map[symbol.Symbol]bool
	gFailedOrders   chan string
	gFailedReplaces chan string
	gNextBalance    clocky.Time
	gNextVolume     clocky.Time
	gTotalPnL       decimal.Decimal
	gTotalFees      decimal.Decimal
	gTotalShares    decimal.Decimal
	gTotalFills     int
	gOrderCount     int64
	gOrderFails     int64
	gTapeMsg        chan *sip.Message
	gTapeDone       chan struct{}
	gBacktest       *Backtest
	gAlpacaClient   *alpaca.Client
	gOrderSeq       int64
)

// defaultSymbols is the portfolio used for live trading.
// Live 2026-04-28: net=$126, 477 fills, 39900 shares.
var defaultSymbols = []SymbolEntry{
	// NASDAQ longs (~$102k notional at max position)
	{symbol.INTC, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.PYPL, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.CMCSA, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.SOFI, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("400"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},

	// NASDAQ shorts (~$101k notional at max position)
	{symbol.HOOD, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.DKNG, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-400"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.RIVN, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-500"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.AAL, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-700"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},

	// ARCA (pro-rata)
	{symbol.XLE, Config{
		venue:  alpaca.OrderDestinationARCA,
		target: decimal.Parse("300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.FXI, Config{
		venue:  alpaca.OrderDestinationARCA,
		target: decimal.Parse("-500"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},

	// NYSE (pro-rata)
	{symbol.VZ, Config{
		venue:  alpaca.OrderDestinationNYSE,
		target: decimal.Parse("400"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.NKE, Config{
		venue:  alpaca.OrderDestinationNYSE,
		target: decimal.Parse("-400"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
}

func main() {
	loggy.Init()
	flag.Parse()
	if *flagLive {
		loggy.AlsoLogToFile()
	} else {
		netty.SetOffline()
		clocky.Now = clocky.FakeNow
		clocky.Sleep = clocky.FakeSleep
		clocky.NewTicker = clocky.FakeNewTicker
	}

	if !*flagLive && *flagData == "" {
		fmt.Fprintf(os.Stderr, "-data must be specified for backtest mode\n")
		os.Exit(1)
	}

	result := Run(defaultSymbols)
	fmt.Printf("total P&L: %s  fees: %s  net: %s  fills: %d  shares: %s\n",
		result.PnL, result.Fees, result.Net, result.Fills, result.Shares)
	os.Exit(0)
}

func Run(symbols []SymbolEntry) Result {
	// capture log output (also write to stderr so tests can see it)
	var logBuf bytes.Buffer
	if !*flagLive {
		log.SetOutput(io.MultiWriter(&logBuf, os.Stderr))
		defer log.SetOutput(os.Stderr)
	}

	// initialize globals
	gOrders = map[string]*State{}
	gSymbols = map[symbol.Symbol]*State{}
	gIgnoreSymbols = map[symbol.Symbol]bool{}
	gFailedOrders = make(chan string, 32)
	gFailedReplaces = make(chan string, 32)
	gTotalPnL = decimal.Zero
	gTotalFees = decimal.Zero
	gTotalShares = decimal.Zero
	gTotalFills = 0
	gOrderCount = 0
	gOrderFails = 0
	gOrderSeq = 0
	gNextBalance = 0
	gBacktest = nil
	gTapeMsg = nil
	gTapeDone = nil
	if *flagLive {
		gAlpacaClient = alpaca.NewClient()
		gBroker = gAlpacaClient
	}

	for _, e := range symbols {
		addSymbol(e.Symbol, e.Config)
	}

	// log configuration
	log.Printf("prepare to make markets")
	for _, st := range gSymbols {
		log.Printf("  %s: target=%s qty=%s spread=%s drift=%s venue=%s",
			st.symbol, st.config.target, st.config.qty, st.config.spread, st.config.drift, st.config.venue)
	}

	// periodically fetch information about supported equities from alpaca
	if *flagLive {
		go synchronizeAssetsForever()
	}

	// cancel lingering orders
	if *flagLive {
		log.Printf("canceling lingering orders...")
		if err := gBroker.CancelAllOrders(); err != nil {
			log.Printf("warning: error canceling orders: %v", err)
		}
	}

	// load existing positions
	if *flagLive {
		var symbols []string
		log.Printf("loading positions...")
		positions, err := gBroker.GetPositions()
		if err != nil {
			log.Fatalf("error getting positions: %v", err)
		}
		for _, pos := range positions {
			sym, err := symbol.Parse(pos.Symbol)
			if err != nil {
				continue
			}
			symbols = append(symbols, pos.Symbol)
			st := getOrCreateSymbol(sym)
			st.position = pos.Qty
			st.costBasis = pos.CostBasis
			log.Printf("loaded position for %s: %s shares @ avg %s (cost basis %s)",
				pos.Symbol, pos.Qty, pos.AvgEntryPrice, pos.CostBasis)
		}
		// fetch quotes for existing positions and place exit orders
		if len(symbols) > 0 {
			feed := alpaca.FeedSIP
			if cboe.GetSession(clocky.Now()) == cboe.SessionOvernight {
				feed = alpaca.FeedBOATS
			}
			log.Printf("fetching %s quotes for %d positions...", feed, len(symbols))
			quotes, err := gBroker.GetQuotes(symbols, feed)
			if err != nil {
				log.Printf("warning: error fetching quotes: %v", err)
			} else {
				for name, q := range quotes {
					st := gSymbols[symbol.MustParse(name)]
					if st == nil || q.BidPrice.IsZero() || q.AskPrice.IsZero() {
						continue
					}
					st.quote = q
				}
			}
		}
	}

	// catch ctrl-c
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// the trading algorithm with a heart
	var heartbeatChan <-chan time.Time
	if *flagLive {
		heartbeat := clocky.NewTicker(kHeartbeatInterval)
		defer heartbeat.Stop()
		heartbeatChan = heartbeat.C
	}

	var stockUpdates <-chan *sip.Message
	var boatsUpdates <-chan *sip.Message
	var orderUpdates <-chan *alpaca.OrderUpdate
	if !*flagLive {
		var err error
		gBacktest, err = NewBacktest(*flagData, sigChan)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error starting backtest: %v\n", err)
			os.Exit(1)
		}
		orderUpdates = gBacktest.OrderUpdates
		stockUpdates = gBacktest.StockUpdates
		boatsUpdates = make(chan *sip.Message)
		heartbeatChan = gBacktest.Heartbeat
		gBroker = gBacktest
	} else {
		log.Printf("subscribing to order updates...")
		orderUpdates = alpaca.OrderUpdates()
		names := symbolNames()
		log.Printf("subscribing to sip stock updates for %d symbols...", len(names))
		stockUpdates = alpaca.MustStockUpdates(alpaca.SIPWSURL, &alpaca.StockUpdatesRequest{
			Action:     "subscribe",
			Quotes:     names,
			Trades:     names,
			Statuses:   names,
			Imbalances: names,
			LULDs:      names,
		})
		log.Printf("subscribing to boats stock updates for %d symbols...", len(names))
		boatsUpdates = alpaca.MustStockUpdates(alpaca.BOATSWSURL, &alpaca.StockUpdatesRequest{
			Action:   "subscribe",
			Quotes:   names,
			Trades:   names,
			Statuses: names,
		})
		// start tape recorder to gcs
		gTapeMsg = make(chan *sip.Message, 65536)
		gTapeDone = make(chan struct{})
		go recordTape(gTapeMsg, gTapeDone)
	}

	// consume events
	for {
		select {
		case msg := <-stockUpdates:
			onMessage(msg)
		case msg := <-boatsUpdates:
			onMessage(msg)
		case update := <-orderUpdates:
			onOrderUpdate(update)
		case clientOrderID := <-gFailedOrders:
			removeOrder(gOrders[clientOrderID], clientOrderID)
			gOrderFails++
		case clientOrderID := <-gFailedReplaces:
			delete(gOrders, clientOrderID)
		case <-heartbeatChan:
			onHeartbeat()
		case <-sigChan:
			result := shutdown()
			result.Log = logBuf.String()
			return result
		}
	}
}

func onHeartbeat() {
	now := clocky.Now()
	if now.After(gNextBalance) {
		logBalance()
		logSpread()
		gNextBalance = now.Add(kBalanceInterval)
	}
}

func logSpread() {
	for _, st := range gSymbols {
		if st.config.target.IsZero() || st.quote == nil {
			continue
		}
		q := st.quote
		if q.BidPrice.IsZero() || q.AskPrice.IsZero() {
			continue
		}
		unrealized := decimal.Zero
		if !st.position.IsZero() && !st.costBasis.IsZero() {
			mid := q.BidPrice.Add(q.AskPrice).Half()
			unrealized = mid.Mul(st.position).Sub(st.costBasis)
		}
		net := st.realizedPnL.Sub(st.totalFees)
		log.Printf("SPREAD %s %s  [%s]  %sx%d | %sx%d  [%s]  pos=%s  realized=%s fees=%s net=%s unrealized=%s",
			st.symbol, st.config.venue,
			st.buyPrice,
			q.BidPrice, q.BidSize,
			q.AskPrice, q.AskSize,
			st.sellPrice,
			st.position,
			st.realizedPnL, st.totalFees, net, unrealized)
	}
}

func synchronizeAssetsForever() {
	for {
		log.Printf("synchronizing alpaca assets...")
		if err := gBroker.SyncAssets(); err != nil {
			log.Printf("error synchronizing assets: %v", err)
		}
		clocky.Sleep(15 * clocky.Minute)
	}
}

func addSymbol(sym symbol.Symbol, cfg Config) *State {
	st := getOrCreateSymbol(sym)
	if st != nil {
		st.config = cfg
	}
	return st
}

func getOrCreateSymbol(sym symbol.Symbol) *State {
	st := gSymbols[sym]
	if st == nil {
		asset := GetAsset(sym)
		if asset != nil {
			st = &State{
				symbol: sym,
				asset:  asset,
			}
			gSymbols[sym] = st
		}
	}
	return st
}

func symbolNames() []string {
	names := make([]string, 0, len(gSymbols))
	for sym := range gSymbols {
		names = append(names, sym.String())
	}
	return names
}

func removeOrder(st *State, clientOrderID string) {
	delete(gOrders, clientOrderID)
	if st.buyClientOrderID == clientOrderID {
		st.buyClientOrderID = ""
		st.buyClientOrderID2 = ""
		st.buyOrderID = ""
		st.buyPrice = decimal.Zero
	}
	if st.sellClientOrderID == clientOrderID {
		st.sellClientOrderID = ""
		st.sellClientOrderID2 = ""
		st.sellOrderID = ""
		st.sellPrice = decimal.Zero
	}
}

func onMessage(msg *sip.Message) {
	if gTapeMsg != nil {
		gTapeMsg <- msg
	}
	switch msg.Type {
	case sip.MessageTypeQuote:
		onQuote(msg.Quote())
	case sip.MessageTypeTrade:
		onTrade(msg.Trade())
	case sip.MessageTypeStatus:
		onStatus(msg.Status())
	}
}

func onQuote(q *sip.Quote) {
	st := getOrCreateSymbol(q.Symbol)
	if st == nil {
		return
	}
	st.quote = q
	Evaluate(st)
}

func onTrade(t *sip.Trade) {
	st := gSymbols[t.Symbol]
	if st == nil {
		return
	}
}

func onStatus(s *sip.Status) {
	st := gSymbols[s.Symbol]
	if st == nil {
		return
	}
	if s.Halt() {
		st.halt = true
		log.Printf("trading of %s has halted: %s", st.symbol, s)
	} else if s.Resume() {
		st.halt = false
		log.Printf("trading of %s has resumed: %s", st.symbol, s)
	}
}

func onOrderUpdate(update *alpaca.OrderUpdate) {
	st, ok := gOrders[update.Order.ClientOrderID]
	if !ok {
		return
	}

	log.Printf("%s order %s: %s %s %s @ %s pos=%s filled=%s/%s",
		st.symbol, update.Event, update.Order.Status,
		update.Order.Side, update.Order.Qty, update.Order.LimitPrice,
		update.PositionQty, update.Order.FilledQty, update.Order.Qty)

	// update order metadata
	if update.Order.Side == ds.SideBuy {
		if st.buyOrderID == "" {
			st.buyOrderID = update.Order.ID
		}
	} else {
		if st.sellOrderID == "" {
			st.sellOrderID = update.Order.ID
		}
	}

	// track fills and P&L
	//
	// costBasis tracks total dollars invested in current position:
	//   long:  positive (dollars spent buying)
	//   short: negative (dollars received selling)
	//   zero position: zero cost basis
	//
	// when a fill reduces position size, we realize P&L proportionally
	// when a fill increases position size, we add to cost basis
	if !update.Qty.IsZero() {
		fillPrice := update.Price
		absQty := update.Qty // always positive from alpaca

		// signed fill: positive for buy, negative for sell
		signedQty := absQty
		if update.Order.Side == ds.SideSell {
			signedQty = absQty.Neg()
		}

		// track totals
		if update.Order.Side == ds.SideBuy {
			st.totalBought = st.totalBought.Add(absQty)
			st.totalCostIn = st.totalCostIn.Add(fillPrice.Mul(absQty))
		} else {
			st.totalSold = st.totalSold.Add(absQty)
			st.totalCostOut = st.totalCostOut.Add(fillPrice.Mul(absQty))
		}

		// P&L logic: is this fill reducing or increasing our position?
		oldPos := st.position
		fillPnL := decimal.Zero
		reducing := (oldPos.IsPositive() && signedQty.IsNegative()) ||
			(oldPos.IsNegative() && signedQty.IsPositive())

		if reducing && !oldPos.IsZero() {
			// closing (partially or fully): realize P&L
			avgCost := st.costBasis.Div(oldPos) // per-share cost (signed correctly)
			closingQty := absQty.Min(oldPos.Abs())
			if oldPos.IsPositive() {
				// closing long: profit = (sell price - avg cost) * qty
				fillPnL = fillPrice.Sub(avgCost).Mul(closingQty)
			} else {
				// covering short: profit = (sale price - buy price) * qty
				// avgCost = costBasis / position = negative / negative = positive sale price
				fillPnL = avgCost.Sub(fillPrice).Mul(closingQty)
			}
			st.realizedPnL = st.realizedPnL.Add(fillPnL)
			gTotalPnL = gTotalPnL.Add(fillPnL)
			// reduce cost basis proportionally
			costReduction := avgCost.Mul(closingQty)
			if oldPos.IsNegative() {
				costReduction = costReduction.Neg()
			}
			st.costBasis = st.costBasis.Sub(costReduction)
		}

		// update cost basis for any portion that opens/extends position
		if !reducing {
			// entirely opening/extending
			st.costBasis = st.costBasis.Add(fillPrice.Mul(signedQty))
		} else if absQty.Cmp(oldPos.Abs()) > 0 {
			// flipping through zero: the excess opens in the new direction
			excess := absQty.Sub(oldPos.Abs())
			st.costBasis = fillPrice.Mul(signedQty.Div(absQty)).Mul(excess)
		}

		// estimate fees/rebates (CAT + broker are per-order, not per-fill)
		firstFill := update.Order.FilledQty.Cmp(absQty) == 0
		marketable := update.Order.Type == alpaca.OrderTypeMarket ||
			(update.Order.Side == ds.SideBuy && st.quote != nil && fillPrice.Cmp(st.quote.AskPrice) >= 0) ||
			(update.Order.Side == ds.SideSell && st.quote != nil && fillPrice.Cmp(st.quote.BidPrice) <= 0)
		fee := EstimateFee(update.Order.Side, absQty, fillPrice, firstFill, marketable)
		st.totalFees = st.totalFees.Add(fee)
		gTotalFees = gTotalFees.Add(fee)

		st.position = update.PositionQty
		gTotalShares = gTotalShares.Add(absQty)
		gTotalFills++

		log.Printf("%s filled %s @ %s | pos=%s | fill_pnl=%s | fee=%s | realized=%s | total_pnl=%s | total_fees=%s | fills=%d shares=%s",
			st.symbol, signedQty, fillPrice, st.position,
			fillPnL, fee, st.realizedPnL, gTotalPnL, gTotalFees, gTotalFills, gTotalShares)
	}

	switch update.Event {
	case alpaca.OrderEventReplaced:
		if update.Order.Side == ds.SideBuy {
			st.buyOrderID = update.Order.ReplacedBy
			delete(gOrders, st.buyClientOrderID)
			st.buyClientOrderID = st.buyClientOrderID2
			st.buyClientOrderID2 = ""
			st.buyPrice = st.buyPrice2
		} else {
			st.sellOrderID = update.Order.ReplacedBy
			delete(gOrders, st.sellClientOrderID)
			st.sellClientOrderID = st.sellClientOrderID2
			st.sellClientOrderID2 = ""
			st.sellPrice = st.sellPrice2
		}
	case alpaca.OrderEventRejected:
		st.cooldownUntil = clocky.Now().Add(*flagCooldown)
		log.Printf("%s rejected: %s (setting %s cooldown)", st.symbol, update.Reason, *flagCooldown)
	}

	// clean up completed orders
	if update.Order.Status.IsFinal() {
		removeOrder(st, update.Order.ClientOrderID)
		Evaluate(st)
	}
}

func Evaluate(st *State) {
	if st.config.target.IsZero() {
		return // not configured for trading
	}
	if st.halt {
		return
	}
	if st.quote == nil {
		return
	}
	if st.quote.Indicative() {
		return
	}
	if !st.asset.Tradable.Load() {
		return
	}
	if st.cooldownUntil != 0 && clocky.Now().Before(st.cooldownUntil) {
		return // got rejected recently, wait before trying again
	}

	// check if market is open
	now := clocky.Now()
	session := cboe.GetSession(now)
	switch session {
	case cboe.SessionClosed:
		return
	case cboe.SessionExtended:
		if !*flagExtended {
			return
		}
	case cboe.SessionOvernight:
		if !*flagOvernight {
			return
		}
		if st.asset.OvernightHalted.Load() {
			return
		}
		if !st.asset.OvernightTradable.Load() {
			return
		}
	}

	cfg := &st.config
	alreadyBuying := st.buyClientOrderID != ""
	alreadySelling := st.sellClientOrderID != ""
	mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
	var canBuy, canSell bool
	var maximumPosition, minimumPosition decimal.Decimal
	if cfg.target.IsPositive() {
		minimumPosition = decimal.Zero
		maximumPosition = cfg.target.MulInt(2)
	} else {
		minimumPosition = cfg.target.MulInt(2)
		maximumPosition = decimal.Zero
	}
	canBuy = st.position.Add(cfg.qty).Cmp(maximumPosition) <= 0 && !alreadyBuying
	canSell = st.position.Sub(cfg.qty).Cmp(minimumPosition) >= 0 && !alreadySelling
	buyPrice := QuantizeBuyPrice(mid.Sub(cfg.spread))
	sellPrice := QuantizeSellPrice(mid.Add(cfg.spread))

	if canBuy {
		st.buyPrice = buyPrice
		LimitOrder(st, ds.SideBuy, cfg.qty, buyPrice, cfg.venue, session, "")
	} else if alreadyBuying && st.buyClientOrderID2 == "" {
		if buyPrice.Sub(st.buyPrice).Abs().Cmp(cfg.drift) > 0 {
			ReplaceOrder(st, ds.SideBuy, cfg.qty, buyPrice)
		}
	}

	if canSell {
		st.sellPrice = sellPrice
		LimitOrder(st, ds.SideSell, cfg.qty, sellPrice, cfg.venue, session, "")
	} else if alreadySelling && st.sellClientOrderID2 == "" {
		if sellPrice.Sub(st.sellPrice).Abs().Cmp(cfg.drift) > 0 {
			ReplaceOrder(st, ds.SideSell, cfg.qty, sellPrice)
		}
	}
}

func LimitOrder(st *State, side ds.Side, qty decimal.Decimal, price decimal.Decimal, dest alpaca.OrderDestination, session cboe.Session, explain string) {
	if qty.IsZero() || !price.IsPositive() {
		return
	}
	gOrderCount++
	quote := st.quote
	position := st.position
	clientOrderID := generateClientOrderID()
	gOrders[clientOrderID] = st
	if side == ds.SideBuy {
		st.buyClientOrderID = clientOrderID
	} else {
		st.sellClientOrderID = clientOrderID
	}
	spawn(func() {
		var advancedInstructions *alpaca.AdvancedInstructions
		if dest != alpaca.OrderDestinationNone {
			advancedInstructions = &alpaca.AdvancedInstructions{
				Algorithm:   alpaca.OrderAlgorithmDMA,
				Destination: dest,
			}
		}
		_, err := gBroker.CreateOrder(&alpaca.CreateOrderRequest{
			Symbol:               st.symbol.String(),
			Side:                 side,
			Qty:                  qty,
			Type:                 alpaca.OrderTypeLimit,
			TimeInForce:          alpaca.TimeInForceDay,
			ExtendedHours:        session == cboe.SessionExtended || session == cboe.SessionOvernight,
			LimitPrice:           price,
			ClientOrderID:        clientOrderID,
			AdvancedInstructions: advancedInstructions,
			NonBlocking:          true,
		})
		if err != nil {
			if err != ds.ErrBusy {
				log.Printf("%s error placing order: %v", st.symbol, err)
			}
			gFailedOrders <- clientOrderID
			return
		}
		log.Printf("%s placed %s %s @ %s -> %s (pos=%s spread=%s bid=%sx%d ask=%sx%d%s)",
			st.symbol, side, qty, price, dest,
			position, quote.AskPrice.Sub(quote.BidPrice),
			quote.BidPrice, quote.BidSize,
			quote.AskPrice, quote.AskSize,
			explain)
	})
}

func ReplaceOrder(st *State, side ds.Side, qty decimal.Decimal, price decimal.Decimal) {
	quote := st.quote
	clientOrderID := generateClientOrderID()
	var orderID string
	var oldPrice decimal.Decimal
	if side == ds.SideBuy {
		oldPrice = st.buyPrice
		st.buyPrice2 = price
		st.buyClientOrderID2 = clientOrderID
		orderID = st.buyOrderID
	} else {
		oldPrice = st.sellPrice
		st.sellPrice2 = price
		st.sellClientOrderID2 = clientOrderID
		orderID = st.sellOrderID
	}
	if orderID == "" {
		return
	}
	gOrders[clientOrderID] = st
	spawn(func() {
		_, err := gBroker.ReplaceOrder(orderID, &alpaca.ReplaceOrderRequest{
			LimitPrice:    price,
			ClientOrderID: clientOrderID,
			NonBlocking:   true,
		})
		if err != nil {
			if err != ds.ErrBusy {
				log.Printf("failed to replace order: %v", err)
			}
			gFailedReplaces <- clientOrderID
			return
		}
		log.Printf("%s replacing %s order: %s -> %s (pos=%s spread=%s bid=%sx%d ask=%sx%d)",
			st.symbol, side, oldPrice, price,
			st.position, quote.AskPrice.Sub(quote.BidPrice),
			quote.BidPrice, quote.BidSize,
			quote.AskPrice, quote.AskSize)
	})
}

// spawn runs f asynchronously in live mode, synchronously in backtest mode.
func spawn(f func()) {
	if *flagLive {
		go f()
	} else {
		f()
	}
}

func generateClientOrderID() string {
	gOrderSeq++
	return fmt.Sprintf("order-%d", gOrderSeq)
}

func logBalance() {
	longNotional := decimal.Zero
	shortNotional := decimal.Zero
	longCount := 0
	shortCount := 0
	pendingCount := 0
	totalUnrealized := decimal.Zero
	liquidationPnL := decimal.Zero
	for _, st := range gSymbols {
		if st.quote == nil {
			continue
		}
		if st.position.IsPositive() {
			mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
			if mid.IsZero() {
				mid = st.costBasis.Div(st.position) // fallback to avg cost
			}
			notional := mid.Mul(st.position)
			longNotional = longNotional.Add(notional)
			longCount++
			totalUnrealized = totalUnrealized.Add(notional.Sub(st.costBasis))
			liquidationPnL = liquidationPnL.Add(st.quote.BidPrice.Mul(st.position).Sub(st.costBasis))
		} else if st.position.IsNegative() {
			mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
			if mid.IsZero() {
				mid = st.costBasis.Div(st.position)
			}
			notional := mid.Mul(st.position.Neg())
			shortNotional = shortNotional.Add(notional)
			shortCount++
			totalUnrealized = totalUnrealized.Add(st.position.Mul(mid).Sub(st.costBasis))
			liquidationPnL = liquidationPnL.Add(st.position.Mul(st.quote.AskPrice).Sub(st.costBasis))
		}
		if st.buyClientOrderID != "" || st.sellClientOrderID != "" {
			pendingCount++
		}
	}
	net := gTotalPnL.Sub(gTotalFees)
	equity := net.Add(totalUnrealized)
	liquidate := net.Add(liquidationPnL)
	log.Printf("BALANCE long=$%s (%d) short=$%s (%d) expose=$%s | realize=%s fee=%s net=%s unrealized=%s equity=%s liquidate=%s | fills=%d pending=%d | ofails=%d/%d",
		longNotional.Format(0), longCount,
		shortNotional.Format(0), shortCount,
		longNotional.Sub(shortNotional).Format(0),
		gTotalPnL, gTotalFees, net, totalUnrealized, equity, liquidate,
		gTotalFills, pendingCount,
		gOrderFails, gOrderCount)
}

func shutdown() Result {
	log.Printf("shutting down... canceling all orders")
	if err := gBroker.CancelAllOrders(); err != nil {
		log.Printf("error canceling orders: %v", err)
	}
	if gTapeMsg != nil {
		log.Printf("flushing tape...")
		close(gTapeMsg)
		<-gTapeDone
	}
	log.Printf("=== P&L SUMMARY ===")
	symbols := make([]*State, 0, len(gSymbols))
	for _, st := range gSymbols {
		symbols = append(symbols, st)
	}
	slices.SortFunc(symbols, func(a, b *State) int {
		if a.symbol < b.symbol {
			return -1
		} else if a.symbol > b.symbol {
			return 1
		}
		return 0
	})
	for _, st := range symbols {
		if st.totalBought.IsZero() && st.totalSold.IsZero() || st.quote == nil {
			continue
		}
		unrealizedPnL := decimal.Zero
		if !st.position.IsZero() && !st.costBasis.IsZero() {
			mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
			unrealizedPnL = mid.Mul(st.position).Sub(st.costBasis)
		}
		log.Printf("  %8s: pos=%-8s realized=%-8s unrealized=%-8s fees=%-10s bought=%-8s sold=%-8s",
			st.symbol, st.position, st.realizedPnL.Format(2), unrealizedPnL.Format(2),
			st.totalFees.Format(2), st.totalBought, st.totalSold)
	}
	net := gTotalPnL.Sub(gTotalFees)
	log.Printf("  TOTAL realized P&L: %s  fees: %s  net: %s", gTotalPnL, gTotalFees, net)
	log.Printf("  total fills: %d  symbols tracked: %d", gTotalFills, len(gSymbols))
	return Result{
		PnL:    gTotalPnL,
		Fees:   gTotalFees,
		Net:    net,
		Fills:  gTotalFills,
		Shares: gTotalShares,
	}
}

func QuantizeBuyPrice(price decimal.Decimal) decimal.Decimal {
	return price.QuantizeTruncate(Tick(price)) // buy low
}

func QuantizeSellPrice(price decimal.Decimal) decimal.Decimal {
	return price.QuantizeAway(Tick(price)) // sell high
}

func Tick(price decimal.Decimal) decimal.Decimal {
	if price.Cmp(decimal.One) < 0 {
		return decimal.Pip
	} else {
		return decimal.Cent
	}
}

func GetAsset(sym symbol.Symbol) *alpaca.Asset {
	if gIgnoreSymbols[sym] {
		return nil
	}
	asset := alpaca.GetAsset(sym)
	if asset == nil {
		gIgnoreSymbols[sym] = true
		return nil
	}
	if asset.Exchange == alpaca.ExchangeOTC ||
		asset.Class != alpaca.AssetClassUSEquity ||
		asset.Status.Load() != alpaca.AssetStatusActive ||
		asset.PTPNoException.Load() ||
		asset.PTPWithException.Load() {
		gIgnoreSymbols[sym] = true
		return nil
	}
	return asset
}
