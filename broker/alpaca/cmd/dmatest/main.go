// dmatest verifies that Alpaca's Elite Smart Router DMA (Direct Market Access)
// feature routes orders directly to lit exchanges as claimed, bypassing PFOF
// (Payment For Order Flow) arrangements with market makers like Citadel/Virtu.
//
// The test works by:
//  1. Subscribing to the SIP (Securities Information Processor) websocket feed
//  2. Placing a limit order that improves the NBBO (posts inside the spread)
//  3. Watching the SIP tape to verify our quote appears on the specified exchange
//  4. Measuring latency from order submission to NBBO visibility
//
// Key findings from testing on AWS us-east-1 (~1.5ms from Alpaca):
//   - DMA to specific exchange: 21-35ms to NBBO visibility
//   - Smart router (non-DMA): 32-66ms to NBBO visibility
//   - Exchange codes on SIP confirm routing: N=NYSE, Q=NASDAQ, P=ARCA
//   - DMA routing via DASH Financial Technologies (no PFOF per their Rule 606)
//
// Important: Use round lots (100 shares) to update NBBO display. Odd lots
// (<100 shares) don't update the national best bid/offer under SEC rules.
// Use wide-spread stocks (MGEE, not SPY) to actually improve the NBBO.
//
// Exchange codes you'll see on the SIP tape:
//
//	N = NYSE                    Q = NASDAQ OMX
//	P = NYSE Arca               Z = BATS/Cboe
//	K = EDGX                    V = IEX
//	D = FINRA ADF (dark/OTC)
//
// Usage:
//
//	go run ./broker/alpaca/cmd/dmatest -symbol MGEE -qty 100 -dma -dest NYSE
//	go run ./broker/alpaca/cmd/dmatest -symbol MGEE -qty 100 -dma -dest NASDAQ
//	go run ./broker/alpaca/cmd/dmatest -symbol MGEE -qty 100  # smart router
//	go run ./broker/alpaca/cmd/dmatest -symbol SPY -watch     # just watch SIP
package main

import (
	"context"
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"dropbear/netty"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const sipWSURL = "wss://stream.data.alpaca.markets/v2/sip"

var (
	flagSymbol      = flag.String("symbol", "MGEE", "symbol to trade (use wide-spread stock to test NBBO improvement)")
	flagQty         = decimal.Flag("qty", "1", "number of shares")
	flagDMA         = flag.Bool("dma", false, "use DMA routing")
	flagDestination = flag.String("dest", "ARCA", "destination exchange for DMA (NYSE, NASDAQ, ARCA)")
	flagSide        = flag.String("side", "buy", "buy or sell")
	flagImprove     = flag.Bool("improve", true, "improve NBBO by 1 cent (post inside the spread)")
	flagWatch       = flag.Bool("watch", false, "just watch SIP feed without placing orders")
	flagCancel      = flag.Bool("cancel", true, "cancel order after test (set false to let it fill)")
	flagTimeout     = clocky.DurationFlag("timeout", "30s", "how long to monitor after order placement")
	flagVerbose     = flag.Bool("v", false, "verbose: print all SIP quotes and trades")
)

// SIPMessage represents a message from the SIP (Securities Information Processor)
// websocket feed. The SIP consolidates quotes and trades from all US exchanges
// into a single national feed - this is the "ground truth" for market data.
type SIPMessage struct {
	Type      string          `json:"T"` // "t" for trade, "q" for quote
	Symbol    string          `json:"S"`
	Timestamp clocky.Time     `json:"t"` // exchange timestamp
	Price     decimal.Decimal `json:"p"` // trade price
	Size      decimal.Decimal `json:"s"` // trade size
	Exchange  string          `json:"x"` // exchange code (N=NYSE, Q=NASDAQ, P=ARCA, etc.)
	// Quote fields (NBBO = National Best Bid/Offer)
	BidPrice    decimal.Decimal `json:"bp"` // best bid price
	BidSize     decimal.Decimal `json:"bs"` // size at best bid
	BidExchange string          `json:"bx"` // exchange with best bid
	AskPrice    decimal.Decimal `json:"ap"` // best ask price
	AskSize     decimal.Decimal `json:"as"` // size at best ask
	AskExchange string          `json:"ax"` // exchange with best ask
	Conditions  []string        `json:"c"`  // trade/quote condition codes
}

// TestResult captures all the latency measurements and observations from a test run.
// The key metric is OurQuoteTime - when our order becomes visible on the SIP tape.
// If OurQuoteSeen is false, the order was either internalized or filled instantly.
type TestResult struct {
	UsedDMA           bool
	Destination       string
	Symbol            string
	Side              ds.Side
	OrderQty          decimal.Decimal
	LimitPrice        decimal.Decimal
	NBBOAtSubmit      *alpaca.Quote
	OrderSubmitTime   time.Time // when we called CreateOrder
	OrderAcceptedTime time.Time // when API returned (order accepted by Alpaca)
	OrderID           string
	OrderNewTime      time.Time // when "new" event received (order routed to exchange)
	OrderNewEvent     bool
	FillTime          time.Time
	FillPrice         decimal.Decimal
	FillQty           decimal.Decimal
	Canceled          bool
	SIPQuotesSeen     int       // total quotes observed on SIP feed
	SIPTradesSeen     int       // total trades observed on SIP feed
	OurQuoteSeen      bool      // did we see our order improve the NBBO?
	OurTradeSeen      bool      // did we see a trade at our price?
	OurQuoteTime      time.Time // when our quote appeared on SIP (NBBO improved)
	OurTradeTime      time.Time // when trade at our price appeared on SIP
}

func main() {
	flag.Parse()
	loggy.Init()

	// parse destination
	dest, err := alpaca.ParseOrderDestination(*flagDestination)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid destination: %v\n", err)
		os.Exit(1)
	}

	// parse side
	var side ds.Side
	switch *flagSide {
	case "buy":
		side = ds.SideBuy
	case "sell":
		side = ds.SideSell
	default:
		fmt.Fprintf(os.Stderr, "invalid side: %s (use 'buy' or 'sell')\n", *flagSide)
		os.Exit(1)
	}

	// setup signal handling for clean cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Printf("Received interrupt, shutting down...")
		cancel()
	}()

	client := alpaca.NewClient()

	// get initial quote
	quote, err := client.GetQuote(*flagSymbol)
	if err != nil {
		loggy.Fatalf("failed to get quote: %v", err)
	}
	log.Printf("NBBO: bid=%s@%s ask=%s@%s",
		quote.BidPrice, quote.BidExchange,
		quote.AskPrice, quote.AskExchange)

	if *flagWatch {
		watchSIP(*flagSymbol)
		return
	}

	// calculate limit price
	var limitPrice decimal.Decimal
	if *flagImprove {
		// improve NBBO by 1 cent - post inside the spread
		if side == ds.SideBuy {
			// bid 1 cent higher than current best bid
			limitPrice = quote.BidPrice.Add(decimal.Cent)
		} else {
			// ask 1 cent lower than current best ask
			limitPrice = quote.AskPrice.Sub(decimal.Cent)
		}
	} else {
		// post at current NBBO (join the queue)
		if side == ds.SideBuy {
			limitPrice = quote.BidPrice
		} else {
			limitPrice = quote.AskPrice
		}
	}
	spread := quote.AskPrice.Sub(quote.BidPrice)
	log.Printf("Spread: %s (%.1f bps)", spread, spread.Div(quote.BidPrice).Mul(decimal.FromInt(10000)).Float64())

	result := &TestResult{
		UsedDMA:      *flagDMA,
		Destination:  *flagDestination,
		Symbol:       *flagSymbol,
		Side:         side,
		OrderQty:     *flagQty,
		LimitPrice:   limitPrice,
		NBBOAtSubmit: quote,
	}

	log.Printf("Test config: DMA=%v dest=%s side=%s qty=%s limit=%s",
		*flagDMA, *flagDestination, side, *flagQty, limitPrice)

	// start SIP monitoring in background
	sipChan := make(chan SIPMessage, 1000)
	go monitorSIP(ctx, *flagSymbol, sipChan)

	// start order updates monitoring
	orderUpdates := alpaca.OrderUpdates()

	// give SIP connection time to establish
	time.Sleep(500 * time.Millisecond)

	// place order
	var advancedInstructions *alpaca.AdvancedInstructions
	if *flagDMA {
		advancedInstructions = &alpaca.AdvancedInstructions{
			Algorithm:   alpaca.OrderAlgorithmDMA,
			Destination: dest,
		}
	}

	log.Printf("Placing order...")
	result.OrderSubmitTime = time.Now()

	order, err := client.CreateOrder(&alpaca.CreateOrderRequest{
		Symbol:               *flagSymbol,
		Qty:                  *flagQty,
		Side:                 side,
		Type:                 alpaca.OrderTypeLimit,
		TimeInForce:          alpaca.TimeInForceDay,
		LimitPrice:           limitPrice,
		AdvancedInstructions: advancedInstructions,
	})
	if err != nil {
		loggy.Fatalf("failed to place order: %v", err)
	}

	result.OrderAcceptedTime = time.Now()
	result.OrderID = order.ID
	submitLatency := result.OrderAcceptedTime.Sub(result.OrderSubmitTime)
	orderJSON, _ := json.Marshal(order)
	log.Printf("Order accepted: latency=%v response=%s", submitLatency, orderJSON)

	// monitor for order events and SIP messages
	timeout := time.After(time.Duration(*flagTimeout) * time.Microsecond)
	done := false

	for !done {
		select {
		case <-ctx.Done():
			done = true

		case <-timeout:
			log.Printf("Timeout reached")
			done = true

		case update := <-orderUpdates:
			if update.Order.ID != order.ID {
				continue
			}
			eventTime := time.Now()
			log.Printf("Order update: event=%s status=%s qty=%s price=%s",
				update.Event, update.Order.Status, update.Qty, update.Price)

			switch update.Event {
			case alpaca.OrderEventNew:
				if !result.OrderNewEvent {
					result.OrderNewEvent = true
					result.OrderNewTime = eventTime
					wsLatency := eventTime.Sub(result.OrderSubmitTime)
					log.Printf("ORDER ROUTED: websocket 'new' event received, latency=%v", wsLatency)
				}
			case alpaca.OrderEventFill, alpaca.OrderEventPartialFill:
				result.FillTime = time.Now()
				result.FillPrice = update.Price
				result.FillQty = update.Qty
				fillLatency := result.FillTime.Sub(result.OrderSubmitTime)
				log.Printf("FILL: price=%s qty=%s latency=%v", update.Price, update.Qty, fillLatency)
				if update.Event == alpaca.OrderEventFill {
					done = true
				}
			case alpaca.OrderEventCanceled:
				result.Canceled = true
				done = true
			case alpaca.OrderEventRejected:
				log.Printf("Order rejected!")
				done = true
			}

		case msg := <-sipChan:
			switch msg.Type {
			case "t": // trade
				result.SIPTradesSeen++
				if *flagVerbose {
					log.Printf("SIP TRADE: %s @ %s x=%s", msg.Price, msg.Size, msg.Exchange)
				}
				// check if this could be our fill
				if msg.Price.Cmp(limitPrice) == 0 {
					if !result.OurTradeSeen {
						result.OurTradeSeen = true
						result.OurTradeTime = time.Now()
						log.Printf("TRADE AT OUR PRICE: latency=%v\n  %#v",
							result.OurTradeTime.Sub(result.OrderSubmitTime), msg)
					}
				}
			case "q": // quote
				result.SIPQuotesSeen++
				if *flagVerbose {
					log.Printf("SIP QUOTE: bid=%s@%s(%s) ask=%s@%s(%s)",
						msg.BidPrice, msg.BidSize, msg.BidExchange,
						msg.AskPrice, msg.AskSize, msg.AskExchange)
				}
				// check if our order appears as a quote (NBBO improvement)
				var ourPriceSeen bool
				var ourExchange string
				if side == ds.SideBuy {
					// our bid should appear as the new best bid
					if msg.BidPrice.Cmp(limitPrice) == 0 {
						ourPriceSeen = true
						ourExchange = msg.BidExchange
					}
				} else {
					// our ask should appear as the new best ask
					if msg.AskPrice.Cmp(limitPrice) == 0 {
						ourPriceSeen = true
						ourExchange = msg.AskExchange
					}
				}
				if ourPriceSeen && !result.OurQuoteSeen {
					result.OurQuoteSeen = true
					result.OurQuoteTime = time.Now()
					quoteLatency := result.OurQuoteTime.Sub(result.OrderSubmitTime)
					log.Printf("NBBO IMPROVED! Our %s at %s visible via %s, latency=%v\n  %#v",
						side, limitPrice, ourExchange, quoteLatency, msg)
				}
			}
		}
	}

	// cancel order if requested and not filled
	if *flagCancel && !result.Canceled && result.FillQty.IsZero() {
		log.Printf("Canceling order...")
		if err := client.CancelOrder(order.ID); err != nil {
			log.Printf("Failed to cancel order: %v", err)
		} else {
			result.Canceled = true
		}
	}

	// print summary
	fmt.Println("\n=== TEST RESULTS ===")
	fmt.Printf("DMA Enabled:       %v\n", result.UsedDMA)
	if result.UsedDMA {
		fmt.Printf("Destination:       %s\n", result.Destination)
	}
	fmt.Printf("Symbol:            %s\n", result.Symbol)
	fmt.Printf("Side:              %s\n", result.Side)
	fmt.Printf("Quantity:          %s\n", result.OrderQty)
	fmt.Printf("Limit Price:       %s\n", result.LimitPrice)
	fmt.Printf("NBBO at Submit:    %s/%s\n", result.NBBOAtSubmit.BidPrice, result.NBBOAtSubmit.AskPrice)
	fmt.Println()
	fmt.Println("--- Latencies ---")
	fmt.Printf("API Response:      %v\n", result.OrderAcceptedTime.Sub(result.OrderSubmitTime))
	if result.OrderNewEvent {
		fmt.Printf("WS 'new' Event:    %v\n", result.OrderNewTime.Sub(result.OrderSubmitTime))
	} else {
		fmt.Printf("WS 'new' Event:    (not received)\n")
	}
	if result.OurQuoteSeen {
		fmt.Printf("NBBO Improved:     %v  <-- our order visible on SIP!\n", result.OurQuoteTime.Sub(result.OrderSubmitTime))
	} else {
		fmt.Printf("NBBO Improved:     NO  (order not visible on public tape)\n")
	}
	fmt.Println()
	fmt.Println("--- SIP Activity ---")
	fmt.Printf("Quotes Seen:       %d\n", result.SIPQuotesSeen)
	fmt.Printf("Trades Seen:       %d\n", result.SIPTradesSeen)
	fmt.Println()
	if !result.FillQty.IsZero() {
		fmt.Println("--- Fill ---")
		fmt.Printf("Filled:            %s @ %s\n", result.FillQty, result.FillPrice)
		fmt.Printf("Fill Latency:      %v\n", result.FillTime.Sub(result.OrderSubmitTime))
		slippage := result.FillPrice.Sub(result.LimitPrice)
		if result.Side == ds.SideSell {
			slippage = result.LimitPrice.Sub(result.FillPrice)
		}
		fmt.Printf("Slippage:          %s\n", slippage)
	} else if result.Canceled {
		fmt.Printf("Status:            Canceled (no fill)\n")
	} else {
		fmt.Printf("Status:            Pending\n")
	}
}

// watchSIP prints all SIP quotes and trades for a symbol in real-time.
// Useful for understanding market microstructure and seeing what the tape looks like.
func watchSIP(symbol string) {
	log.Printf("Watching SIP feed for %s (press Ctrl+C to stop)...", symbol)

	conn, _, err := netty.FastWSDial(sipWSURL, nil)
	if err != nil {
		loggy.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// handle Ctrl-C by closing connection
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Printf("Shutting down...")
		conn.Close()
	}()

	// authenticate
	key := alpaca.GetKey()
	auth := map[string]any{
		"action": "auth",
		"key":    key.Key,
		"secret": key.Secret,
	}
	if err := conn.WriteJSON(auth); err != nil {
		loggy.Fatalf("auth failed: %v", err)
	}

	// read auth response
	_, msg, err := conn.ReadMessage()
	if err != nil {
		loggy.Fatalf("failed to read auth response: %v", err)
	}
	log.Printf("Auth response: %s", string(msg))

	// subscribe
	subscribe := map[string]any{
		"action": "subscribe",
		"trades": []string{symbol},
		"quotes": []string{symbol},
	}
	if err := conn.WriteJSON(subscribe); err != nil {
		loggy.Fatalf("subscribe failed: %v", err)
	}

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return // connection closed
		}

		var msgs []SIPMessage
		if err := json.Unmarshal(message, &msgs); err != nil {
			log.Printf("parse error: %v", err)
			continue
		}

		for _, m := range msgs {
			switch m.Type {
			case "t":
				fmt.Printf("TRADE %s: %s @ %s x=%s c=%v\n",
					m.Symbol, m.Price, m.Size, m.Exchange, m.Conditions)
			case "q":
				fmt.Printf("QUOTE %s: bid=%s@%s(%s) ask=%s@%s(%s)\n",
					m.Symbol, m.BidPrice, m.BidSize, m.BidExchange,
					m.AskPrice, m.AskSize, m.AskExchange)
			}
		}
	}
}

// monitorSIP subscribes to the SIP feed and sends messages to the output channel.
// Runs until context is canceled. Used to detect when our order appears on the tape.
func monitorSIP(ctx context.Context, symbol string, out chan<- SIPMessage) {
	conn, _, err := netty.FastWSDial(sipWSURL, nil)
	if err != nil {
		log.Printf("SIP connect failed: %v", err)
		return
	}
	defer conn.Close()

	// close connection when context is canceled
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	// authenticate
	key := alpaca.GetKey()
	auth := map[string]any{
		"action": "auth",
		"key":    key.Key,
		"secret": key.Secret,
	}
	if err := conn.WriteJSON(auth); err != nil {
		log.Printf("SIP auth failed: %v", err)
		return
	}

	// read auth response
	_, _, err = conn.ReadMessage()
	if err != nil {
		log.Printf("SIP auth response failed: %v", err)
		return
	}

	// subscribe
	subscribe := map[string]any{
		"action": "subscribe",
		"trades": []string{symbol},
		"quotes": []string{symbol},
	}
	if err := conn.WriteJSON(subscribe); err != nil {
		log.Printf("SIP subscribe failed: %v", err)
		return
	}

	log.Printf("SIP monitoring started for %s", symbol)

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return // context canceled, exit quietly
			}
			log.Printf("SIP read error: %v", err)
			return
		}

		var msgs []SIPMessage
		if err := json.Unmarshal(message, &msgs); err != nil {
			continue
		}

		for _, m := range msgs {
			if m.Type == "t" || m.Type == "q" {
				select {
				case out <- m:
				case <-ctx.Done():
					return
				default:
					// channel full, drop message
				}
			}
		}
	}
}
