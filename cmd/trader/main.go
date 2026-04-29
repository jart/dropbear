// quote imbalance trend follower
//
// this algorithm monitors alpaca's real-time stock data feeds (sip and boats)
// for quotes where the bid size is substantially greater than the ask size or
// vice versa, which can be a signal of short-term momentum. so if everyone is
// trying to buy a stock, then we try to do it too, at the bid price. if we're
// filled, then we immediately try to flip the shares at the asking price.
package main

import (
	"dropbear/broker/alpaca"
	"dropbear/broker/alpaca/sip"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/loggy"
	"dropbear/netty"
	"dropbear/symbol"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"slices"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
)

var (
	flagSize      = decimal.Flag("size", "10_000", "how much capital to devote to each trade")
	flagRisk      = decimal.Flag("risk", "5_000", "maximum portfolio exposure in usd before index hedging")
	flagHedge     = symbol.Flag("hedge", "IWM", "symbol to use for index hedging when risk threshold is breached")
	flagPower     = decimal.Flag("power", "200_000", "amount of capital available to backtest")
	flagMaxSyms   = flag.Int("maxsyms", 500, "maximum number of symbols to actively trade simultaneously")
	flagGreed     = decimal.FlagBPS("greed", "10", "amount of basis points to demand over cost basis")
	flagMargin    = decimal.Flag("margin", "0.3", "maximum margin requirement to consider trading stock")
	flagMinEdge   = decimal.Flag("minedge", "1", "minimum spread in ticks")
	flagMinPrice  = decimal.Flag("minprice", "1", "minimum price of stock in usd to trade")
	flagMaxPrice  = decimal.Flag("maxprice", "1000", "maximum price of stock in usd to trade")
	flagThreshold = decimal.Flag("threshold", "0.1", "imbalance ratio threshold to trigger (0-1)")
	flagISOShares = decimal.Flag("iso", "1000", "net ISO shares threshold to trigger entry")
	flagUnload    = clocky.DurationFlag("unload", "60m", "time before day session close to switch to exit-only (0 to disable)")
	flagFlatten   = clocky.DurationFlag("flatten", "11m", "time before day session close to flatten positions with moc orders (0 to disable)")
	flagPatience  = clocky.DurationFlag("patience", "5m", "time to wait before canceling unfilled orders (0 to disable)")
	flagFreshness = clocky.DurationFlag("freshness", "1s", "required freshness of quote to be eligible for trading")
	flagCooldown  = clocky.DurationFlag("cooldown", "30m", "cooldown period after order rejection")
	flagVolume    = decimal.FlagPercent("volume", "90", "minimum volume percentile to enter new positions")
	flagBucket    = flag.String("bucket", "dropbear-sip", "google cloud storage bucket for recording market data")
	flagExitOnly  = flag.Bool("exit", false, "exit-only mode: close existing positions, no new entries")
	flagLive      = flag.Bool("live", false, "enables live trading and network access")
	flagOvernight = flag.Bool("overnight", false, "enables overnight hours trading")
	flagExtended  = flag.Bool("extended", false, "enables extended hours trading")
	flagData      = flag.String("data", "", "path of sip data file for backtest")
)

var (
	kStandardMarginRate   = decimal.Parse("0.3")
	kVolumeInterval       = 23 * clocky.Second
	kHeartbeatInterval    = 1 * clocky.Second
	kBalanceInterval      = 5 * clocky.Second
	kVolatilityLookbehind = 1 * clocky.Hour
	kPricePeriod          = clocky.Second
	kPriceSmoothing       = 2000
	kVolumeSmoothing      = 7
)

var (
	gBroker           Broker
	gOrders           map[string]*State
	gSymbols          map[symbol.Symbol]*State
	gActiveSymbols    map[symbol.Symbol]bool
	gIgnoreSymbols    map[symbol.Symbol]bool
	gFailedOrders     chan string
	gNextBalance      clocky.Time
	gNextVolume       clocky.Time
	gTotalPnL         decimal.Decimal
	gTotalFees        decimal.Decimal
	gTotalShares      decimal.Decimal
	gTotalFills       int
	gOrderCount       int64
	gOrderFails       int64
	gPhase            Phase
	gTapeMsg          chan *sip.Message
	gTapeDone         chan struct{}
	gBacktest         *Backtest
	gAlpacaClient     *alpaca.Client
	gBuyingPower      decimal.Decimal
	gSomethingChanged atomic.Bool
)

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

	if !flagSize.IsPositive() {
		fmt.Fprintf(os.Stderr, "-size must be positive\n")
		os.Exit(1)
	}
	if flagRisk.IsNegative() {
		fmt.Fprintf(os.Stderr, "-risk can't be negative\n")
		os.Exit(1)
	}
	if !flagMinPrice.IsPositive() {
		fmt.Fprintf(os.Stderr, "-minprice must be positive\n")
		os.Exit(1)
	}
	if !flagMaxPrice.IsPositive() {
		fmt.Fprintf(os.Stderr, "-maxprice must be positive\n")
		os.Exit(1)
	}
	if !flagISOShares.IsPositive() {
		fmt.Fprintf(os.Stderr, "-iso must be positive\n")
		os.Exit(1)
	}
	if *flagMaxSyms <= 0 {
		fmt.Fprintf(os.Stderr, "-maxsyms must be positive\n")
		os.Exit(1)
	}
	if !flagThreshold.IsPositive() || flagThreshold.Cmp(decimal.One) > 0 {
		fmt.Fprintf(os.Stderr, "-threshold must be between 0 and 1\n")
		os.Exit(1)
	}
	if *flagFlatten != 0 && *flagFlatten <= 10*clocky.Minute {
		fmt.Fprintf(os.Stderr, "-flatten must be greater than 10 minutes if specified\n")
		os.Exit(1)
	}
	if *flagHedge != 0 && !StockExists(*flagHedge) {
		fmt.Fprintf(os.Stderr, "-hedge asset not found: %s\n", *flagHedge)
		os.Exit(1)
	}
	if !*flagLive && *flagData == "" {
		fmt.Fprintf(os.Stderr, "-data must be specified for backtest mode\n")
		os.Exit(1)
	}

	// log configuration
	log.Printf("prepare to trade")
	log.Printf("  size=%s", *flagSize)
	log.Printf("  risk=%s", *flagRisk)
	log.Printf("  hedge=%s", *flagHedge)
	log.Printf("  maxsyms=%d", *flagMaxSyms)
	log.Printf("  greed=%s", *flagGreed)
	log.Printf("  minedge=%s", *flagMinEdge)
	log.Printf("  minprice=%s", *flagMinPrice)
	log.Printf("  maxprice=%s", *flagMaxPrice)
	log.Printf("  threshold=%s", *flagThreshold)
	log.Printf("  iso=%s", *flagISOShares)
	log.Printf("  unload=%s", *flagUnload)
	log.Printf("  flatten=%s", *flagFlatten)
	log.Printf("  patience=%s", *flagPatience)
	log.Printf("  exitonly=%t", *flagExitOnly)

	// initialize globals
	gOrders = map[string]*State{}
	gSymbols = map[symbol.Symbol]*State{}
	gActiveSymbols = map[symbol.Symbol]bool{}
	gIgnoreSymbols = map[symbol.Symbol]bool{}
	gFailedOrders = make(chan string, 32)
	gNextVolume = clocky.Now().Add(kVolumeInterval)
	gNextBalance = clocky.Now().Add(kBalanceInterval)
	if *flagLive {
		gAlpacaClient = alpaca.NewClient()
		gBroker = gAlpacaClient
	}

	// periodically fetch information about supported equities from alpaca
	if *flagLive {
		go synchronizeAssetsForever()
		go synchronizeAccountForever()
	}

	// cancel lingering orders
	if *flagLive {
		log.Printf("canceling lingering orders...")
		if _, err := gBroker.CancelAllOrders(); err != nil {
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
			st.greed = *flagGreed
			if st.Active() {
				gActiveSymbols[sym] = true
			}
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
		log.Printf("subscribing to sip stock updates...")
		stockUpdates = alpaca.MustStockUpdates(alpaca.SIPWSURL, &alpaca.StockUpdatesRequest{
			Action:     "subscribe",
			Quotes:     []string{"*"},
			Trades:     []string{"*"},
			Statuses:   []string{"*"},
			Imbalances: []string{"*"},
			LULDs:      []string{"*"},
		})
		log.Printf("subscribing to boats stock updates...")
		boatsUpdates = alpaca.MustStockUpdates(alpaca.BOATSWSURL, &alpaca.StockUpdatesRequest{
			Action:   "subscribe",
			Quotes:   []string{"*"},
			Trades:   []string{"*"},
			Statuses: []string{"*"},
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
		case <-heartbeatChan:
			onHeartbeat()
		case <-sigChan:
			shutdown()
			return
		}
	}
}

func onHeartbeat() {

	// practice patience
	cleanupOrders()

	// show balance periodically
	now := clocky.Now()
	if now.After(gNextBalance) {
		logBalance()
		gNextBalance = now.Add(kBalanceInterval)
	}

	// decay iso net flow by half every second
	// so that old trades lose influence over time
	for _, st := range gSymbols {
		st.isoNetFlow = st.isoNetFlow.Half()
	}

	// recompute volume percentile rankings
	if now.After(gNextVolume) {
		recomputeVolumeRanks()
		gNextVolume = now.Add(kVolumeInterval)
	}

	// handle end of day phase transitions
	switch gPhase {
	case PhaseNormal, PhaseExitOnly:
		if cboe.GetSession(now) != cboe.SessionDay {
			if gPhase == PhaseExitOnly {
				gPhase = PhaseNormal
				log.Printf("day session ended, resuming normal trading")
			}
			return
		}
		year, month, day := now.Date()
		closeTime := cboe.GetCloseTime(year, month, day)
		if *flagFlatten != 0 && closeTime.Sub(now) <= *flagFlatten {
			if len(gOrders) == 0 {
				flatten()
			} else {
				gPhase = PhaseCanceling
				log.Printf("we shall cancel all orders so we can flatten")
				if _, err := gBroker.CancelAllOrders(); err != nil {
					log.Printf("warning: error canceling orders: %v", err)
				}
			}
		} else if gPhase != PhaseExitOnly && *flagUnload != 0 && closeTime.Sub(now) <= *flagUnload {
			log.Printf("switching to exit-only mode")
			gPhase = PhaseExitOnly
		}
	}
}

// flatten closes all positions with market-on-close orders.
// this ensures we get maximum day trading buying power the next day.
func flatten() {
	for _, st := range gSymbols {
		if st.position.IsZero() {
			continue
		}
		qty := st.position
		side := ds.SideSell
		if qty.IsNegative() {
			side = ds.SideBuy
			qty = qty.Neg()
		}
		MarketOnCloseOrder(st, side, qty)
		gPhase = PhaseFlattening
	}
}

func synchronizeAssetsForever() {
	for {
		log.Printf("synchronizing alpaca assets...")
		if err := gBroker.SyncAssets(); err != nil {
			log.Printf("error synchronizing assets: %v", err)
		}
		time.Sleep(15 * time.Minute)
	}
}

func synchronizeAccountForever() {
	for {
		if gSomethingChanged.Load() {
			account, err := gAlpacaClient.GetAccount()
			if err != nil {
				log.Printf("error synchronizing account: %v", err)
			} else {
				gBuyingPower.Store(account.RegTBuyingPower)
			}
		}
		time.Sleep(3 * time.Second)
	}
}

func haveBuyingPower(notional decimal.Decimal) bool {
	buyingPower := gBuyingPower.Load()
	if buyingPower.IsZero() {
		return false
	}
	return notional.Cmp(buyingPower) <= 0
}

func getMarketValue() decimal.Decimal {
	value := decimal.Zero
	for _, st := range gSymbols {
		if st.quote == nil {
			continue // can't compute exposure without quote
		}
		mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
		if st.position.IsPositive() {
			notional := mid.Mul(st.position)
			value = value.Add(notional)
		} else if st.position.IsNegative() {
			notional := mid.Mul(st.position.Neg())
			value = value.Add(notional)
		}
	}
	return value
}

func getOrCreateSymbol(sym symbol.Symbol) *State {
	st := gSymbols[sym]
	if st == nil {
		asset := GetAsset(sym)
		if asset != nil {
			st = &State{
				symbol:        sym,
				asset:         asset,
				pricema:       indicators.NewWWMA(kPriceSmoothing),
				volma:         indicators.NewWWMA(kVolumeSmoothing),
				minTradePrice: indicators.NewMin(kVolatilityLookbehind),
				maxTradePrice: indicators.NewMax(kVolatilityLookbehind),
			}
			if gAlpacaClient != nil {
				bars, _, err := gAlpacaClient.GetBars(sym, clocky.Minute, clocky.Now().Add(-2*clocky.Day), 0, alpaca.FeedSIP, alpaca.BarAdjustmentRaw, 0, false, "")
				if err != nil {
					log.Printf("error fetching bars for %s: %v", sym, err)
				} else {
					for _, bar := range bars {
						st.minTradePrice.Add(bar.Timestamp, bar.Low)
						st.maxTradePrice.Add(bar.Timestamp, bar.High)
					}
				}
			}
			gSymbols[sym] = st
		}
	}
	return st
}

func removeOrder(st *State, clientOrderID string) {
	delete(gOrders, clientOrderID)
	if st.buyClientOrderID == clientOrderID {
		st.buyClientOrderID = ""
		st.buyOrderID = ""
	}
	if st.sellClientOrderID == clientOrderID {
		st.sellClientOrderID = ""
		st.sellOrderID = ""
	}
	if !st.Active() {
		delete(gActiveSymbols, st.symbol)
	}
	if len(gOrders) == 0 {
		switch gPhase {
		case PhaseCanceling:
			log.Printf("canceling complete so we may proceed with the flattening")
			flatten()
		case PhaseFlattening:
			log.Printf("flattening complete")
			gPhase = PhaseNormal
		}
	}
}

func OpenQuantity(price, margin decimal.Decimal) decimal.Decimal {
	// alpaca's overnight margin requirement is normally 0.3 but we've seen it go as
	// high as 2.0 for risky stocks. penny stocks (anything under $5) usually have a
	// margin requirement of 1.0. leveraged etfs will usually be 0.6 to 0.9. so what
	// happens here is as margin grows above 0.3 we scale down how much size it uses
	rat := kStandardMarginRate.Div(margin)
	qty := flagSize.Mul(rat).Div(price)
	// always trade round lots
	return qty.QuantizeTruncate(cboe.LotSize(price)).Min(cboe.LotSize(price))
}

func CloseQuantity(price, qty decimal.Decimal) decimal.Decimal {
	lot := cboe.LotSize(price)
	if qty.Cmp(lot) >= 0 {
		return qty.QuantizeTruncate(lot)
	} else {
		return qty
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
	switch t.Tape {
	case sip.TapeA, sip.TapeB, sip.TapeC:
		if t.Conditions&(sip.TradeCondRegularSaleCTA|sip.TradeCondRegularSale|sip.TradeCondExtendedHours) != 0 {
			recordTrade(st, t)
		}
	default:
		recordTrade(st, t)
	}
	// classify direction using Lee-Ready:
	// trade above midpoint = buyer-initiated
	// trade below midpoint = seller-initiated
	if st.quote != nil && t.Conditions.Has(sip.TradeCondISO) {
		mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
		if mid.IsZero() {
			return
		}
		size := decimal.FromInt64(t.Size)
		cmp := t.Price.Cmp(mid)
		if cmp > 0 {
			st.isoNetFlow = st.isoNetFlow.Add(size)
		} else if cmp < 0 {
			st.isoNetFlow = st.isoNetFlow.Sub(size)
		}
	}
}

func recordTrade(st *State, t *sip.Trade) {
	st.volsum = st.volsum.Add(decimal.FromInt64(t.Size))
	st.lastTradePrice = t.Price
	st.minTradePrice.Add(t.Timestamp, t.Price)
	st.maxTradePrice.Add(t.Timestamp, t.Price)
	now := clocky.Now()
	if now.After(st.nextprice) {
		st.pricema.Add(t.Price)
		st.nextprice = now.Add(kPricePeriod)
	}
	Evaluate(st)
}

func recomputeVolumeRanks() {
	var volumes []decimal.Decimal
	for _, st := range gSymbols {
		st.highVolume = false
		st.volma.Add(st.volsum)
		st.volsum = decimal.Zero
		if st.volma.IsReady() {
			volumes = append(volumes, st.volma.Value)
		}
	}
	if len(volumes) == 0 {
		return
	}
	slices.SortFunc(volumes, decimal.Compare)
	idx := decimal.FromInt(len(volumes)).Mul(*flagVolume).Truncate().Int()
	if idx >= len(volumes) {
		idx = len(volumes) - 1
	}
	threshold := volumes[idx]
	for _, st := range gSymbols {
		st.highVolume = st.volma.IsReady() && st.volma.Value.Cmp(threshold) >= 0
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
	gSomethingChanged.Store(true)
	st, ok := gOrders[update.Order.ClientOrderID]
	if !ok {
		return
	}

	log.Printf("%s order %s: %s price=%s qty=%s pos=%s filled=%s/%s",
		st.symbol, update.Event, update.Order.Status,
		update.Price, update.Qty, update.PositionQty,
		update.Order.FilledQty, update.Order.Qty)

	// update order metadata
	if st.orderCreatedTime == -1 {
		st.orderCreatedTime = clocky.Now()
	}
	if update.Order.Side == ds.SideBuy {
		st.buyOrderID = update.Order.ID
	} else {
		st.sellOrderID = update.Order.ID
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
			if absQty.Cmp(oldPos.Abs()) > 0 {
				panic(fmt.Sprintf("logic error: fill quantity %s exceeds position size %s", absQty, oldPos))
			}
			if oldPos.IsPositive() {
				// closing long: profit = (sell price - avg cost) * qty
				fillPnL = fillPrice.Sub(avgCost).Mul(absQty)
			} else {
				// covering short: profit = (sale price - buy price) * qty
				// avgCost = costBasis / position = negative / negative = positive sale price
				fillPnL = avgCost.Sub(fillPrice).Mul(absQty)
			}
			st.realizedPnL = st.realizedPnL.Add(fillPnL)
			gTotalPnL = gTotalPnL.Add(fillPnL)
			// reduce cost basis proportionally
			costReduction := avgCost.Mul(absQty)
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
		fee := alpaca.EstimateFee(update.Order.Side, absQty, fillPrice, firstFill, marketable)
		st.totalFees = st.totalFees.Add(fee)
		gTotalFees = gTotalFees.Add(fee)

		st.position = update.PositionQty
		gTotalShares = gTotalShares.Add(absQty)
		gTotalFills++

		log.Printf("%s filled %s @ %s | pos=%s | fill_pnl=%s | fee=%s | realized=%s | total_pnl=%s | total_fees=%s | fills=%d shares=%s",
			st.symbol, signedQty, fillPrice, st.position,
			fillPnL, fee, st.realizedPnL, gTotalPnL, gTotalFees, gTotalFills, gTotalShares)
	}

	// cooldown on rejection
	if update.Event == alpaca.OrderEventRejected {
		st.cooldownUntil = clocky.Now().Add(*flagCooldown)
		log.Printf("%s rejected: %s (setting %s cooldown)", st.symbol, update.Reason, *flagCooldown)
	}

	// clean up completed orders
	if update.Order.Status.IsFinal() {
		removeOrder(st, update.Order.ClientOrderID)
		Evaluate(st)
	}

	// manage our risk
	if update.Order.Symbol != flagHedge.String() {
		manageHedge()
	}
}

func Evaluate(st *State) {
	switch gPhase {
	case PhaseCanceling, PhaseFlattening:
		return
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
	if st.symbol == *flagHedge {
		return // let manageHedge trade this
	}
	if st.cooldownUntil != 0 && clocky.Now().Before(st.cooldownUntil) {
		return // got rejected recently, wait before trying again
	}

	// don't trade old quotes
	now := clocky.Now()
	if st.quote.Timestamp.Add(*flagFreshness).Before(now) {
		return
	}

	// check if market is open
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

	// determine if it's a good idea to trade
	mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
	shouldBuy := st.pricema.IsReady() && mid.Cmp(st.pricema.Value) < 0
	shouldSell := st.pricema.IsReady() && mid.Cmp(st.pricema.Value) > 0

	// determine if we should be greedy
	wantGreed := !st.greed.IsZero() && !st.costBasis.IsZero()
	noGreed := gPhase == PhaseExitOnly && session == cboe.SessionDay
	greedy := wantGreed && !noGreed
	explain := fmt.Sprintf(" because(wantGreed=%v noGreed=%v greed=%v)", wantGreed, noGreed, greedy)
	if !st.minTradePrice.IsReady() || st.minTradePrice.Value.IsZero() {
		return
	}
	st.greed = flagGreed.Max(st.maxTradePrice.Value.Sub(st.minTradePrice.Value).Div(st.minTradePrice.Value).Half())

	// PRIORITY 1: Exit existing positions at a profit.
	// Post AT the bid/ask (don't improve) so we capture the spread.
	// Entry improves by a penny to get priority; exit waits at the edge.
	// With -winner, floor the exit price at avg entry + a tick to guarantee profit.
	if shouldSell && st.position.IsPositive() && st.sellClientOrderID == "" && st.buyClientOrderID == "" && st.asset.Tradable.Load() && !st.quote.DangerousAsk() {
		price := st.quote.AskPrice
		if greedy {
			avgCost := st.costBasis.Div(st.position)
			minPrice := avgCost.Mul(decimal.One.Add(st.greed))
			price = price.Max(minPrice)
			price = QuantizeSellPrice(price)
			explain = fmt.Sprintf("%s clamp(avgCost=%s minPrice=%s)", explain, avgCost, minPrice)
		}
		dest := BestDestination(st.quote.AskExchange, st.asset, session)
		qty := CloseQuantity(price, st.position)
		LimitOrder(st, ds.SideSell, qty, price, dest, session, explain)
		return
	}
	if shouldBuy && st.position.IsNegative() && st.buyClientOrderID == "" && st.sellClientOrderID == "" && st.asset.Tradable.Load() && !st.quote.DangerousBid() {
		price := st.quote.BidPrice
		if greedy {
			// short: avg entry = costBasis / position = negative / negative = positive
			avgCost := st.costBasis.Div(st.position.Neg())
			maxPrice := avgCost.Mul(decimal.One.Sub(st.greed))
			price = price.Min(maxPrice)
			price = QuantizeBuyPrice(price)
			explain = fmt.Sprintf("%s clamp(avgCost=%s maxPrice=%s)", explain, avgCost, maxPrice)
		}
		dest := BestDestination(st.quote.BidExchange, st.asset, session)
		qty := CloseQuantity(price, st.position.Neg())
		LimitOrder(st, ds.SideBuy, qty, price, dest, session, explain)
		return
	}

	// PRIORITY 2: enter new positions
	if *flagExitOnly || gPhase == PhaseExitOnly {
		return
	}
	if len(gActiveSymbols) >= *flagMaxSyms {
		return
	}
	if st.Active() {
		return
	}
	if !st.highVolume {
		return
	}
	if st.quote.DangerousAsk() || st.quote.DangerousBid() {
		return
	}
	spread := st.quote.AskPrice.Sub(st.quote.BidPrice)
	if spread.Cmp(flagMinEdge.Mul(Tick(st.quote.BidPrice))) < 0 {
		return
	}
	if !st.asset.Marginable.Load() {
		return
	}
	bidSize := decimal.FromInt(int(st.quote.BidSize))
	askSize := decimal.FromInt(int(st.quote.AskSize))
	totalSize := bidSize.Add(askSize)
	if totalSize.IsZero() {
		return
	}
	if st.maxTradePrice.Age() < clocky.Hour {
		return
	}

	// bullBreakout := st.lastTradePrice.Cmp(st.maxTradePrice.Value) >= 0
	// bearBreakdown := st.lastTradePrice.Cmp(st.minTradePrice.Value) <= 0
	// imbalance := bidSize.Sub(askSize).Div(totalSize)
	wantBuy := // bullBreakout && imbalance.Cmp(*flagThreshold) > 0 &&
		st.asset.MarginRequirementLong.Load().Cmp(*flagMargin) <= 0
	wantSell := // bearBreakdown && imbalance.Cmp(flagThreshold.Neg()) < 0 &&
		st.asset.MarginRequirementShort.Load().Cmp(*flagMargin) <= 0 &&
			st.asset.EasyToBorrow.Load() && st.asset.Shortable.Load()

	if shouldBuy && wantBuy {
		price := st.quote.BidPrice
		if price.Cmp(*flagMinPrice) >= 0 && price.Cmp(*flagMaxPrice) < 0 {
			mar := st.asset.MarginRequirementLong.Load()
			qty := OpenQuantity(mid, mar)
			if haveBuyingPower(qty.Mul(price)) {
				dst := BestDestination(st.quote.BidExchange, st.asset, session)
				LimitOrder(st, ds.SideBuy, qty, price, dst, session, "")
				st.greed = *flagGreed
			}
		}
	}

	if shouldSell && wantSell {
		price := st.quote.AskPrice
		if price.Cmp(*flagMinPrice) >= 0 && price.Cmp(*flagMaxPrice) < 0 {
			mar := st.asset.MarginRequirementShort.Load()
			qty := OpenQuantity(mid, mar)
			if haveBuyingPower(qty.Mul(price)) {
				dst := BestDestination(st.quote.AskExchange, st.asset, session)
				LimitOrder(st, ds.SideSell, qty, price, dst, session, "")
				st.greed = *flagGreed
			}
		}
	}
}

func BestDestination(exchange sip.Exchange, asset *alpaca.Asset, session cboe.Session) alpaca.OrderDestination {
	// overnight trading happens on boats which we can't dma
	// just let alpaca smart router figure things out for us
	if session == cboe.SessionOvernight {
		return alpaca.OrderDestinationNone
	}
	if true {
		return alpaca.OrderDestinationNASDAQ
	}
	// dma to whichever exchange quote came from
	// assuming we have the ability to directly route there
	switch exchange {
	case sip.ExchangeNASDAQ:
		return alpaca.OrderDestinationNASDAQ
	case sip.ExchangeARCA:
		return alpaca.OrderDestinationARCA
	case sip.ExchangeNYSE:
		return alpaca.OrderDestinationNYSE
	}
	// otherwise dma to exchange on which stock is listed
	// for tech stocks this will almost certainly be nasdaq
	// for etfs it's usually arca which is where action happens
	switch asset.Exchange {
	case alpaca.ExchangeNASDAQ:
		return alpaca.OrderDestinationNASDAQ
	case alpaca.ExchangeARCA:
		return alpaca.OrderDestinationARCA
	case alpaca.ExchangeNYSE:
		// nyse is only open during day
		if session == cboe.SessionDay {
			return alpaca.OrderDestinationNYSE
		}
	}
	// fallback to best exchange that exists
	return alpaca.OrderDestinationNASDAQ
}

func LimitOrder(st *State, side ds.Side, qty decimal.Decimal, price decimal.Decimal, dest alpaca.OrderDestination, session cboe.Session, explain string) {
	if qty.IsZero() || !price.IsPositive() {
		return
	}

	gOrderCount++
	clientOrderID := generateClientOrderID()
	st.orderCreatedTime = -1
	gOrders[clientOrderID] = st
	gActiveSymbols[st.symbol] = true
	if side == ds.SideBuy {
		st.buyClientOrderID = clientOrderID
	} else {
		st.sellClientOrderID = clientOrderID
	}

	logMsg := fmt.Sprintf(
		"%s placed %s %s @ %s -> %s (pos=%s spread=%s bid=%sx%d ask=%sx%d%s)",
		st.symbol, side, qty, price, dest,
		st.position, st.quote.AskPrice.Sub(st.quote.BidPrice),
		st.quote.BidPrice, st.quote.BidSize,
		st.quote.AskPrice, st.quote.AskSize,
		explain)

	// don't slow down quote processing
	go func() {

		// configure alpaca smart router
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
		log.Printf("%s", logMsg)
	}()
}

func MarketOnCloseOrder(st *State, side ds.Side, qty decimal.Decimal) {
	if qty.IsZero() {
		return
	}
	clientOrderID := generateClientOrderID()
	gOrders[clientOrderID] = st
	gActiveSymbols[st.symbol] = true
	st.orderCreatedTime = 0
	if side == ds.SideBuy {
		st.buyClientOrderID = clientOrderID
	} else {
		st.sellClientOrderID = clientOrderID
	}
	log.Printf("placing moc order to %s %s shares of %s", side, qty, st.symbol)
	go func() {
		_, err := gBroker.CreateOrder(&alpaca.CreateOrderRequest{
			Symbol:        st.symbol.String(),
			Side:          side,
			Qty:           qty,
			Type:          alpaca.OrderTypeMarket,
			TimeInForce:   alpaca.TimeInForceCLS,
			ClientOrderID: clientOrderID,
		})
		if err != nil {
			log.Printf("%s error placing order: %v", st.symbol, err)
			gFailedOrders <- clientOrderID
			return
		}
	}()
}

func generateClientOrderID() string {
	return uuid.New().String()
}

func logBalance() {
	longNotional := decimal.Zero
	shortNotional := decimal.Zero
	longCount := 0
	shortCount := 0
	pendingCount := 0
	totalUnrealized := decimal.Zero
	liquidationPnL := decimal.Zero
	hedgeNotional := decimal.Zero
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
		if st.symbol == *flagHedge && !st.position.IsZero() {
			mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
			hedgeNotional = mid.Mul(st.position)
		}
	}
	net := gTotalPnL.Sub(gTotalFees)
	equity := net.Add(totalUnrealized)
	liquidate := net.Add(liquidationPnL)
	log.Printf("BALANCE long=$%s (%d) short=$%s (%d) expose=$%s | realize=%s fee=%s net=%s unrealized=%s equity=%s liquidate=%s | hedge=%s | fills=%d pending=%d active=%d | ofails=%d/%d",
		longNotional.Format(0), longCount,
		shortNotional.Format(0), shortCount,
		longNotional.Sub(shortNotional).Format(0),
		gTotalPnL, gTotalFees, net, totalUnrealized, equity, liquidate,
		hedgeNotional, gTotalFills, pendingCount, len(gActiveSymbols),
		gOrderFails, gOrderCount)
	// simulate buying power
	if !*flagLive {
		capital := flagPower.Add(gTotalPnL).Sub(gTotalFees)
		gBuyingPower.Store(capital.Sub(getMarketValue()))
	}
}

func shutdown() {
	log.Printf("shutting down... canceling all orders")
	if _, err := gBroker.CancelAllOrders(); err != nil {
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
		log.Printf("  %8s: price=%-8s pos=%-8s realized=%-8s unrealized=%-8s fees=%-10s bought=%-8s sold=%-8s",
			st.symbol, st.lastTradePrice, st.position, st.realizedPnL.Format(2), unrealizedPnL.Format(2),
			st.totalFees.Format(2), st.totalBought, st.totalSold)
	}
	net := gTotalPnL.Sub(gTotalFees)
	log.Printf("  TOTAL realized P&L: %s  fees: %s  net: %s", gTotalPnL, gTotalFees, net)
	log.Printf("  total fills: %d  symbols tracked: %d", gTotalFills, len(gSymbols))
	fmt.Printf("total P&L: %s  fees: %s  net: %s  fills: %d  shares: %s\n", gTotalPnL, gTotalFees, net, gTotalFills, gTotalShares)
	os.Exit(0)
}

func cleanupOrders() {
	now := clocky.Now()
	for _, st := range gOrders {
		if st.orderCreatedTime <= 0 {
			continue
		}
		elapsed := now.Sub(st.orderCreatedTime)
		if elapsed > *flagPatience {
			log.Printf("%s canceling stale order (created %s ago)", st.symbol, elapsed)
			if st.buyOrderID != "" {
				cancelOrder(st.buyOrderID)
				st.orderCreatedTime = 0
			}
			if st.sellOrderID != "" {
				cancelOrder(st.sellOrderID)
				st.orderCreatedTime = 0
			}
		}
	}
}

func cancelOrder(orderID string) {
	if err := gBroker.CancelOrder(orderID); err != nil {
		log.Printf("error canceling order: %v", err)
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

func StockExists(sym symbol.Symbol) bool {
	return GetAsset(sym) != nil
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
