// Exchange imbalance trader.
//
// Subscribes to the SIP firehose for all stocks, filters to healthy
// assets, and looks for quote size imbalances. When one side of the
// book is significantly heavier, takes a small position and tries to
// exit when the imbalance reverses.
//
// Uses DMA routing to send orders directly to NASDAQ or ARCA.
package main

import (
	"dropbear/broker/alpaca"
	"dropbear/broker/alpaca/sip"
	"dropbear/cboe"
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
	flagMaxSyms   = flag.Int("maxsyms", 300, "maximum number of symbols to trade simultaneously")
	flagMaxPos    = flag.Int("maxpos", 4, "maximum position size per symbol (in shares)")
	flagQty       = flag.Int("qty", 4, "shares per order")
	flagMinEdge   = decimal.Flag("minedge", "0.02", "minimum spread in dollars to act")
	flagMinPrice  = decimal.Flag("minprice", "1.0", "minimum stock price to consider")
	flagMaxPrice  = decimal.Flag("maxprice", "500", "maximum stock price to consider")
	flagThreshold = decimal.Flag("threshold", "0.3", "imbalance ratio threshold to trigger (0-1)")
	flagExitOnly  = flag.Bool("exit", false, "exit-only mode: close existing positions, no new entries")
	flagWinner    = flag.Bool("winner", false, "never close a position at a loss (based on avg entry price)")
)

// Fee/rebate constants for equity DMA orders (always making).
var (
	kMakerRebatePerShare = decimal.Parse("0.0018")   // exchange rebate for providing liquidity
	kTafFeePerShare      = decimal.Parse("0.000195") // TAF fee (sells only)
	kCatFeePerTrade      = decimal.Parse("0.0003")   // CAT fee per trade
	kBrokerFeePerTrade   = decimal.Parse("0.0025")   // alpaca elite smart router fee per trade
)

// estimateFee returns the estimated fee for a fill. Negative means net rebate.
// firstFill should be true only for the first fill of an order (CAT and broker
// fees are per-order, not per-fill).
func estimateFee(side ds.Side, qty decimal.Decimal, firstFill bool) decimal.Decimal {
	fee := decimal.Zero
	if firstFill {
		fee = fee.Add(kCatFeePerTrade)
		fee = fee.Add(kBrokerFeePerTrade)
	}
	if side == ds.SideSell {
		fee = fee.Add(kTafFeePerShare.Mul(qty))
	}
	fee = fee.Sub(kMakerRebatePerShare.Mul(qty)) // rebate reduces fee
	return fee
}

// symbolState tracks all state for a single symbol.
type symbolState struct {
	symbol   string
	asset    *alpaca.Asset
	position decimal.Decimal // current shares held (negative if short)

	// latest NBBO quote
	nbboBid     decimal.Decimal
	nbboBidSize int32
	nbboBidEx   sip.Exchange
	nbboAsk     decimal.Decimal
	nbboAskSize int32
	nbboAskEx   sip.Exchange

	// our pending orders
	buyOrderID  string // alpaca order ID
	sellOrderID string

	// P&L tracking
	costBasis    decimal.Decimal // total cost of current position (signed)
	realizedPnL  decimal.Decimal // cumulative realized P&L
	totalBought  decimal.Decimal // total shares bought
	totalSold    decimal.Decimal // total shares sold
	totalCostIn  decimal.Decimal // total dollars spent buying
	totalCostOut decimal.Decimal // total dollars received selling
	totalFees    decimal.Decimal // cumulative fees (negative = net rebate)
}

var (
	gClient       *alpaca.Client
	gSymbols      map[string]*symbolState
	gOrderSymbols map[string]string // client order ID -> symbol
	gTotalPnL     decimal.Decimal
	gTotalFees    decimal.Decimal
	gTotalFills   int
	gQuoteCount   int64
)

func main() {
	loggy.Init()
	flag.Parse()
	loggy.AlsoLogToFile()

	gClient = alpaca.NewClient()
	gOrderSymbols = map[string]string{}
	gSymbols = map[string]*symbolState{}

	// sync assets so GetAsset works
	log.Printf("syncing assets from Alpaca...")
	if err := gClient.SyncAssets(); err != nil {
		log.Fatalf("error syncing assets: %v", err)
	}

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
		log.Printf("loaded position for %s: %s shares @ avg %s (cost basis %s)",
			pos.Symbol, pos.Qty, pos.AvgEntryPrice, pos.CostBasis)
	}

	// cancel stale open orders
	log.Printf("canceling stale orders...")
	if err := gClient.CancelAllOrders(); err != nil {
		log.Printf("warning: error canceling orders: %v", err)
	}

	// fetch quotes for existing positions and place exit orders
	posSymbols := make([]string, 0, len(gSymbols))
	for name := range gSymbols {
		posSymbols = append(posSymbols, name)
	}
	if len(posSymbols) > 0 {
		log.Printf("fetching quotes for %d existing positions...", len(posSymbols))
		quotes, err := gClient.GetQuotes(posSymbols, alpaca.FeedSIP)
		if err != nil {
			log.Printf("warning: error fetching quotes: %v", err)
		} else {
			for name, q := range quotes {
				st := gSymbols[name]
				if st == nil || q.BidPrice.IsZero() || q.AskPrice.IsZero() {
					continue
				}
				st.nbboBid = q.BidPrice
				st.nbboBidSize = int32(q.BidSize.Int())
				st.nbboBidEx = q.BidExchange
				st.nbboAsk = q.AskPrice
				st.nbboAskSize = int32(q.AskSize.Int())
				st.nbboAskEx = q.AskExchange
			}
		}
	}

	// subscribe to order updates
	log.Printf("subscribing to order updates...")
	orderUpdates := alpaca.OrderUpdates()

	// subscribe to SIP firehose
	log.Printf("subscribing to SIP firehose...")
	stockUpdates, err := alpaca.StockUpdates(alpaca.SIPWSURL, &alpaca.StockUpdatesRequest{
		Action: "subscribe",
		Quotes: []string{"*"},
	})
	if err != nil {
		log.Fatalf("error subscribing to stock updates: %v", err)
	}

	// catch ctrl-c
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Printf("trader started: maxpos=%d maxsyms=%d qty=%d minedge=%s threshold=%s price=[%s,%s]",
		*flagMaxPos, *flagMaxSyms, *flagQty, flagMinEdge, flagThreshold, flagMinPrice, flagMaxPrice)

	// place exit orders for existing positions
	for _, st := range gSymbols {
		if !st.position.IsZero() && !st.nbboBid.IsZero() {
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
		case update := <-orderUpdates:
			onOrderUpdate(update)
		case <-balanceTicker.C:
			logBalance()
		case <-sigChan:
			shutdown()
			return
		}
	}
}

func getOrCreateSymbol(name string, asset *alpaca.Asset) *symbolState {
	st, ok := gSymbols[name]
	if !ok {
		st = &symbolState{symbol: name, asset: asset}
		gSymbols[name] = st
	}
	return st
}

func isEligible(sym symbol.Symbol) *alpaca.Asset {
	a := alpaca.GetAsset(sym)
	if a == nil {
		return nil
	}
	if a.Exchange == alpaca.ExchangeOTC ||
		a.Class != alpaca.AssetClassUSEquity ||
		a.Status != alpaca.AssetStatusActive ||
		!a.Tradable.Load() ||
		!a.EasyToBorrow.Load() ||
		a.PTPNoException.Load() ||
		a.PTPWithException.Load() {
		return nil
	}
	return a
}

func onQuote(q *sip.Quote) {
	gQuoteCount++

	if q.BidPrice.IsZero() || q.AskPrice.IsZero() {
		return
	}
	if q.BidPrice.Cmp(*flagMinPrice) < 0 || q.AskPrice.Cmp(*flagMaxPrice) > 0 {
		return
	}
	if q.IsNonFirm() {
		return
	}

	name := q.Symbol.String()
	st, ok := gSymbols[name]
	if !ok {
		asset := isEligible(q.Symbol)
		if asset == nil {
			return
		}
		if activeSymbolCount() >= *flagMaxSyms {
			return
		}
		st = getOrCreateSymbol(name, asset)
	}

	st.nbboBid = q.BidPrice
	st.nbboBidSize = q.BidSize
	st.nbboBidEx = q.BidExchange
	st.nbboAsk = q.AskPrice
	st.nbboAskSize = q.AskSize
	st.nbboAskEx = q.AskExchange

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
		fee := estimateFee(update.Order.Side, absQty, firstFill)
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
		delete(gOrderSymbols, update.Order.ClientOrderID)
		if st.buyOrderID == update.Order.ID {
			st.buyOrderID = ""
		}
		if st.sellOrderID == update.Order.ID {
			st.sellOrderID = ""
		}
		evaluate(st)
	}
}

func evaluate(st *symbolState) {
	if st.nbboBid.IsZero() || st.nbboAsk.IsZero() {
		return
	}

	spread := st.nbboAsk.Sub(st.nbboBid)
	if spread.Cmp(*flagMinEdge) < 0 {
		return
	}

	maxPos := decimal.FromInt(*flagMaxPos)
	qty := decimal.FromInt(*flagQty)

	// PRIORITY 1: Exit existing positions at a profit.
	// Post AT the bid/ask (don't improve) so we capture the spread.
	// Entry improves by a penny to get priority; exit waits at the edge.
	// With -winner, floor the exit price at avg entry + a tick to guarantee profit.
	if st.position.IsPositive() && st.sellOrderID == "" {
		price := st.nbboAsk
		if *flagWinner && !st.costBasis.IsZero() {
			minPrice := st.costBasis.Div(st.position).Add(cboe.Tick01)
			price = price.Max(minPrice)
		}
		dest := bestDest(st.nbboAskEx)
		placeOrder(st, ds.SideSell, qty.Min(st.position), price, dest)
		return
	}
	if st.position.IsNegative() && st.buyOrderID == "" {
		price := st.nbboBid
		if *flagWinner && !st.costBasis.IsZero() {
			// short: avg entry = costBasis / position = negative / negative = positive
			maxPrice := st.costBasis.Div(st.position).Sub(cboe.Tick01)
			price = price.Min(maxPrice)
		}
		dest := bestDest(st.nbboBidEx)
		placeOrder(st, ds.SideBuy, qty.Min(st.position.Neg()), price, dest)
		return
	}

	// PRIORITY 2: Enter new positions based on imbalance signal.
	// Only when flat (no position) and no pending orders.
	if *flagExitOnly {
		return
	}
	if !st.position.IsZero() || st.buyOrderID != "" || st.sellOrderID != "" {
		return
	}

	bidSize := decimal.FromInt(int(st.nbboBidSize))
	askSize := decimal.FromInt(int(st.nbboAskSize))
	totalSize := bidSize.Add(askSize)
	if totalSize.IsZero() {
		return
	}
	imbalance := bidSize.Sub(askSize).Div(totalSize)

	if imbalance.Cmp(*flagThreshold) > 0 && st.position.Cmp(maxPos) < 0 {
		// strong bid pressure — buy near the bid
		price := st.nbboBid.Add(cboe.Tick01)
		if price.Cmp(st.nbboAsk) >= 0 {
			price = st.nbboBid
		}
		dest := bestDest(st.nbboBidEx)
		placeOrder(st, ds.SideBuy, qty, price, dest)
	} else if imbalance.Cmp(flagThreshold.Neg()) < 0 && st.position.Cmp(maxPos.Neg()) > 0 {
		// strong ask pressure — sell near the ask
		price := st.nbboAsk.Sub(cboe.Tick01)
		if price.Cmp(st.nbboBid) <= 0 {
			price = st.nbboAsk
		}
		dest := bestDest(st.nbboAskEx)
		placeOrder(st, ds.SideSell, qty, price, dest)
	}
}

func bestDest(ex sip.Exchange) alpaca.OrderDestination {
	switch ex {
	case sip.ExchangeNASDAQ:
		return alpaca.OrderDestinationNASDAQ
	case sip.ExchangeARCA:
		return alpaca.OrderDestinationARCA
	default:
		return alpaca.OrderDestinationNASDAQ
	}
}

func placeOrder(st *symbolState, side ds.Side, qty decimal.Decimal, price decimal.Decimal, dest alpaca.OrderDestination) {
	clientOrderID := uuid.New().String()
	gOrderSymbols[clientOrderID] = st.symbol

	log.Printf("%s placing %s %s @ %s -> %s (pos=%s spread=%s)",
		st.symbol, side, qty, price, dest,
		st.position, st.nbboAsk.Sub(st.nbboBid))

	order, err := gClient.CreateOrder(&alpaca.CreateOrderRequest{
		Symbol:        st.symbol,
		Side:          side,
		Qty:           qty,
		Type:          alpaca.OrderTypeLimit,
		TimeInForce:   alpaca.TimeInForceDay,
		ExtendedHours: true,
		LimitPrice:    price,
		ClientOrderID: clientOrderID,
		AdvancedInstructions: &alpaca.AdvancedInstructions{
			Algorithm:   alpaca.OrderAlgorithmDMA,
			Destination: dest,
		},
	})
	if err != nil {
		log.Printf("%s error placing order: %v", st.symbol, err)
		delete(gOrderSymbols, clientOrderID)
		return
	}

	if side == ds.SideBuy {
		st.buyOrderID = order.ID
	} else {
		st.sellOrderID = order.ID
	}
}

func activeSymbolCount() int {
	n := 0
	for _, st := range gSymbols {
		if !st.position.IsZero() || st.buyOrderID != "" || st.sellOrderID != "" {
			n++
		}
	}
	return n
}

func isOptionsSymbol(sym string) bool {
	_, _, _, _, _, _, err := osi.Parse(sym)
	return err == nil
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
			mid := st.nbboBid.Add(st.nbboAsk).Half()
			if mid.IsZero() {
				mid = st.costBasis.Div(st.position) // fallback to avg cost
			}
			notional := mid.Mul(st.position)
			longNotional = longNotional.Add(notional)
			longCount++
			totalUnrealized = totalUnrealized.Add(notional.Sub(st.costBasis))
		} else if st.position.IsNegative() {
			mid := st.nbboBid.Add(st.nbboAsk).Half()
			if mid.IsZero() {
				mid = st.costBasis.Div(st.position)
			}
			notional := mid.Mul(st.position.Neg())
			shortNotional = shortNotional.Add(notional)
			shortCount++
			totalUnrealized = totalUnrealized.Add(st.position.Mul(mid).Sub(st.costBasis))
		}
		if st.buyOrderID != "" || st.sellOrderID != "" {
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
			mid := st.nbboBid.Add(st.nbboAsk).Half()
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
