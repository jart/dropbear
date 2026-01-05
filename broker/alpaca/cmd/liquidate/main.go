package main

import (
	"bytes"
	"context"
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
	cross   = decimal.Flag("cross", "0", "spread crossing: 0=ask (passive), 0.5=mid, 1=bid (aggressive)")
	verbose = flag.Bool("v", false, "verbose: print full JSON responses")

	// track in-flight orders for cancellation on ctrl-c
	ordersMu       sync.Mutex
	inFlightOrders = make(map[string]bool)
	client         *alpaca.Client
	ctx            context.Context
	cancel         context.CancelFunc
)

type result struct {
	dest      alpaca.OrderDestination
	qty       decimal.Decimal
	filledQty decimal.Decimal
	avgPrice  decimal.Decimal
	err       error
}

func main() {
	loggy.Init()
	flag.Parse()

	client = alpaca.NewClient()
	ctx, cancel = context.WithCancel(context.Background())

	// handle ctrl-c: cancel all in-flight orders
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		fmt.Println("\nCancelling in-flight orders...")
		cancel()
		ordersMu.Lock()
		for orderID := range inFlightOrders {
			fmt.Printf("Cancelling order %s...\n", orderID[:8])
		TryAgain:
			if err := client.CancelOrder(orderID); err != nil {
				if err == ds.ErrTooManyRequests {
					clocky.Sleep(clocky.Second)
					goto TryAgain
				}
				fmt.Printf("  error: %v\n", err)
			} else {
				fmt.Printf("  cancelled\n")
			}
		}
		ordersMu.Unlock()
		os.Exit(1)
	}()

	// get positions
	positions, err := client.GetPositions()
	if err != nil {
		loggy.Fatalf("getting positions: %v", err)
	}

	// determine which symbols to liquidate
	var symbols []string
	if flag.NArg() == 0 {
		// no args: liquidate all positions
		for _, pos := range positions {
			if !pos.Qty.IsZero() {
				symbols = append(symbols, pos.Symbol)
			}
		}
		if len(symbols) == 0 {
			fmt.Println("no positions to liquidate")
			return
		}
		fmt.Printf("liquidating all %d positions...\n", len(symbols))
	} else {
		// specific symbols
		for _, sym := range flag.Args() {
			symbols = append(symbols, strings.ToUpper(sym))
		}
	}

	posMap := make(map[string]alpaca.Position)
	for _, pos := range positions {
		posMap[pos.Symbol] = pos
	}

	// process all symbols concurrently
	var wg sync.WaitGroup
	for _, sym := range symbols {
		pos, ok := posMap[sym]
		if !ok {
			fmt.Printf("%s: no position\n", sym)
			continue
		}
		if pos.Qty.IsZero() {
			fmt.Printf("%s: zero quantity\n", sym)
			continue
		}
		wg.Add(1)
		go func(sym string, pos alpaca.Position) {
			defer wg.Done()
			liquidateSymbol(sym, pos)
		}(sym, pos)
	}
	wg.Wait()
}

func liquidateSymbol(sym string, pos alpaca.Position) {
	// get nbbo quote - use crypto endpoint for crypto assets
	var quote *alpaca.Quote
	var err error
	isCrypto := pos.AssetClass == alpaca.AssetClassCrypto
	if isCrypto {
		quote, err = client.GetCryptoQuote(sym)
	} else {
		quote, err = client.GetQuote(sym)
	}
	if err != nil {
		fmt.Printf("%s: error getting quote: %v\n", sym, err)
		return
	}
	if *verbose {
		fmt.Printf("%s quote:\n", sym)
		prettyPrint(quote)
	}

	if quote.BidPrice.IsZero() || quote.AskPrice.IsZero() {
		fmt.Printf("%s: no valid quote (bid=%s ask=%s)\n", sym, quote.BidPrice, quote.AskPrice)
		return
	}

	// determine side and price based on -cross flag
	var side ds.Side
	var price decimal.Decimal
	qty := pos.Qty.Abs()
	spread := quote.AskPrice.Sub(quote.BidPrice)

	if pos.Qty.IsPositive() {
		// selling: cross=0 means ask (passive), cross=1 means bid (aggressive)
		side = ds.SideSell
		price = quote.AskPrice.Sub(spread.Mul(decimal.Decimal(*cross)))
	} else {
		// buying: cross=0 means bid (passive), cross=1 means ask (aggressive)
		side = ds.SideBuy
		price = quote.BidPrice.Add(spread.Mul(decimal.Decimal(*cross)))
	}
	if !isCrypto {
		price = price.QuantizeNearest(decimal.Cent)
	}

	fmt.Printf("%s: %s %s @ %s (bid=%s ask=%s spread=%s cross=%s)\n",
		sym, side, qty, price, quote.BidPrice, quote.AskPrice, spread, cross)

	var wg sync.WaitGroup
	results := make(chan result, 3)

	if isCrypto {
		// crypto: single order, no DMA, no exchange splitting
		wg.Add(1)
		go func() {
			defer wg.Done()
			filledQty, avgPrice, err := liquidateCrypto(sym, side, qty, price)
			results <- result{dest: alpaca.OrderDestinationNone, qty: qty, filledQty: filledQty, avgPrice: avgPrice, err: err}
		}()
	} else {
		// equities: split quantity into thirds for each exchange via DMA
		third := qty.DivInt(3).QuantizeTruncate(decimal.One)
		remainder := qty.Sub(third.MulInt(2))
		destinations := []struct {
			dest alpaca.OrderDestination
			qty  decimal.Decimal
		}{
			{alpaca.OrderDestinationNYSE, third},
			{alpaca.OrderDestinationNASDAQ, third},
			{alpaca.OrderDestinationARCA, remainder},
		}

		for _, d := range destinations {
			if d.qty.IsZero() {
				continue
			}
			wg.Add(1)
			go func(dest alpaca.OrderDestination, qty decimal.Decimal) {
				defer wg.Done()
				filledQty, avgPrice, err := liquidateOnExchange(sym, side, qty, price, dest)
				results <- result{dest: dest, qty: qty, filledQty: filledQty, avgPrice: avgPrice, err: err}
			}(d.dest, d.qty)
		}
	}

	wg.Wait()
	close(results)

	// print summary
	fmt.Printf("\n=== %s SUMMARY ===\n", sym)
	var totalQty, totalFilled, totalValue decimal.Decimal
	for r := range results {
		totalQty = totalQty.Add(r.qty)
		totalFilled = totalFilled.Add(r.filledQty)
		totalValue = totalValue.Add(r.filledQty.Mul(r.avgPrice))
		destName := r.dest.String()
		if r.dest == alpaca.OrderDestinationNone {
			destName = "CRYPTO"
		}
		if r.err != nil {
			fmt.Printf("  %s: ERROR %v\n", destName, r.err)
		} else {
			fmt.Printf("  %s: filled %s / %s @ %s\n", destName, r.filledQty, r.qty, r.avgPrice)
		}
	}
	var avgPrice decimal.Decimal
	if totalFilled.IsPositive() {
		avgPrice = totalValue.Div(totalFilled)
	}
	fmt.Printf("  TOTAL: filled %s / %s @ avg %s (value: $%s)\n",
		totalFilled, totalQty, avgPrice.Format(2), totalValue.Format(2))

	// calculate price improvement over market
	// for sells: market = bid, improvement = avg - bid
	// for buys: market = ask, improvement = ask - avg
	if totalFilled.IsPositive() {
		var marketPrice, improvement decimal.Decimal
		if side == ds.SideSell {
			marketPrice = quote.BidPrice
			improvement = avgPrice.Sub(marketPrice)
		} else {
			marketPrice = quote.AskPrice
			improvement = marketPrice.Sub(avgPrice)
		}
		totalImprovement := improvement.Mul(totalFilled)
		fmt.Printf("  PRICE IMPROVEMENT: $%s/share ($%s total) vs market %s\n",
			improvement.Format(4), totalImprovement.Format(2), marketPrice)

		// estimate fees using FeeCalculator (equities only - crypto fees embedded in spread)
		if !isCrypto {
			// we're maker if sell price > bid, or buy price < ask
			var isMaker bool
			if side == ds.SideSell {
				isMaker = price.Cmp(quote.BidPrice) > 0
			} else {
				isMaker = price.Cmp(quote.AskPrice) < 0
			}

			now := clocky.Now()
			feeCalc := alpaca.NewFeeCalculator()
			ourFee := feeCalc.GetFee(now, totalFilled, !isMaker)
			marketFee := alpaca.NewFeeCalculator().GetFee(now, totalFilled, true)
			feeSavings := marketFee.Sub(ourFee)

			if isMaker {
				fmt.Printf("  FEES (maker): $%s\n", ourFee.Format(4))
			} else {
				fmt.Printf("  FEES (taker): $%s\n", ourFee.Format(4))
			}
			fmt.Printf("  FEE SAVINGS: $%s vs market order\n", feeSavings.Format(4))

			// total savings
			totalSavings := totalImprovement.Add(feeSavings)
			fmt.Printf("  TOTAL SAVINGS: $%s (price: $%s + fees: $%s)\n",
				totalSavings.Format(2), totalImprovement.Format(2), feeSavings.Format(2))
		}
	}
	fmt.Println()
}

func liquidateOnExchange(symbol string, side ds.Side, qty, price decimal.Decimal, dest alpaca.OrderDestination) (decimal.Decimal, decimal.Decimal, error) {
	fmt.Printf("[%s] placing %s %s @ %s\n", dest, side, qty, price)

	var order *alpaca.Order
	for {
		var err error
		order, err = client.LimitOrder(symbol, side, qty, price, alpaca.TimeInForceDay, alpaca.OrderAlgorithmDMA, dest, false)
		if err != nil {
			if err == ds.ErrTooManyRequests {
				clocky.Sleep(clocky.Second)
				continue
			}
			fmt.Printf("[%s] error placing order: %v\n", dest, err)
			return decimal.Zero, decimal.Zero, err
		}
		break
	}

	// track order for cancellation on ctrl-c
	ordersMu.Lock()
	inFlightOrders[order.ID] = true
	ordersMu.Unlock()
	defer func() {
		ordersMu.Lock()
		delete(inFlightOrders, order.ID)
		ordersMu.Unlock()
	}()

	if *verbose {
		fmt.Printf("[%s] order placed:\n", dest)
		prettyPrint(order)
	} else {
		fmt.Printf("[%s] order %s status=%s\n", dest, order.ID[:8], order.Status)
	}

	// poll until filled or terminal
	pollInterval := 3 * time.Second
	for {
		select {
		case <-ctx.Done():
			return order.FilledQty, order.FilledAvgPrice, ctx.Err()
		case <-time.After(pollInterval):
		}

		order, err := client.GetOrder(order.ID)
		if err != nil {
			fmt.Printf("[%s] error polling: %v\n", dest, err)
			continue
		}

		if *verbose {
			fmt.Printf("[%s] poll:\n", dest)
			prettyPrint(order)
		} else {
			fmt.Printf("[%s] %s filled=%s/%s status=%s\n",
				dest, order.ID[:8], order.FilledQty, qty, order.Status)
		}

		// check if terminal
		switch order.Status {
		case alpaca.OrderStatusFilled:
			fmt.Printf("[%s] FILLED %s @ %s\n", dest, order.FilledQty, order.FilledAvgPrice)
			return order.FilledQty, order.FilledAvgPrice, nil
		case alpaca.OrderStatusCanceled, alpaca.OrderStatusExpired, alpaca.OrderStatusRejected:
			fmt.Printf("[%s] TERMINAL status=%s filled=%s\n", dest, order.Status, order.FilledQty)
			return order.FilledQty, order.FilledAvgPrice, nil
		}
	}
}

func liquidateCrypto(symbol string, side ds.Side, qty, price decimal.Decimal) (decimal.Decimal, decimal.Decimal, error) {
	fmt.Printf("[CRYPTO] placing %s %s @ %s\n", side, qty, price)

	// crypto: no DMA, use GTC time-in-force
	var order *alpaca.Order
	for {
		var err error
		order, err = client.LimitOrder(symbol, side, qty, price, alpaca.TimeInForceGTC, alpaca.OrderAlgorithmNone, alpaca.OrderDestinationNone, false)
		if err != nil {
			if err == ds.ErrTooManyRequests {
				clocky.Sleep(clocky.Second)
				continue
			}
			fmt.Printf("[CRYPTO] error placing order: %v\n", err)
			return decimal.Zero, decimal.Zero, err
		}
		break
	}

	// track order for cancellation on ctrl-c
	ordersMu.Lock()
	inFlightOrders[order.ID] = true
	ordersMu.Unlock()
	defer func() {
		ordersMu.Lock()
		delete(inFlightOrders, order.ID)
		ordersMu.Unlock()
	}()

	if *verbose {
		fmt.Printf("[CRYPTO] order placed:\n")
		prettyPrint(order)
	} else {
		fmt.Printf("[CRYPTO] order %s status=%s\n", order.ID[:8], order.Status)
	}

	// poll until filled or terminal
	pollInterval := 3 * time.Second
	for {
		select {
		case <-ctx.Done():
			return order.FilledQty, order.FilledAvgPrice, ctx.Err()
		case <-time.After(pollInterval):
		}

		order, err := client.GetOrder(order.ID)
		if err != nil {
			fmt.Printf("[CRYPTO] error polling: %v\n", err)
			continue
		}

		if *verbose {
			fmt.Printf("[CRYPTO] poll:\n")
			prettyPrint(order)
		} else {
			fmt.Printf("[CRYPTO] %s filled=%s/%s status=%s\n",
				order.ID[:8], order.FilledQty, qty, order.Status)
		}

		// check if terminal
		switch order.Status {
		case alpaca.OrderStatusFilled:
			fmt.Printf("[CRYPTO] FILLED %s @ %s\n", order.FilledQty, order.FilledAvgPrice)
			return order.FilledQty, order.FilledAvgPrice, nil
		case alpaca.OrderStatusCanceled, alpaca.OrderStatusExpired, alpaca.OrderStatusRejected:
			fmt.Printf("[CRYPTO] TERMINAL status=%s filled=%s\n", order.Status, order.FilledQty)
			return order.FilledQty, order.FilledAvgPrice, nil
		}
	}
}

func prettyPrint(v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Printf("%+v\n", v)
		return
	}
	// remove empty/zero fields for cleaner output
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
