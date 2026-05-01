// symmetrical market making strategy
package main

import (
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
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

var (
	flagCooldown  = clocky.DurationFlag("cooldown", "30m", "cooldown period after order rejection")
	flagBucket    = flag.String("bucket", "dropbear-sip", "google cloud storage bucket for recording market data")
	flagBudget    = decimal.Flag("budget", "30_000", "max notional exposure per stock")
	flagLive      = flag.Bool("live", false, "enables live trading and network access")
	flagData      = flag.String("data", "", "path of sip data file for backtest")
	flagTUI       = flag.Bool("tui", false, "enable terminal user interface")
	flagExit      = decimal.FlagPercent("exit", "5", "target profit as percent of entry price")
	flagBailLong  = clocky.DurationFlag("bail-long", "0", "close long if not profitable within this duration (0=disabled)")
	flagBailShort = clocky.DurationFlag("bail-short", "0", "close short if not profitable within this duration (0=disabled)")
	flagMinPrice  = decimal.Flag("min-price", "1", "minimum stock price to scoop")
	flagMaxSpread = decimal.FlagPercent("max-spread", "5", "maximum bid-ask spread as percent of midpoint")
	flagMinTrades = flag.Int("min-trades", 10, "minimum trades seen before scooping a stock")
)

const (
	kHeartbeatInterval = 1 * clocky.Second
	kBalanceInterval   = 5 * clocky.Second
	kDecayInterval     = 15 * clocky.Second
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
	gNextDecay      clocky.Time
	gTotalPnL       decimal.Decimal
	gTotalFees      decimal.Decimal
	gTotalShares    decimal.Decimal
	gTotalFills     int
	gOrderCount     int64
	gOrderFails     int64
	gTapeMsg        chan alpaca.StockUpdate
	gTapeDone       chan struct{}
	gLogMsg         chan string
	gLogDone        chan struct{}
	gBacktest       *Backtest
	gAlpacaClient   *alpaca.Client
	gOrderSeq       int64
	gSystemOn       bool // kill switch: when false, no new positions opened
	gMu             sync.Mutex
)

func main() {
	log.SetFlags(0)
	log.SetOutput(&gLogWriter)
	flag.Parse()
	if *flagLive {
		if *loggy.FlagLog != "" {
			f, err := os.OpenFile(*loggy.FlagLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				log.Fatalf("opening log file: %v", err)
			}
			gLogWriter.file = f
		}
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
	result := Run()
	fmt.Printf("total P&L: %s  fees: %s  net: %s  fills: %d  shares: %s\n",
		result.PnL, result.Fees, result.Net, result.Fills, result.Shares)
	os.Exit(0)
}

func Evaluate(st *State) {
	if st.quote == nil {
		return
	}
	if !st.asset.Tradable.Load() {
		return
	}
	if st.cooldownUntil != 0 && clocky.Now().Before(st.cooldownUntil) {
		return
	}

	// check time
	now := clocky.Now()
	session := cboe.GetSession(now)
	switch session {
	case cboe.SessionClosed, cboe.SessionOvernight:
		return
	}

	// === EXIT ===
	if st.position.Sign() != 0 {
		exitScoop(st, now, session)
		return
	}

	// === ENTRY: no new positions if disabled or system off ===
	if st.disabled {
		return
	}
	if !gSystemOn {
		return
	}
	if st.halt {
		return
	}
	if st.position.Sign() != 0 {
		return // already have a position
	}
	if st.buyClientOrderID != "" || st.sellClientOrderID != "" {
		return // already have a pending order
	}
	if st.quote.Indicative() {
		return
	}

	// Safety filters: skip zombie SPACs, illiquid trash, and wide-spread garbage.
	if st.quote.BidPrice.IsPositive() && st.quote.AskPrice.IsPositive() {
		mid := st.quote.Midpoint()
		if mid.Cmp(*flagMinPrice) < 0 {
			return // too cheap
		}
		spread := st.quote.Spread()
		maxSpread := mid.Mul(*flagMaxSpread)
		if spread.Cmp(maxSpread) > 0 {
			return // spread too wide
		}
	}
	if st.tradeCount < *flagMinTrades {
		return // not enough trading activity yet
	}

	// Determine if we have an entry signal and which direction.
	var side ds.Side
	var price decimal.Decimal
	var bandPrice decimal.Decimal
	var quoteExchange sip.Exchange
	var quoteSize int32

	if st.luldLower.IsPositive() && st.quote.AskPrice.IsPositive() &&
		st.quote.AskPrice.Cmp(st.luldLower) == 0 {
		// LIMIT DOWN: ask pinned at lower band → buy the dip
		side = ds.SideBuy
		price = st.luldLower
		bandPrice = st.luldLower
		quoteExchange = st.quote.AskExchange
		quoteSize = st.quote.AskSize
	} else if st.luldUpper.IsPositive() && st.quote.BidPrice.IsPositive() &&
		st.quote.BidPrice.Cmp(st.luldUpper) == 0 &&
		st.asset.Shortable.Load() && st.asset.EasyToBorrow.Load() {
		// LIMIT UP: bid pinned at upper band → short the top
		side = ds.SideSell
		price = st.luldUpper
		bandPrice = st.luldUpper
		quoteExchange = st.quote.BidExchange
		quoteSize = st.quote.BidSize
	} else {
		return
	}

	// Only DMA to exchanges we can route to.
	var dest alpaca.OrderDestination
	switch quoteExchange {
	case sip.ExchangeNASDAQ:
		dest = alpaca.OrderDestinationNASDAQ
	case sip.ExchangeARCA:
		dest = alpaca.OrderDestinationARCA
	case sip.ExchangeNYSE:
		dest = alpaca.OrderDestinationNYSE
	default:
		return
	}

	// Size: take all available liquidity, capped by notional limit.
	// Round down to 100-share lots.
	roundLot := cboe.LotSize(price)
	maxShares := flagBudget.Div(price).QuantizeTruncate(roundLot)
	available := decimal.FromInt(int(quoteSize)).QuantizeTruncate(roundLot)
	qty := available.Min(maxShares)
	if qty.IsZero() {
		return
	}

	// Set exit target.
	if side == ds.SideBuy {
		st.scoopTarget = price.Add(price.Mul(*flagExit))
		st.scoopShort = false
	} else {
		st.scoopTarget = price.Sub(price.Mul(*flagExit))
		st.scoopShort = true
	}
	st.scoopEntryAt = now

	log.Printf("scoop entry %s %s: %s shares @ %s on %s (LULD band=%s, target=%s)",
		side, st.symbol, qty, price, quoteExchange, bandPrice, st.scoopTarget)
	LimitOrder(st, side, qty, price, dest, session,
		fmt.Sprintf("scoop %s %s @ %s DMA %s", side, qty, price, dest))
}

func exitScoop(st *State, now clocky.Time, session cboe.Session) {
	if st.halt {
		return
	}

	isLong := st.position.IsPositive() && !st.scoopShort
	isShort := st.position.IsNegative() && st.scoopShort

	// Determine if we should be exiting.
	shouldExit := st.disabled // TUI disabled → liquidate
	if isLong && st.scoopTarget.IsPositive() && st.quote.BidPrice.IsPositive() {
		if st.quote.BidPrice.Cmp(st.scoopTarget) >= 0 {
			shouldExit = true
		}
		if *flagBailLong > 0 && st.scoopEntryAt != 0 && now.Sub(st.scoopEntryAt) > *flagBailLong {
			shouldExit = true
		}
	}
	if isShort && st.scoopTarget.IsPositive() && st.quote.AskPrice.IsPositive() {
		if st.quote.AskPrice.Cmp(st.scoopTarget) <= 0 {
			shouldExit = true
		}
		if *flagBailShort > 0 && st.scoopEntryAt != 0 && now.Sub(st.scoopEntryAt) > *flagBailShort {
			shouldExit = true
		}
	}
	if !shouldExit {
		return
	}

	// Cross the spread to get out. Smart router handles venue.
	// If we already have a pending exit, chase the market if price moved.
	if isLong {
		price := st.quote.BidPrice
		if st.sellClientOrderID == "" {
			log.Printf("scoop exit long %s: %s shares @ %s (target=%s)",
				st.symbol, st.position, price, st.scoopTarget)
			LimitOrder(st, ds.SideSell, st.position, price,
				alpaca.OrderDestinationNone, session,
				fmt.Sprintf("scoop exit %s @ %s", st.position, price))
		} else if price.Cmp(st.sellPrice) < 0 &&
			(st.lastReplaceAt == 0 || now.Sub(st.lastReplaceAt) > 1*clocky.Minute) {
			st.lastReplaceAt = now
			ReplaceOrder(st, ds.SideSell, st.position, price)
		}
	}
	if isShort {
		price := st.quote.AskPrice
		qty := st.position.Abs()
		if st.buyClientOrderID == "" {
			log.Printf("scoop exit short %s: %s shares @ %s (target=%s)",
				st.symbol, qty, price, st.scoopTarget)
			LimitOrder(st, ds.SideBuy, qty, price,
				alpaca.OrderDestinationNone, session,
				fmt.Sprintf("scoop cover %s @ %s", qty, price))
		} else if price.Cmp(st.buyPrice) > 0 &&
			(st.lastReplaceAt == 0 || now.Sub(st.lastReplaceAt) > 1*clocky.Minute) {
			st.lastReplaceAt = now
			ReplaceOrder(st, ds.SideBuy, qty, price)
		}
	}
}

func Run() Result {
	gOrders = map[string]*State{}
	gSymbols = map[symbol.Symbol]*State{}
	gIgnoreSymbols = map[symbol.Symbol]bool{}
	gFailedOrders = make(chan string, 32)
	gFailedReplaces = make(chan string, 32)
	gTotalPnL = decimal.Zero
	gTotalFees = decimal.Zero
	gTotalShares = decimal.Zero
	gSystemOn = true
	gTotalFills = 0
	gOrderCount = 0
	gOrderFails = 0
	gOrderSeq = 0
	gNextBalance = 0
	gNextVolume = 0
	gNextDecay = 0
	gBacktest = nil
	gTapeMsg = nil
	gTapeDone = nil
	if *flagLive {
		gAlpacaClient = alpaca.NewClient()
		gBroker = gAlpacaClient
	}

	// log configuration
	log.Printf("prepare to scoop")

	// periodically fetch information about supported equities from alpaca
	if *flagLive {
		go synchronizeAssetsForever()
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
			st := GetState(sym)
			if st == nil {
				continue
			}
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
					st := GetState(symbol.MustParse(name))
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

	var stockUpdates <-chan alpaca.StockUpdate
	var boatsUpdates <-chan alpaca.StockUpdate
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
		boatsUpdates = make(chan alpaca.StockUpdate)
		heartbeatChan = gBacktest.Heartbeat
		gBroker = gBacktest
	} else {
		log.Printf("subscribing to order updates...")
		orderUpdates = alpaca.OrderUpdates()
		log.Printf("subscribing to sip stock updates...")
		stockUpdates = alpaca.MustStockUpdates(alpaca.SIPWSURL, &alpaca.StockUpdatesRequest{
			Action:      "subscribe",
			Quotes:      []string{"*"},
			Trades:      []string{"*"},
			Statuses:    []string{"*"},
			Imbalances:  []string{"*"},
			LULDs:       []string{"*"},
			Bars:        []string{"*"},
			DailyBars:   []string{"*"},
			UpdatedBars: []string{"*"},
		})
		log.Printf("subscribing to boats stock updates...")
		boatsUpdates = alpaca.MustStockUpdates(alpaca.BOATSWSURL, &alpaca.StockUpdatesRequest{
			Action:   "subscribe",
			Quotes:   []string{"*"},
			Trades:   []string{"*"},
			Statuses: []string{"*"},
		})
		// start tape recorder to gcs
		now := clocky.Now()
		gTapeMsg = make(chan alpaca.StockUpdate, 65536)
		gTapeDone = make(chan struct{})
		go recordTape(now, gTapeMsg, gTapeDone)
		gLogMsg = make(chan string, 65536)
		gLogDone = make(chan struct{})
		go recordLog(now, gLogMsg, gLogDone)
	}

	// start tui
	var tuiChan <-chan TUICommand
	if *flagTUI {
		ch := make(chan TUICommand, 32)
		tui := startTUI(ch, sigChan)
		defer tui.stop()
		tuiChan = ch
	}

	// consume events
	for {
		select {
		case stockUpdate, ok := <-stockUpdates:
			if !ok {
				return shutdown()
			}
			onStockUpdate(stockUpdate)
		case stockUpdate := <-boatsUpdates:
			onStockUpdate(stockUpdate)
		case update := <-orderUpdates:
			onOrderUpdate(update)
		case clientOrderID := <-gFailedOrders:
			removeOrder(gOrders[clientOrderID], clientOrderID)
			gOrderFails++
		case clientOrderID := <-gFailedReplaces:
			undoReplaceOrder(gOrders[clientOrderID], clientOrderID)
		case <-heartbeatChan:
			onHeartbeat()
		case cmd := <-tuiChan:
			processTUICommand(cmd)
		case <-sigChan:
			return shutdown()
		}
	}
}

func onHeartbeat() {
	now := clocky.Now()
	if now.After(gNextBalance) {
		logBalance()
		gNextBalance = now.Add(kBalanceInterval)
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

func getSymbolsThatMatter() []*State {
	gMu.Lock()
	states := make([]*State, 0, len(gSymbols))
	for _, st := range gSymbols {
		if st.HasBeenTraded() {
			states = append(states, st)
		}
	}
	gMu.Unlock()
	slices.SortFunc(states, compareStateBySymbol)
	return states
}

func compareStateBySymbol(a, b *State) int {
	if a.symbol < b.symbol {
		return -1
	} else if a.symbol > b.symbol {
		return 1
	} else {
		return 0
	}
}

func removeOrder(st *State, clientOrderID string) {
	delete(gOrders, clientOrderID)
	if st.buyClientOrderID == clientOrderID {
		st.buyClientOrderID = ""
		st.buyClientOrderID2 = ""
		st.buyOrderID = ""
		st.buyPrice = decimal.Zero
		st.buyPrice2 = decimal.Zero
	}
	if st.sellClientOrderID == clientOrderID {
		st.sellClientOrderID = ""
		st.sellClientOrderID2 = ""
		st.sellOrderID = ""
		st.sellPrice = decimal.Zero
		st.sellPrice2 = decimal.Zero
	}
}

func undoReplaceOrder(st *State, clientOrderID string) {
	delete(gOrders, clientOrderID)
	if st.buyClientOrderID2 == clientOrderID {
		st.buyClientOrderID2 = ""
		st.buyPrice2 = decimal.Zero
	}
	if st.sellClientOrderID2 == clientOrderID {
		st.sellClientOrderID2 = ""
		st.sellPrice2 = decimal.Zero
	}
}

func onStockUpdate(stockUpdate alpaca.StockUpdate) {
	if gTapeMsg != nil {
		gTapeMsg <- stockUpdate
	}
	st := GetState(stockUpdate.Message.Symbol)
	if st == nil {
		return
	}
	switch stockUpdate.Message.Type {
	case sip.MessageTypeQuote:
		onQuote(st, stockUpdate.Message.Quote())
	case sip.MessageTypeTrade:
		onTrade(st, stockUpdate.Message.Trade())
	case sip.MessageTypeStatus:
		onStatus(st, stockUpdate.Message.Status())
	case sip.MessageTypeLULD:
		onLULD(st, stockUpdate.Message.LULD())
	case sip.MessageTypeImbalance:
		onImbalance(st, stockUpdate.Message.Imbalance())
	case sip.MessageTypeBar, sip.MessageTypeDailyBar, sip.MessageTypeUpdatedBar:
		onBar(st, stockUpdate.Message.Bar())
	}
}

func onBar(st *State, b *sip.Bar) {
}

func onLULD(st *State, l *sip.LULD) {
	if l.Indicator.IsPriceBand() {
		st.luldLower = l.LowerLimit
		st.luldUpper = l.UpperLimit
		st.luldAt = l.Timestamp
	} else {
		// Limit state indicator (C-J): bands are zeroed.
		st.luldLower = decimal.Zero
		st.luldUpper = decimal.Zero
	}
	Evaluate(st)
}

func onImbalance(st *State, b *sip.Imbalance) {
	st.imbPrice = b.Price
}

func onQuote(st *State, q *sip.Quote) {
	st.quote = q
	Evaluate(st)
}

func onTrade(st *State, t *sip.Trade) {
	if t.Price.IsPositive() {
		st.lastTrade = t.Price
		st.lastTradeAt = t.Timestamp
		st.tradeCount++
	}
	Evaluate(st)
}

func onStatus(st *State, s *sip.Status) {
	if s.Halt() {
		st.halt = true
		log.Printf("trading of %s has halted: %s", st.symbol, s)
	} else if s.Resume() {
		st.halt = false
		log.Printf("trading of %s has resumed: %s", st.symbol, s)
		Evaluate(st) // try to exit on resumption
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
				DisplayQty:  cboe.LotSize(price),
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
		// order ID not yet known (haven't received 'new' event);
		// clean up so we can try again on the next tick
		if side == ds.SideBuy {
			st.buyClientOrderID2 = ""
		} else {
			st.sellClientOrderID2 = ""
		}
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

func logBalance() {
	longNotional := decimal.Zero
	shortNotional := decimal.Zero
	longCount := 0
	shortCount := 0
	pendingCount := 0
	totalUnrealized := decimal.Zero
	liquidationPnL := decimal.Zero
	for _, st := range getSymbolsThatMatter() {
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
	if _, err := gBroker.CancelAllOrders(); err != nil {
		log.Printf("error canceling orders: %v", err)
	}
	log.Printf("=== P&L SUMMARY ===")
	totalUnrealized := decimal.Zero
	for _, st := range getSymbolsThatMatter() {
		if st.totalBought.IsZero() && st.totalSold.IsZero() || st.quote == nil {
			continue
		}
		unrealizedPnL := decimal.Zero
		if !st.position.IsZero() && !st.costBasis.IsZero() {
			mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
			unrealizedPnL = mid.Mul(st.position).Sub(st.costBasis)
		}
		equity := st.realizedPnL.Sub(st.totalFees).Add(unrealizedPnL)
		totalUnrealized = totalUnrealized.Add(unrealizedPnL)
		log.Printf("  %8s: pos=%-8s equity=%-10s realized=%-8s unrealized=%-8s fees=%-10s bought=%-8s sold=%-8s",
			st.symbol, st.position, equity.Format(2),
			st.realizedPnL.Format(2), unrealizedPnL.Format(2),
			st.totalFees.Format(2), st.totalBought, st.totalSold)
	}
	net := gTotalPnL.Sub(gTotalFees)
	totalEquity := net.Add(totalUnrealized)
	log.Printf("  TOTAL equity: %s  realized: %s  unrealized: %s  fees: %s  net: %s",
		totalEquity, gTotalPnL, totalUnrealized, gTotalFees, net)
	log.Printf("  total fills: %d  symbols tracked: %d", gTotalFills, len(gSymbols))
	if gTapeMsg != nil {
		close(gTapeMsg)
		gTapeMsg = nil
		<-gTapeDone
	}
	if gLogMsg != nil {
		close(gLogMsg)
		gLogMsg = nil
		<-gLogDone
	}
	return Result{
		PnL:    gTotalPnL,
		Fees:   gTotalFees,
		Net:    net,
		Fills:  gTotalFills,
		Shares: gTotalShares,
		Equity: totalEquity,
	}
}

func processTUICommand(cmd TUICommand) {
	st := gSymbols[cmd.symbol]
	if st == nil {
		return
	}
	switch cmd.kind {
	case tuiCmdToggle:
		st.disabled = !st.disabled
		if st.disabled {
			if st.buyOrderID != "" {
				id := st.buyOrderID
				spawn(func() { gBroker.CancelOrder(id) })
			}
			if st.sellOrderID != "" {
				id := st.sellOrderID
				spawn(func() { gBroker.CancelOrder(id) })
			}
			log.Printf("%s DISABLED", st.symbol)
		} else {
			log.Printf("%s ENABLED", st.symbol)
			Evaluate(st)
		}
	case tuiCmdAdjust:
		// no adjustable fields in scoop mode
	case tuiCmdCancel:
		if st.buyOrderID != "" {
			id := st.buyOrderID
			spawn(func() { gBroker.CancelOrder(id) })
		}
		if st.sellOrderID != "" {
			id := st.sellOrderID
			spawn(func() { gBroker.CancelOrder(id) })
		}
		log.Printf("%s CANCEL orders", st.symbol)
	case tuiCmdVenue:
		switch st.config.venue {
		case alpaca.OrderDestinationNASDAQ:
			st.config.venue = alpaca.OrderDestinationARCA
		case alpaca.OrderDestinationARCA:
			st.config.venue = alpaca.OrderDestinationNYSE
		case alpaca.OrderDestinationNYSE:
			st.config.venue = alpaca.OrderDestinationNone
		default:
			st.config.venue = alpaca.OrderDestinationNASDAQ
		}
		log.Printf("%s venue: %s", st.symbol, st.config.venue)
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

func GetState(sym symbol.Symbol) *State {
	st := gSymbols[sym]
	if st == nil {
		asset := GetAsset(sym)
		if asset != nil {
			st = &State{
				symbol: sym,
				asset:  asset,
			}
			gMu.Lock()
			gSymbols[sym] = st
			gMu.Unlock()
		}
	}
	return st
}

func generateClientOrderID() string {
	if *flagLive {
		return uuid.New().String()
	}
	gOrderSeq++
	return fmt.Sprintf("order-%d", gOrderSeq)
}

func spawn(f func()) {
	if *flagLive {
		go f()
	} else {
		f()
	}
}
