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
	"dropbear/loggy"
	"dropbear/symbol"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
)

var (
	flagSize      = decimal.Flag("size", "1_000", "how much capital to devote to each trade")
	flagRisk      = decimal.Flag("risk", "5_000", "maximum portfolio exposure in usd before index hedging")
	flagHedge     = symbol.Flag("hedge", "IWM", "symbol to use for index hedging when risk threshold is breached")
	flagFloor     = decimal.Flag("floor", "2", "minimum trade quantity in shares (2+ ensures negative fees)")
	flagMaxSyms   = flag.Int("maxsyms", 150, "maximum number of symbols to actively trade simultaneously")
	flagGreed     = decimal.FlagBPS("greed", "1", "amount of basis points to demand over cost basis")
	flagMinEdge   = decimal.Flag("minedge", "3", "minimum spread in ticks")
	flagMinPrice  = decimal.Flag("minprice", "10", "minimum price of stock in usd to trade")
	flagMaxPrice  = decimal.Flag("maxprice", "100", "maximum price of stock in usd to trade")
	flagThreshold = decimal.Flag("threshold", "0.9", "imbalance ratio threshold to trigger (0-1)")
	flagISOShares = decimal.Flag("iso", "200", "net ISO shares threshold to trigger entry")
	flagUnload    = clocky.DurationFlag("unload", "60m", "time before day session close to switch to exit-only (0 to disable)")
	flagFlatten   = clocky.DurationFlag("flatten", "11m", "time before day session close to flatten positions with moc orders (0 to disable)")
	flagPatience  = clocky.DurationFlag("patience", "5m", "time to wait before canceling unfilled orders (0 to disable)")
	flagBucket    = flag.String("bucket", "dropbear-sip", "google cloud storage bucket for recording market data")
	flagExitOnly  = flag.Bool("exit", false, "exit-only mode: close existing positions, no new entries")
)

var (
	kStandardMarginRate = decimal.Parse("0.3")
	kHeartbeatInterval  = 1 * clocky.Second
	kBalanceInterval    = 5 * clocky.Second
)

var (
	gClient        *alpaca.Client
	gOrders        map[string]*State
	gSymbols       map[symbol.Symbol]*State
	gActiveSymbols map[symbol.Symbol]bool
	gFailedOrders  chan string
	gNextBalance   clocky.Time
	gTotalPnL      decimal.Decimal
	gTotalFees     decimal.Decimal
	gTotalFills    int
	gQuoteCount    int64
	gOrderCount    int64
	gOrderFails    int64
	gPhase         Phase
	gTapeMsg       chan *sip.Message
	gTapeDone      chan struct{}
)

func main() {
	loggy.Init()
	flag.Parse()
	loggy.AlsoLogToFile()

	if !flagSize.IsPositive() {
		fmt.Fprintf(os.Stderr, "-size must be positive\n")
		os.Exit(1)
	}
	if flagRisk.IsNegative() {
		fmt.Fprintf(os.Stderr, "-risk can't be negative\n")
		os.Exit(1)
	}
	if !flagFloor.IsPositive() {
		fmt.Fprintf(os.Stderr, "-floor must be positive\n")
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

	// log configuration
	log.Printf("prepare to trade")
	log.Printf("  size=%s", *flagSize)
	log.Printf("  risk=%s", *flagRisk)
	log.Printf("  hedge=%s", *flagHedge)
	log.Printf("  floor=%s", *flagFloor)
	log.Printf("  maxsyms=%d", *flagMaxSyms)
	log.Printf("  greed=%s", *flagGreed)
	log.Printf("  minedge=%s", *flagMinEdge)
	log.Printf("  minprice=%s", *flagMinPrice)
	log.Printf("  threshold=%s", *flagThreshold)
	log.Printf("  iso=%s", *flagISOShares)
	log.Printf("  unload=%s", *flagUnload)
	log.Printf("  flatten=%s", *flagFlatten)
	log.Printf("  patience=%s", *flagPatience)
	log.Printf("  exitonly=%t", *flagExitOnly)

	// initialize globals
	gClient = alpaca.NewClient()
	gOrders = map[string]*State{}
	gSymbols = map[symbol.Symbol]*State{}
	gActiveSymbols = map[symbol.Symbol]bool{}
	gFailedOrders = make(chan string, 32)
	gNextBalance = clocky.Now().Add(kBalanceInterval)

	// periodically fetch information about supported equities from alpaca
	go synchronizeAssetsForever()

	// cancel lingering orders
	log.Printf("canceling lingering orders...")
	if err := gClient.CancelAllOrders(); err != nil {
		log.Printf("warning: error canceling orders: %v", err)
	}

	// load existing positions
	var symbols []string
	log.Printf("loading positions...")
	positions, err := gClient.GetPositions()
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
		quotes, err := gClient.GetQuotes(symbols, feed)
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

	log.Printf("subscribing to order updates...")
	orderUpdates := alpaca.OrderUpdates()

	log.Printf("subscribing to sip stock updates...")
	stockUpdates := alpaca.MustStockUpdates(alpaca.SIPWSURL, &alpaca.StockUpdatesRequest{
		Action:     "subscribe",
		Quotes:     []string{"*"},
		Trades:     []string{"*"},
		Statuses:   []string{"*"},
		Imbalances: []string{"*"},
		LULDs:      []string{"*"},
	})

	log.Printf("subscribing to boats stock updates...")
	boatsUpdates := alpaca.MustStockUpdates(alpaca.BOATSWSURL, &alpaca.StockUpdatesRequest{
		Action:   "subscribe",
		Quotes:   []string{"*"},
		Trades:   []string{"*"},
		Statuses: []string{"*"},
	})

	// start tape recorder to gcs
	gTapeMsg = make(chan *sip.Message, 65536)
	gTapeDone = make(chan struct{})
	go recordTape(gTapeMsg, gTapeDone)

	// catch ctrl-c
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// create timers
	heartbeat := clocky.NewTicker(kHeartbeatInterval)
	defer heartbeat.Stop()

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
		case <-heartbeat.C:
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

	// manage our risk
	manageHedge()

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
				log.Printf("we shall flatten our positions")
				flatten()
			} else {
				gPhase = PhaseCanceling
				log.Printf("we shall cancel all orders so we can flatten")
				if err := gClient.CancelAllOrders(); err != nil {
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
		if err := gClient.SyncAssets(); err != nil {
			log.Printf("error synchronizing assets: %v", err)
		}
		time.Sleep(15 * time.Minute)
	}
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

func TradeQuantity(price, margin, maximum decimal.Decimal) decimal.Decimal {
	// alpaca's overnight margin requirement is normally 0.3 but we've seen it go as
	// high as 2.0 for risky stocks. penny stocks (anything under $5) usually have a
	// margin requirement of 1.0. leveraged etfs will usually be 0.6 to 0.9. so what
	// happens here is as margin grows above 0.3 we scale down how much size it uses
	rat := kStandardMarginRate.Div(margin)
	qty := flagSize.Mul(rat).Div(price)
	// we don't trade fractional shares
	qty = qty.Truncate()
	// trade in round lots if we have at least that
	lot := cboe.LotSize(price)
	if qty.Cmp(lot) >= 0 {
		qty = qty.QuantizeTruncate(lot)
	}
	// we don't want to take too much liquidity
	qty = qty.Min(maximum)
	// each order needs at minimum two shares for commissions+fees to go negative
	if qty.Cmp(*flagFloor) < 0 {
		return decimal.Zero // stock is probably very expensive like meta
	}
	return qty
}

func onMessage(msg *sip.Message) {
	gTapeMsg <- msg
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
	gQuoteCount++
	st.quote = q
	Evaluate(st)
}

func onTrade(t *sip.Trade) {
	if !t.Conditions.Has(sip.TradeCondISO) {
		return
	}
	st := gSymbols[t.Symbol]
	if st == nil {
		return
	}
	// classify direction using Lee-Ready:
	// trade above midpoint = buyer-initiated
	// trade below midpoint = seller-initiated
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
		fillPnL := decimal.Zero
		oldPos := st.position
		reducing := (oldPos.IsPositive() && signedQty.IsNegative()) ||
			(oldPos.IsNegative() && signedQty.IsPositive())

		if reducing && !oldPos.IsZero() {
			// closing (partially or fully): realize P&L
			avgCost := st.costBasis.Div(oldPos)  // per-share cost (signed correctly)
			closeQty := absQty.Min(oldPos.Abs()) // don't realize more than we hold
			if oldPos.IsPositive() {
				// closing long: profit = (sell price - avg cost) * qty
				fillPnL = fillPrice.Sub(avgCost).Mul(closeQty)
			} else {
				// covering short: profit = (sale price - buy price) * qty
				// avgCost = costBasis / position = negative / negative = positive sale price
				fillPnL = avgCost.Sub(fillPrice).Mul(closeQty)
			}
			st.realizedPnL = st.realizedPnL.Add(fillPnL)
			gTotalPnL = gTotalPnL.Add(fillPnL)
			// reduce cost basis proportionally
			costReduction := avgCost.Mul(closeQty)
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
		fee := EstimateFee(update.Order.Side, absQty, fillPrice, firstFill)
		st.totalFees = st.totalFees.Add(fee)
		gTotalFees = gTotalFees.Add(fee)

		st.position = update.PositionQty
		gTotalFills++

		log.Printf("%s filled %s @ %s | pos=%s | fill_pnl=%s | fee=%s | realized=%s | total_pnl=%s | total_fees=%s | fills=%d",
			st.symbol, signedQty, fillPrice, st.position,
			fillPnL, fee, st.realizedPnL, gTotalPnL, gTotalFees, gTotalFills)
	}

	// clean up completed orders
	if update.Order.Status.IsFinal() {
		removeOrder(st, update.Order.ClientOrderID)
		Evaluate(st)
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

	// check if market is open
	now := clocky.Now()
	session := cboe.GetSession(now)
	switch session {
	case cboe.SessionClosed:
		return
	case cboe.SessionOvernight:
		if !st.asset.OvernightTradable.Load() || st.asset.OvernightHalted.Load() {
			return
		}
	}

	// determine if we should be greedy
	wantGreed := !st.greed.IsZero() && !st.costBasis.IsZero()
	noGreed := gPhase == PhaseExitOnly
	greedy := wantGreed && !noGreed

	// PRIORITY 1: Exit existing positions at a profit.
	// Post AT the bid/ask (don't improve) so we capture the spread.
	// Entry improves by a penny to get priority; exit waits at the edge.
	// With -winner, floor the exit price at avg entry + a tick to guarantee profit.
	if st.position.IsPositive() && st.sellClientOrderID == "" && st.asset.Tradable.Load() && !st.quote.DangerousAsk() {
		price := st.quote.AskPrice
		if greedy {
			minPrice := st.costBasis.Div(st.position).Mul(decimal.One.Add(st.greed))
			price = price.Max(minPrice)
			price = QuantizeSellPrice(price)
		}
		dest := BestDestination(st.quote.AskExchange, st.asset, session)
		qty := cboe.LotSize(price).Min(st.position)
		LimitOrder(st, ds.SideSell, qty, price, dest, session)
		st.greed = st.greed.Half()
		return
	}
	if st.position.IsNegative() && st.buyClientOrderID == "" && st.asset.Tradable.Load() && !st.quote.DangerousBid() {
		price := st.quote.BidPrice
		if greedy {
			// short: avg entry = costBasis / position = negative / negative = positive
			maxPrice := st.costBasis.Div(st.position).Mul(decimal.One.Sub(st.greed))
			price = price.Min(maxPrice)
			price = QuantizeBuyPrice(price)
		}
		dest := BestDestination(st.quote.BidExchange, st.asset, session)
		qty := cboe.LotSize(price).Min(st.position.Neg())
		LimitOrder(st, ds.SideBuy, qty, price, dest, session)
		st.greed = st.greed.Half()
		return
	}

	// PRIORITY 2: Enter new positions based on imbalance signal.
	// Only when flat (no position) and no pending orders.
	if *flagExitOnly || gPhase == PhaseExitOnly {
		return
	}
	if len(gActiveSymbols) >= *flagMaxSyms {
		return
	}
	if st.Active() {
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

	// imbalance = (bid size - ask size) / (bid size + ask size)
	imbalance := bidSize.Sub(askSize).Div(totalSize)

	// entry signals: quote imbalance OR ISO sweep momentum
	wantBuy := imbalance.Cmp(*flagThreshold) > 0 ||
		st.isoNetFlow.Cmp(*flagISOShares) > 0
	wantSell := (imbalance.Cmp(flagThreshold.Neg()) < 0 ||
		st.isoNetFlow.Cmp(flagISOShares.Neg()) < 0) &&
		st.asset.EasyToBorrow.Load()

	if wantBuy {
		price := st.quote.BidPrice.Add(Tick(st.quote.BidPrice))           // try to outbid
		price = price.Min(st.quote.AskPrice.Sub(Tick(st.quote.AskPrice))) // but don't take
		if price.Cmp(*flagMinPrice) >= 0 && price.Cmp(*flagMaxPrice) < 0 {
			mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
			mar := st.asset.MarginRequirementLong.Load()
			qty := TradeQuantity(mid, mar, totalSize)
			dst := BestDestination(st.quote.BidExchange, st.asset, session)
			LimitOrder(st, ds.SideBuy, qty, price, dst, session)
			st.isoNetFlow = decimal.Zero
			st.greed = *flagGreed
		}
	} else if wantSell {
		price := st.quote.AskPrice.Sub(Tick(st.quote.AskPrice))           // try to outbid
		price = price.Max(st.quote.BidPrice.Add(Tick(st.quote.BidPrice))) // but don't take
		if price.Cmp(*flagMinPrice) >= 0 && price.Cmp(*flagMaxPrice) < 0 {
			mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
			mar := st.asset.MarginRequirementShort.Load()
			qty := TradeQuantity(mid, mar, totalSize)
			dst := BestDestination(st.quote.AskExchange, st.asset, session)
			LimitOrder(st, ds.SideSell, qty, price, dst, session)
			st.isoNetFlow = decimal.Zero
			st.greed = *flagGreed
		}
	}
}

func BestDestination(exchange sip.Exchange, asset *alpaca.Asset, session cboe.Session) alpaca.OrderDestination {
	// overnight trading happens on boats which we can't dma
	// just let alpaca smart router figure things out for us
	if session == cboe.SessionOvernight {
		return alpaca.OrderDestinationNone
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

func LimitOrder(st *State, side ds.Side, qty decimal.Decimal, price decimal.Decimal, dest alpaca.OrderDestination, session cboe.Session) {
	if qty.IsZero() {
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

	log.Printf("%s placing %s %s @ %s -> %s (pos=%s spread=%s bid=%sx%d ask=%sx%d)",
		st.symbol, side, qty, price, dest,
		st.position, st.quote.AskPrice.Sub(st.quote.BidPrice),
		st.quote.BidPrice, st.quote.BidSize,
		st.quote.AskPrice, st.quote.AskSize)

	// don't slow down quote processing
	go func() {

		// configure alpaca smart router
		var advancedInstructions *alpaca.AdvancedInstructions
		if dest != alpaca.OrderDestinationNone {
			displayQty := decimal.Zero
			lot := cboe.LotSize(price)
			if qty.Cmp(lot) > 0 {
				// don't reveal our full intent
				displayQty = lot
			}
			advancedInstructions = &alpaca.AdvancedInstructions{
				Algorithm:   alpaca.OrderAlgorithmDMA,
				Destination: dest,
				DisplayQty:  displayQty,
			}
		}

		_, err := gClient.CreateOrder(&alpaca.CreateOrderRequest{
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
			log.Printf("%s error placing order: %v", st.symbol, err)
			gFailedOrders <- clientOrderID
			return
		}
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
		_, err := gClient.CreateOrder(&alpaca.CreateOrderRequest{
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
	for _, st := range gSymbols {
		if st.position.IsPositive() {
			mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
			if mid.IsZero() {
				mid = st.costBasis.Div(st.position) // fallback to avg cost
			}
			notional := mid.Mul(st.position)
			longNotional = longNotional.Add(notional)
			longCount++
			totalUnrealized = totalUnrealized.Add(notional.Sub(st.costBasis))
		} else if st.position.IsNegative() {
			mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
			if mid.IsZero() {
				mid = st.costBasis.Div(st.position)
			}
			notional := mid.Mul(st.position.Neg())
			shortNotional = shortNotional.Add(notional)
			shortCount++
			totalUnrealized = totalUnrealized.Add(st.position.Mul(mid).Sub(st.costBasis))
		}
		if st.buyClientOrderID != "" || st.sellClientOrderID != "" {
			pendingCount++
		}
	}
	net := gTotalPnL.Sub(gTotalFees)
	log.Printf("BALANCE long=$%s (%d) short=$%s (%d) expose=$%s | realize=%s fee=%s net=%s unrealized=%s | fills=%d pending=%d quotes=%d | ofails=%d/%d",
		longNotional.Format(0), longCount,
		shortNotional.Format(0), shortCount,
		longNotional.Sub(shortNotional).Format(0),
		gTotalPnL, gTotalFees, net, totalUnrealized,
		gTotalFills, pendingCount, gQuoteCount,
		gOrderFails, gOrderCount)
}

func shutdown() {
	log.Printf("shutting down... canceling all orders")
	if err := gClient.CancelAllOrders(); err != nil {
		log.Printf("error canceling orders: %v", err)
	}
	log.Printf("flushing tape...")
	close(gTapeMsg)
	<-gTapeDone
	log.Printf("=== P&L SUMMARY ===")
	for _, st := range gSymbols {
		if st.totalBought.IsZero() && st.totalSold.IsZero() {
			continue
		}
		unrealizedPnL := decimal.Zero
		if !st.position.IsZero() && !st.costBasis.IsZero() {
			mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
			unrealizedPnL = mid.Mul(st.position).Sub(st.costBasis)
		}
		log.Printf("  %s: pos=%s realized=%s unrealized=%s fees=%s bought=%s sold=%s",
			st.symbol, st.position, st.realizedPnL, unrealizedPnL, st.totalFees,
			st.totalBought, st.totalSold)
	}
	net := gTotalPnL.Sub(gTotalFees)
	log.Printf("  TOTAL realized P&L: %s  fees: %s  net: %s", gTotalPnL, gTotalFees, net)
	log.Printf("  total fills: %d  quotes processed: %d  symbols tracked: %d",
		gTotalFills, gQuoteCount, len(gSymbols))
	fmt.Printf("total P&L: %s  fees: %s  net: %s  fills: %d\n", gTotalPnL, gTotalFees, net, gTotalFills)
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
	if err := gClient.CancelOrder(orderID); err != nil {
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
	asset := alpaca.GetAsset(sym)
	if asset == nil {
		return nil
	}
	if asset.Exchange == alpaca.ExchangeOTC ||
		asset.Class != alpaca.AssetClassUSEquity ||
		asset.Status.Load() != alpaca.AssetStatusActive ||
		asset.PTPNoException.Load() ||
		asset.PTPWithException.Load() {
		return nil
	}
	return asset
}
