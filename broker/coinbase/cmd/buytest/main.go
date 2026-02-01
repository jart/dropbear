// buytest buys a small amount of BTC using USDC.
//
// This command places a limit IOC order on the BTC-USDC pair. It uses the IOC
// (immediate or cancel) strategy with a price above market to ensure immediate
// fill, similar to a market order but with slippage protection.
//
// Usage:
//
//	go run ./broker/coinbase/cmd/buytest
//	go run ./broker/coinbase/cmd/buytest -dry           # dry run, don't actually trade
//	go run ./broker/coinbase/cmd/buytest -qty 0.0001    # buy 0.0001 BTC instead
//	go run ./broker/coinbase/cmd/buytest -slippage 0.02 # allow 2% slippage (default 1%)
package main

import (
	"dropbear/broker/coinbase"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"flag"
	"log"
)

var (
	dryRun   = flag.Bool("dry", false, "dry run, don't actually trade")
	qty      = flag.String("qty", "0.000064", "quantity of BTC to buy")
	slippage = flag.Float64("slippage", 0.01, "slippage tolerance (0.01 = 1%)")
)

func main() {
	flag.Parse()
	loggy.Init()

	// Create client
	client := coinbase.NewClient()
	defer client.Close()

	// Get accounts to check USDC balance
	accounts, err := client.GetAccounts()
	if err != nil {
		loggy.Fatalf("getting accounts: %v", err)
	}
	var usdcBalance decimal.Decimal
	var btcBalance decimal.Decimal
	for _, acc := range accounts {
		if acc.Currency == "USDC" {
			usdcBalance = acc.AvailableBalance.Value
			log.Printf("USDC balance: %s", usdcBalance)
		}
		if acc.Currency == "BTC" {
			btcBalance = acc.AvailableBalance.Value
			log.Printf("BTC balance: %s", btcBalance)
		}
	}

	// Get BTC-USDC product info
	product, err := client.GetProduct("BTC-USDC")
	if err != nil {
		loggy.Fatalf("getting BTC-USDC product: %v", err)
	}
	log.Printf("BTC-USDC product:")
	log.Printf("  status: %s", product.Status)
	log.Printf("  price: %s", product.Price)
	log.Printf("  base_min_size: %s", product.BaseMinSize)
	log.Printf("  base_max_size: %s", product.BaseMaxSize)
	log.Printf("  base_increment: %s", product.BaseIncrement)
	log.Printf("  quote_increment: %s", product.QuoteIncrement)

	if product.TradingDisabled {
		loggy.Fatalf("trading is disabled for BTC-USDC")
	}

	// Parse quantity
	quantity := decimal.Parse(*qty)
	baseMinSize := decimal.Parse(product.BaseMinSize)
	baseIncrement := decimal.Parse(product.BaseIncrement)

	// Check minimum size
	if quantity.Cmp(baseMinSize) < 0 {
		log.Printf("warning: quantity %s is below minimum %s, using minimum", quantity, baseMinSize)
		quantity = baseMinSize
	}

	// Quantize to base increment
	quantity = quantity.QuantizeTruncate(baseIncrement)
	log.Printf("quantity to buy: %s BTC", quantity)

	// Calculate limit price with slippage
	currentPrice := decimal.Parse(product.Price)
	slippageMultiplier := decimal.FromFloat64(1.0 + *slippage)
	limitPrice := currentPrice.Mul(slippageMultiplier)

	// Quantize to quote increment
	quoteIncrement := decimal.Parse(product.QuoteIncrement)
	limitPrice = limitPrice.QuantizeAway(quoteIncrement)
	log.Printf("current price: %s USDC", currentPrice)
	log.Printf("limit price (with %.1f%% slippage): %s USDC", *slippage*100, limitPrice)

	// Calculate estimated cost
	estimatedCost := quantity.Mul(limitPrice)
	log.Printf("estimated max cost: %s USDC", estimatedCost)

	// Check if we have enough USDC
	if usdcBalance.Cmp(estimatedCost) < 0 {
		loggy.Fatalf("insufficient USDC balance: have %s, need %s", usdcBalance, estimatedCost)
	}

	if *dryRun {
		log.Printf("[DRY RUN] would place limit IOC order: buy %s BTC at limit %s USDC", quantity, limitPrice)
		return
	}

	// Place the order
	log.Printf("placing limit IOC order: buy %s BTC at limit %s USDC", quantity, limitPrice)
	orderID, err := client.LimitOrder(
		"BTC-USDC",
		ds.SideBuy,
		quantity,
		limitPrice,
		"",
		ds.OrderStrategyIOC,
		ds.CostBasisMethodHIFO,
	)
	if err != nil {
		loggy.Fatalf("placing order: %v", err)
	}
	log.Printf("order placed successfully, order ID: %s", orderID)

	// Get the order to check fill status
	order, err := client.GetOrder(orderID)
	if err != nil {
		log.Printf("warning: could not get order status: %v", err)
		return
	}
	log.Printf("order status: %s", order.Status)
	log.Printf("filled size: %s", order.FilledSize)
	log.Printf("filled value: %s", order.FilledValue)
	log.Printf("average fill price: %s", order.AverageFilledPrice)
	log.Printf("total fees: %s", order.TotalFees)
}
