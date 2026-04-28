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
	"syscall"
	"time"

	"github.com/google/uuid"
)

var (
	flagSymbol    = symbol.Flag("symbol", "XE", "stock to trade")
	flagExchange  = alpaca.OrderDestinationFlag("exchange", "NASDAQ", "destination exchange for orders")
	flagTarget    = decimal.Flag("target", "600", "target inventory in shares (defines bias; double this sets upper limit)")
	flagQty       = decimal.Flag("qty", "200", "quantity of shares per trade")
	flagSpread    = decimal.Flag("spread", "0.04", "base half spread to use when market making")
	flagDrift     = decimal.Flag("drift", "0.10", "distance from nbbo before replace order is sent")
	flagCooldown  = clocky.DurationFlag("cooldown", "30m", "cooldown period after order rejection")
	flagBucket    = flag.String("bucket", "dropbear-sip", "google cloud storage bucket for recording market data")
	flagLive      = flag.Bool("live", false, "enables live trading and network access")
	flagExtended  = flag.Bool("extended", false, "enables extended hours trading")
	flagOvernight = flag.Bool("overnight", false, "enables overnight hours trading")
	flagData      = flag.String("data", "", "path of sip data file for backtest")
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
	gExchange       sip.Exchange
	gTotalFills     int
	gOrderCount     int64
	gOrderFails     int64
	gTapeMsg        chan *sip.Message
	gTapeDone       chan struct{}
	gBacktest       *Backtest
	gAlpacaClient   *alpaca.Client
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

	if flagTarget.IsZero() {
		fmt.Fprintf(os.Stderr, "-target must be non-zero\n")
		os.Exit(1)
	}
	if !*flagLive && *flagData == "" {
		fmt.Fprintf(os.Stderr, "-data must be specified for backtest mode\n")
		os.Exit(1)
	}

	// figure out sip exchange
	switch *flagExchange {
	case alpaca.OrderDestinationNone:
		gExchange = sip.ExchangeNone
	case alpaca.OrderDestinationARCA:
		gExchange = sip.ExchangeARCA
	case alpaca.OrderDestinationNASDAQ:
		gExchange = sip.ExchangeNASDAQ
	case alpaca.OrderDestinationNYSE:
		gExchange = sip.ExchangeNYSE
	default:
		fmt.Fprintf(os.Stderr, "unsupported exchange: %s\n", *flagExchange)
		os.Exit(1)
	}

	// log configuration
	log.Printf("prepare to make markets")
	log.Printf("  symbol=%s", *flagSymbol)
	log.Printf("  exchange=%s", *flagExchange)
	log.Printf("  target=%s", *flagTarget)

	// initialize globals
	gOrders = map[string]*State{}
	gSymbols = map[symbol.Symbol]*State{}
	gIgnoreSymbols = map[symbol.Symbol]bool{}
	gFailedOrders = make(chan string, 32)
	gFailedReplaces = make(chan string, 32)
	if *flagLive {
		gAlpacaClient = alpaca.NewClient()
		gBroker = gAlpacaClient
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
		log.Printf("subscribing to sip stock updates...")
		stockUpdates = alpaca.MustStockUpdates(alpaca.SIPWSURL, &alpaca.StockUpdatesRequest{
			Action:     "subscribe",
			Quotes:     []string{flagSymbol.String()},
			Trades:     []string{flagSymbol.String()},
			Statuses:   []string{flagSymbol.String()},
			Imbalances: []string{flagSymbol.String()},
			LULDs:      []string{flagSymbol.String()},
		})
		log.Printf("subscribing to boats stock updates...")
		boatsUpdates = alpaca.MustStockUpdates(alpaca.BOATSWSURL, &alpaca.StockUpdatesRequest{
			Action:   "subscribe",
			Quotes:   []string{flagSymbol.String()},
			Trades:   []string{flagSymbol.String()},
			Statuses: []string{flagSymbol.String()},
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
			shutdown()
			return
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
		time.Sleep(15 * time.Minute)
	}
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
		st.buyClientOrderID2 = ""
		st.buyOrderID = ""
	}
	if st.sellClientOrderID == clientOrderID {
		st.sellClientOrderID = ""
		st.sellClientOrderID2 = ""
		st.sellOrderID = ""
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

	log.Printf("%s order %s: %s price=%s qty=%s pos=%s filled=%s/%s",
		st.symbol, update.Event, update.Order.Status,
		update.Price, update.Qty, update.PositionQty,
		update.Order.FilledQty, update.Order.Qty)

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
		} else {
			st.sellOrderID = update.Order.ReplacedBy
			delete(gOrders, st.sellClientOrderID)
			st.sellClientOrderID = st.sellClientOrderID2
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
	if st.halt {
		return
	}
	if st.quote == nil {
		return
	}
	if st.quote.Symbol != *flagSymbol {
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

	qty := *flagQty
	spread := *flagSpread
	target := *flagTarget
	alreadyBuying := st.buyClientOrderID != ""
	alreadySelling := st.sellClientOrderID != ""
	mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
	var canBuy, canSell bool
	var maximumPosition, minimumPosition decimal.Decimal
	if target.IsPositive() {
		minimumPosition = decimal.Zero
		maximumPosition = target.MulInt(2)
	} else {
		minimumPosition = target.MulInt(2)
		maximumPosition = decimal.Zero
	}
	canBuy = st.position.Add(qty).Cmp(maximumPosition) <= 0 && !alreadyBuying
	canSell = st.position.Sub(qty).Cmp(minimumPosition) >= 0 && !alreadySelling
	buyPrice := mid.Sub(spread)
	sellPrice := mid.Add(spread)

	if canBuy {
		st.buyPrice = buyPrice
		LimitOrder(st, ds.SideBuy, qty, buyPrice, *flagExchange, session, "")
	} else if alreadyBuying && st.buyClientOrderID2 == "" {
		if buyPrice.Sub(st.buyPrice).Abs().Cmp(*flagDrift) >= 0 {
			ReplaceOrder(st, ds.SideBuy, qty, buyPrice)
		}
	}

	if canSell {
		st.sellPrice = sellPrice
		LimitOrder(st, ds.SideSell, qty, sellPrice, *flagExchange, session, "")
	} else if alreadySelling && st.sellClientOrderID2 == "" {
		if sellPrice.Sub(st.sellPrice).Abs().Cmp(*flagDrift) >= 0 {
			ReplaceOrder(st, ds.SideSell, qty, sellPrice)
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
	go func() {
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
	}()
}

func ReplaceOrder(st *State, side ds.Side, qty decimal.Decimal, price decimal.Decimal) {
	quote := st.quote
	oldBuyPrice := st.buyPrice
	clientOrderID := generateClientOrderID()
	var orderID string
	if side == ds.SideBuy {
		st.buyPrice2 = price
		st.buyClientOrderID2 = clientOrderID
		orderID = st.buyOrderID
	} else {
		st.sellPrice2 = price
		st.sellClientOrderID2 = clientOrderID
		orderID = st.sellOrderID
	}
	if orderID == "" {
		return
	}
	gOrders[clientOrderID] = st
	go func() {
		_, err := gBroker.ReplaceOrder(orderID, &alpaca.ReplaceOrderRequest{
			Qty:           qty,
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
		log.Printf("%s replacing buy order: %s -> %s (pos=%s spread=%s bid=%sx%d ask=%sx%d)",
			st.symbol, oldBuyPrice, price,
			st.position, quote.AskPrice.Sub(quote.BidPrice),
			quote.BidPrice, quote.BidSize,
			quote.AskPrice, quote.AskSize)
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

func shutdown() {
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
	fmt.Printf("total P&L: %s  fees: %s  net: %s  fills: %d  shares: %s\n", gTotalPnL, gTotalFees, net, gTotalFills, gTotalShares)
	os.Exit(0)
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
