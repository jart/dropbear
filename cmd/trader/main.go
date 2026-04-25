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
	"dropbear/osi"
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
	flagSize      = decimal.Flag("size", "1000", "how much money to devote to each trade")
	flagFloor     = decimal.Flag("floor", "2", "minimum trade quantity in shares (2+ ensures negative fees)")
	flagMaxSyms   = flag.Int("maxsyms", 150, "maximum number of symbols to trade simultaneously")
	flagGreed     = decimal.FlagBPS("greed", "0", "amount of basis points to demand over cost basis")
	flagMinEdge   = decimal.Flag("minedge", "0.02", "minimum spread in usd to act")
	flagMinPrice  = decimal.Flag("minprice", "0.5", "minimum price in usd to trade")
	flagThreshold = decimal.Flag("threshold", "0.3", "imbalance ratio threshold to trigger (0-1)")
	flagExitOnly  = flag.Bool("exit", false, "exit-only mode: close existing positions, no new entries")
)

var (
	kStandardMarginRate = decimal.Parse("0.3")
)

var (
	gClient        *alpaca.Client
	gSymbols       map[string]*State
	gOrderSymbols  map[string]string // client order ID -> symbol
	gActiveSymbols map[string]bool
	gFailedOrders  chan string
	gTotalPnL      decimal.Decimal
	gTotalFees     decimal.Decimal
	gTotalFills    int
	gQuoteCount    int64
)

func main() {
	loggy.Init()
	flag.Parse()
	loggy.AlsoLogToFile()

	gClient = alpaca.NewClient()
	gSymbols = map[string]*State{}
	gOrderSymbols = map[string]string{}
	gFailedOrders = make(chan string, 32)

	// keep assets synchronized
	log.Printf("syncing assets from Alpaca...")
	if err := gClient.SyncAssets(); err != nil {
		log.Printf("error syncing assets: %v", err)
	}
	go func() {
		for {
			time.Sleep(15 * time.Minute)
			if err := gClient.SyncAssets(); err != nil {
				log.Printf("error syncing assets: %v", err)
			}
		}
	}()

	// load existing positions
	log.Printf("loading positions...")
	positions, err := gClient.GetPositions()
	if err != nil {
		log.Fatalf("error getting positions: %v", err)
	}
	for _, pos := range positions {
		if isOptionsSymbol(pos.Symbol) {
			continue
		}
		sym, err := symbol.Parse(pos.Symbol)
		if err != nil {
			continue
		}
		asset := alpaca.GetAsset(sym)
		if asset == nil {
			continue
		}
		st := getOrCreateSymbol(pos.Symbol, asset)
		st.position = pos.Qty
		st.costBasis = pos.CostBasis
		if st.Active() {
			gActiveSymbols[pos.Symbol] = true
		}
		log.Printf("loaded position for %s: %s shares @ avg %s (cost basis %s)",
			pos.Symbol, pos.Qty, pos.AvgEntryPrice, pos.CostBasis)
	}

	// cancel stale open orders
	log.Printf("canceling stale orders...")
	if err := gClient.CancelAllOrders(); err != nil {
		log.Printf("warning: error canceling orders: %v", err)
	}

	// determine which feed to use based on session
	now := clocky.Now()
	quoteFeed := alpaca.FeedSIP
	if cboe.IsOvernight(now) {
		quoteFeed = alpaca.FeedBOATS
	}

	// fetch quotes for existing positions and place exit orders
	posSymbols := make([]string, 0, len(gSymbols))
	for name := range gSymbols {
		posSymbols = append(posSymbols, name)
	}
	if len(posSymbols) > 0 {
		log.Printf("fetching quotes for %d existing positions...", len(posSymbols))
		quotes, err := gClient.GetQuotes(posSymbols, quoteFeed)
		if err != nil {
			log.Printf("warning: error fetching quotes: %v", err)
		} else {
			for name, q := range quotes {
				st := gSymbols[name]
				if st == nil || q.BidPrice.IsZero() || q.AskPrice.IsZero() {
					continue
				}
				st.quote = q
			}
		}
	}

	// subscribe to order updates
	log.Printf("subscribing to order updates...")
	orderUpdates := alpaca.OrderUpdates()

	// subscribe to quote firehose
	log.Printf("subscribing to sip stock updates...")
	stockUpdates := alpaca.MustStockUpdates(alpaca.SIPWSURL, &alpaca.StockUpdatesRequest{
		Action: "subscribe",
		Quotes: []string{"*"},
	})
	log.Printf("subscribing to boats stock updates...")
	boatsUpdates := alpaca.MustStockUpdates(alpaca.BOATSWSURL, &alpaca.StockUpdatesRequest{
		Action: "subscribe",
		Quotes: []string{"*"},
	})

	// catch ctrl-c
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Printf("trader started: maxsyms=%d minedge=%s threshold=%s",
		*flagMaxSyms, flagMinEdge, flagThreshold)

	// place exit orders for existing positions
	for _, st := range gSymbols {
		if !st.position.IsZero() && !st.quote.BidPrice.IsZero() {
			evaluate(st)
		}
	}

	// create timer
	balanceTicker := time.NewTicker(5 * time.Second)
	defer balanceTicker.Stop()

	for {
		select {
		case msg := <-stockUpdates:
			if msg.Type == sip.MessageTypeQuote {
				onQuote(msg.Quote())
			}
		case msg := <-boatsUpdates:
			if msg.Type == sip.MessageTypeQuote {
				onQuote(msg.Quote())
			}
		case update := <-orderUpdates:
			onOrderUpdate(update)
		case clientOrderID := <-gFailedOrders:
			st := gSymbols[gOrderSymbols[clientOrderID]]
			removeOrder(st, clientOrderID)
		case <-balanceTicker.C:
			logBalance()
		case <-sigChan:
			shutdown()
			return
		}
	}
}

func getOrCreateSymbol(name string, asset *alpaca.Asset) *State {
	st, ok := gSymbols[name]
	if !ok {
		st = &State{symbol: name, asset: asset}
		gSymbols[name] = st
	}
	return st
}

func removeOrder(st *State, clientOrderID string) {
	delete(gOrderSymbols, clientOrderID)
	if st.buyClientOrderID == clientOrderID {
		st.buyClientOrderID = ""
	}
	if st.sellClientOrderID == clientOrderID {
		st.sellClientOrderID = ""
	}
	if !st.Active() {
		delete(gActiveSymbols, st.symbol)
	}
}

func TradeQuantity(price, margin, maximum decimal.Decimal) decimal.Decimal {
	// alpaca's overnight margin requirement is normally 0.3 but we've seen it go as
	// high as 2.0 for risky stocks. penny stocks (anything under $5) usually have a
	// margin requirement of 1.0. leveraged etfs will usually be 0.6 to 0.9. so what
	// happens here is as margin grows above 0.3 we scale down how much size it uses
	rat := kStandardMarginRate.Div(margin)
	qty := flagSize.Mul(rat).Div(price)
	// we like to trade one round lot at a time
	qty = qty.Min(cboe.LotSize(price))
	// we don't want to take too much liquidity
	qty = qty.Min(maximum)
	// we don't trade fractional shares
	qty = qty.Truncate()
	// each order needs at minimum two shares for commissions+fees to go negative
	if qty.Cmp(*flagFloor) < 0 {
		return decimal.Zero // stock is probably very expensive like meta
	}
	return qty
}

func onQuote(q *sip.Quote) {
	gQuoteCount++
	name := q.Symbol.String()
	st, ok := gSymbols[name]
	if !ok {
		asset := alpaca.GetAsset(q.Symbol)
		if asset == nil {
			return
		}
		if asset.Exchange == alpaca.ExchangeOTC ||
			asset.Class != alpaca.AssetClassUSEquity ||
			asset.Status != alpaca.AssetStatusActive ||
			asset.PTPNoException.Load() ||
			asset.PTPWithException.Load() {
			return
		}
		st = getOrCreateSymbol(name, asset)
	}
	st.quote = q
	evaluate(st)
}

func onOrderUpdate(update *alpaca.OrderUpdate) {
	sym, ok := gOrderSymbols[update.Order.ClientOrderID]
	if !ok {
		return
	}
	st := gSymbols[sym]

	log.Printf("%s order %s: %s price=%s qty=%s pos=%s filled=%s/%s",
		sym, update.Event, update.Order.Status,
		update.Price, update.Qty, update.PositionQty,
		update.Order.FilledQty, update.Order.Qty)

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
		fee := EstimateFee(update.Order.Side, absQty, firstFill)
		st.totalFees = st.totalFees.Add(fee)
		gTotalFees = gTotalFees.Add(fee)

		st.position = update.PositionQty
		gTotalFills++

		log.Printf("%s filled %s @ %s | pos=%s | fill_pnl=%s | fee=%s | realized=%s | total_pnl=%s | total_fees=%s | fills=%d",
			sym, signedQty, fillPrice, st.position,
			fillPnL, fee, st.realizedPnL, gTotalPnL, gTotalFees, gTotalFills)
	}

	// clean up completed orders
	if update.Order.Status.IsFinal() {
		removeOrder(st, update.Order.ClientOrderID)
		evaluate(st)
	}
}

func evaluate(st *State) {
	if st.quote.Indicative() {
		return
	}
	if !st.asset.Tradable.Load() {
		return
	}

	// check if market is open
	overnight := false
	now := clocky.Now()
	if !cboe.IsMarketOpenExtended(now) {
		if !cboe.IsOvernight(now) {
			return
		}
		if !st.asset.OvernightTradable.Load() || st.asset.OvernightHalted.Load() {
			return
		}
		overnight = true
	}

	// PRIORITY 1: Exit existing positions at a profit.
	// Post AT the bid/ask (don't improve) so we capture the spread.
	// Entry improves by a penny to get priority; exit waits at the edge.
	// With -winner, floor the exit price at avg entry + a tick to guarantee profit.
	if st.position.IsPositive() && st.sellClientOrderID == "" && st.asset.Tradable.Load() && !st.quote.DangerousAsk() {
		price := st.quote.AskPrice
		if !flagGreed.IsZero() && !st.costBasis.IsZero() {
			minPrice := st.costBasis.Div(st.position).Mul(decimal.One.Add(*flagGreed))
			price = price.Max(minPrice)
			price = QuantizeSell(price)
		}
		dest := BestDestination(st.quote.AskExchange, overnight)
		qty := cboe.LotSize(price).Min(st.position)
		PlaceOrder(st, ds.SideSell, qty, price, dest)
		return
	}
	if st.position.IsNegative() && st.buyClientOrderID == "" && st.asset.Tradable.Load() && !st.quote.DangerousBid() {
		price := st.quote.BidPrice
		if !flagGreed.IsZero() && !st.costBasis.IsZero() {
			// short: avg entry = costBasis / position = negative / negative = positive
			maxPrice := st.costBasis.Div(st.position).Mul(decimal.One.Sub(*flagGreed))
			price = price.Min(maxPrice)
			price = QuantizeBuy(price)
		}
		dest := BestDestination(st.quote.BidExchange, overnight)
		qty := cboe.LotSize(price).Min(st.position.Neg())
		PlaceOrder(st, ds.SideBuy, qty, price, dest)
		return
	}

	// PRIORITY 2: Enter new positions based on imbalance signal.
	// Only when flat (no position) and no pending orders.
	if *flagExitOnly {
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
	if spread.Cmp(*flagMinEdge) < 0 {
		return
	}
	if !st.asset.Marginable.Load() {
		return
	}

	// imbalance = (bid size - ask size) / (bid size + ask size)
	bidSize := decimal.FromInt(int(st.quote.BidSize))
	askSize := decimal.FromInt(int(st.quote.AskSize))
	totalSize := bidSize.Add(askSize)
	if totalSize.IsZero() {
		return
	}
	imbalance := bidSize.Sub(askSize).Div(totalSize)

	if imbalance.Cmp(*flagThreshold) > 0 {
		// strong bid pressure — buy near bid
		price := st.quote.BidPrice.Add(Tick(st.quote.BidPrice))
		price = price.Min(st.quote.AskPrice.Sub(Tick(st.quote.AskPrice)))
		if price.Cmp(*flagMinPrice) >= 0 {
			mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
			mar := st.asset.MarginRequirementLong.Load()
			qty := TradeQuantity(mid, mar, totalSize)
			dst := BestDestination(st.quote.BidExchange, overnight)
			PlaceOrder(st, ds.SideBuy, qty, price, dst)
		}
	} else if imbalance.Cmp(flagThreshold.Neg()) < 0 && st.asset.EasyToBorrow.Load() {
		// strong ask pressure — sell near ask
		price := st.quote.AskPrice.Sub(Tick(st.quote.AskPrice))
		price = price.Max(st.quote.BidPrice.Add(Tick(st.quote.BidPrice)))
		if price.Cmp(*flagMinPrice) >= 0 {
			mid := st.quote.BidPrice.Add(st.quote.AskPrice).Half()
			mar := st.asset.MarginRequirementShort.Load()
			qty := TradeQuantity(mid, mar, totalSize)
			dst := BestDestination(st.quote.AskExchange, overnight)
			PlaceOrder(st, ds.SideSell, qty, price, dst)
		}
	}
}

func BestDestination(exchange sip.Exchange, overnight bool) alpaca.OrderDestination {
	if overnight {
		return alpaca.OrderDestinationNone // let alpaca route to boats
	}
	switch exchange {
	case sip.ExchangeNASDAQ:
		return alpaca.OrderDestinationNASDAQ
	case sip.ExchangeARCA:
		return alpaca.OrderDestinationARCA
	case sip.ExchangeNYSE:
		return alpaca.OrderDestinationNYSE
	default:
		return alpaca.OrderDestinationNASDAQ
	}
}

func PlaceOrder(st *State, side ds.Side, qty decimal.Decimal, price decimal.Decimal, dest alpaca.OrderDestination) {
	if qty.IsZero() {
		return
	}

	clientOrderID := uuid.New().String()
	gOrderSymbols[clientOrderID] = st.symbol
	gActiveSymbols[st.symbol] = true
	if side == ds.SideBuy {
		st.buyClientOrderID = clientOrderID
	} else {
		st.sellClientOrderID = clientOrderID
	}

	log.Printf("%s placing %s %s @ %s -> %s (pos=%s spread=%s)",
		st.symbol, side, qty, price, dest,
		st.position, st.quote.AskPrice.Sub(st.quote.BidPrice))

	// don't slow down quote processing
	go func() {

		// check if extended hours
		now := clocky.Now()
		extendedHours := !cboe.IsMarketOpen(now)
		if extendedHours && dest == alpaca.OrderDestinationNYSE {
			dest = alpaca.OrderDestinationNone // nyse is for day trading only
		}

		// configure alpaca smart router
		var advancedInstructions *alpaca.AdvancedInstructions
		if dest != alpaca.OrderDestinationNone {
			advancedInstructions = &alpaca.AdvancedInstructions{
				Algorithm:   alpaca.OrderAlgorithmDMA,
				Destination: dest,
			}
		}

		_, err := gClient.CreateOrder(&alpaca.CreateOrderRequest{
			Symbol:               st.symbol,
			Side:                 side,
			Qty:                  qty,
			Type:                 alpaca.OrderTypeLimit,
			TimeInForce:          alpaca.TimeInForceDay,
			ExtendedHours:        extendedHours,
			LimitPrice:           price,
			ClientOrderID:        clientOrderID,
			AdvancedInstructions: advancedInstructions,
		})
		if err != nil {
			log.Printf("%s error placing order: %v", st.symbol, err)
			gFailedOrders <- clientOrderID
			return
		}
	}()
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
	log.Printf("BALANCE long=$%s (%d) short=$%s (%d) net_exposure=$%s | realized=%s fees=%s net=%s unrealized=%s | fills=%d pending=%d quotes=%d",
		longNotional.Format(0), longCount,
		shortNotional.Format(0), shortCount,
		longNotional.Sub(shortNotional).Format(0),
		gTotalPnL, gTotalFees, net, totalUnrealized,
		gTotalFills, pendingCount, gQuoteCount)
}

func shutdown() {
	log.Printf("shutting down... canceling all orders")
	if err := gClient.CancelAllOrders(); err != nil {
		log.Printf("error canceling orders: %v", err)
	}

	// print per-symbol P&L summary
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

func QuantizeBuy(price decimal.Decimal) decimal.Decimal {
	return price.QuantizeTruncate(Tick(price)) // buy low
}

func QuantizeSell(price decimal.Decimal) decimal.Decimal {
	return price.QuantizeAway(Tick(price)) // sell high
}

func Tick(price decimal.Decimal) decimal.Decimal {
	if price.Cmp(decimal.One) < 0 {
		return decimal.Pip
	} else {
		return decimal.Cent
	}
}

func isOptionsSymbol(sym string) bool {
	_, _, _, _, _, _, err := osi.Parse(sym)
	return err == nil
}
