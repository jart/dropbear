// liquidate.market - liquidate positions using Alpaca's default smart router
// compare this to liquidate/ which uses DMA to specific exchanges
package main

import (
	"bytes"
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

var (
	verbose = flag.Bool("v", false, "verbose: print full JSON responses")

	// track in-flight order for cancellation on ctrl-c
	orderMu      sync.Mutex
	currentOrder *alpaca.Order
	client       *alpaca.Client
)

func main() {
	loggy.Init()
	flag.Parse()
	if flag.NArg() < 1 {
		loggy.Fatalf("usage: %s [flags] SYMBOL...", os.Args[0])
	}

	client = alpaca.NewClient()

	// handle ctrl-c: cancel in-flight order
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		fmt.Println("\nCancelling in-flight order...")
		orderMu.Lock()
		if currentOrder != nil {
			fmt.Printf("Cancelling order %s...\n", currentOrder.ID[:8])
			if err := client.CancelOrder(currentOrder.ID); err != nil {
				fmt.Printf("  error: %v\n", err)
			} else {
				fmt.Printf("  cancelled\n")
			}
		}
		orderMu.Unlock()
		os.Exit(1)
	}()

	// get positions
	positions, err := client.GetPositions()
	if err != nil {
		loggy.Fatalf("getting positions: %v", err)
	}
	posMap := make(map[string]alpaca.Position)
	for _, pos := range positions {
		posMap[pos.Symbol] = pos
	}

	// process each symbol
	for _, sym := range flag.Args() {
		sym = strings.ToUpper(sym)
		pos, ok := posMap[sym]
		if !ok {
			fmt.Printf("%s: no position\n", sym)
			continue
		}
		if pos.Qty.IsZero() {
			fmt.Printf("%s: zero quantity\n", sym)
			continue
		}

		// get nbbo quote before order
		quote, err := client.GetQuote(sym)
		if err != nil {
			fmt.Printf("%s: error getting quote: %v\n", sym, err)
			continue
		}
		if *verbose {
			fmt.Printf("%s quote:\n", sym)
			prettyPrint(quote)
		}

		if quote.BidPrice.IsZero() || quote.AskPrice.IsZero() {
			fmt.Printf("%s: no valid quote (bid=%s ask=%s)\n", sym, quote.BidPrice, quote.AskPrice)
			continue
		}

		// determine side
		var side ds.Side
		qty := pos.Qty.Abs()
		if pos.Qty.IsPositive() {
			side = ds.SideSell
		} else {
			side = ds.SideBuy
		}

		spread := quote.AskPrice.Sub(quote.BidPrice)
		midPrice := quote.BidPrice.Add(quote.AskPrice).DivInt(2)

		fmt.Printf("%s: %s %s via MARKET ORDER (bid=%s ask=%s spread=%s)\n",
			sym, side, qty, quote.BidPrice, quote.AskPrice, spread)

		// place market order through default smart router
		fmt.Printf("[SMART] placing %s %s\n", side, qty)
		order, err := client.CreateOrder(&alpaca.OrderRequest{
			Symbol:      sym,
			Qty:         qty,
			Side:        side,
			Type:        alpaca.OrderTypeMarket,
			TimeInForce: alpaca.TimeInForceDay,
		})
		if err != nil {
			fmt.Printf("[SMART] error placing order: %v\n", err)
			continue
		}

		// track for ctrl-c cancellation
		orderMu.Lock()
		currentOrder = order
		orderMu.Unlock()

		if *verbose {
			fmt.Printf("[SMART] order placed:\n")
			prettyPrint(order)
		} else {
			fmt.Printf("[SMART] order %s status=%s\n", order.ID[:8], order.Status)
		}

		// poll until filled
		for {
			time.Sleep(time.Second)

			order, err = client.GetOrder(order.ID)
			orderMu.Lock()
			currentOrder = order
			orderMu.Unlock()
			if err != nil {
				fmt.Printf("[SMART] error polling: %v\n", err)
				continue
			}

			if *verbose {
				fmt.Printf("[SMART] poll:\n")
				prettyPrint(order)
			} else {
				fmt.Printf("[SMART] %s filled=%s/%s status=%s\n",
					order.ID[:8], order.FilledQty, qty, order.Status)
			}

			// check if terminal
			switch order.Status {
			case alpaca.OrderStatusFilled:
				fmt.Printf("[SMART] FILLED %s @ %s\n", order.FilledQty, order.FilledAvgPrice)
				goto done
			case alpaca.OrderStatusCanceled, alpaca.OrderStatusExpired, alpaca.OrderStatusRejected:
				fmt.Printf("[SMART] TERMINAL status=%s filled=%s\n", order.Status, order.FilledQty)
				goto done
			}
		}
	done:
		// clear tracked order
		orderMu.Lock()
		currentOrder = nil
		orderMu.Unlock()

		// print summary
		fmt.Printf("\n=== %s SUMMARY (MARKET ORDER via Smart Router) ===\n", sym)
		fmt.Printf("  filled %s / %s @ %s (value: $%s)\n",
			order.FilledQty, qty, order.FilledAvgPrice,
			order.FilledQty.Mul(order.FilledAvgPrice).Format(2))

		if order.FilledQty.IsPositive() {
			// calculate price improvement vs worst case (bid for sells, ask for buys)
			var worstPrice, improvement decimal.Decimal
			if side == ds.SideSell {
				worstPrice = quote.BidPrice
				improvement = order.FilledAvgPrice.Sub(worstPrice)
			} else {
				worstPrice = quote.AskPrice
				improvement = worstPrice.Sub(order.FilledAvgPrice)
			}
			totalImprovement := improvement.Mul(order.FilledQty)

			// how much of the spread did smart router capture?
			spreadCapture := decimal.Zero
			if spread.IsPositive() {
				spreadCapture = improvement.Div(spread).MulInt(100)
			}

			fmt.Printf("  NBBO at order time: bid=%s ask=%s mid=%s spread=%s\n",
				quote.BidPrice, quote.AskPrice, midPrice.Format(2), spread)
			fmt.Printf("  PRICE IMPROVEMENT: $%s/share ($%s total) vs worst %s\n",
				improvement.Format(4), totalImprovement.Format(2), worstPrice)
			fmt.Printf("  SPREAD CAPTURE: %s%% of spread\n", spreadCapture.Format(1))

			// fee estimate (market orders are always taker)
			feeCalc := alpaca.NewFeeCalculator()
			fee := feeCalc.GetFee(clocky.Now(), order.FilledQty, true)
			fmt.Printf("  FEES (taker): $%s\n", fee.Format(4))

			// what we could have gotten with passive limit at ask/bid
			var passivePrice decimal.Decimal
			if side == ds.SideSell {
				passivePrice = quote.AskPrice
			} else {
				passivePrice = quote.BidPrice
			}
			var missedImprovement decimal.Decimal
			if side == ds.SideSell {
				missedImprovement = passivePrice.Sub(order.FilledAvgPrice)
			} else {
				missedImprovement = order.FilledAvgPrice.Sub(passivePrice)
			}
			missedTotal := missedImprovement.Mul(order.FilledQty)

			// maker fee for comparison
			makerFee := alpaca.NewFeeCalculator().GetFee(clocky.Now(), order.FilledQty, false)
			feeDiff := fee.Sub(makerFee)

			fmt.Printf("  VS PASSIVE LIMIT @ %s: lost $%s price + $%s fees = $%s total\n",
				passivePrice, missedTotal.Format(2), feeDiff.Format(2),
				missedTotal.Add(feeDiff).Format(2))
		}
		fmt.Println()
	}
}

func prettyPrint(v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Printf("%+v\n", v)
		return
	}
	var m map[string]any
	if json.Unmarshal(data, &m) == nil {
		clean := make(map[string]any)
		for k, v := range m {
			if !isEmpty(v) {
				clean[k] = v
			}
		}
		if cleaned, err := json.MarshalIndent(clean, "", "  "); err == nil {
			data = cleaned
		}
	}
	var out bytes.Buffer
	if json.Indent(&out, data, "", "  ") == nil {
		fmt.Println(out.String())
	} else {
		fmt.Println(string(data))
	}
}

func isEmpty(v any) bool {
	switch val := v.(type) {
	case string:
		return val == "" || val == "0001-01-01T00:00:00Z" || val == "1970-01-01T00:00:00.000000Z"
	case float64:
		return val == 0
	case nil:
		return true
	default:
		return false
	}
}
