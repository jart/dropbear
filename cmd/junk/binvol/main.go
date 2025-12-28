package main

import (
	"dropbear/decimal"
	"dropbear/exchange/binance"
	"dropbear/exchange/binanceusd"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

type row struct {
	symbol    string
	market    string // "spot" or "perp"
	volume    float64
	lastPrice decimal.Decimal
}

// Common quote assets that need USD conversion
var quoteAssets = []string{
	"USDT", "USDC", "BUSD", "FDUSD", "TUSD", "USD", // stablecoins (1:1)
	"BTC", "ETH", "BNB", "XRP", "DOGE", "TRX", "EUR", "TRY", "BRL", "DAI",
}

func main() {
	var rows []row

	// fetch spot and futures tickers in parallel
	spotCh := make(chan []*binance.Ticker)
	futuresCh := make(chan []*binanceusd.Ticker)
	errCh := make(chan error, 2)

	go func() {
		tickers, err := binance.NewClient().GetTickers24hr()
		if err != nil {
			errCh <- fmt.Errorf("spot: %w", err)
			spotCh <- nil
			return
		}
		errCh <- nil
		spotCh <- tickers
	}()

	go func() {
		tickers, err := binanceusd.NewClient().GetTickers()
		if err != nil {
			errCh <- fmt.Errorf("futures: %w", err)
			futuresCh <- nil
			return
		}
		errCh <- nil
		futuresCh <- tickers
	}()

	// collect results
	spotTickers := <-spotCh
	futuresTickers := <-futuresCh
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	// build price map for quote asset -> USD conversion
	// first pass: get USD prices for major assets
	priceUSD := map[string]float64{
		"USDT":  1.0,
		"USDC":  1.0,
		"BUSD":  1.0,
		"FDUSD": 1.0,
		"TUSD":  1.0,
		"USD":   1.0,
		"DAI":   1.0,
	}

	// find XXXUSDT prices to get USD value of other quote assets
	for _, t := range spotTickers {
		for _, quote := range []string{"BTC", "ETH", "BNB", "XRP", "DOGE", "TRX"} {
			if t.Symbol == quote+"USDT" {
				priceUSD[quote] = parseFloat(t.LastPrice)
				break
			}
		}
		// EUR and other fiat via USDT pairs
		if t.Symbol == "EURUSDT" {
			priceUSD["EUR"] = parseFloat(t.LastPrice)
		}
	}

	// process spot tickers - include all pairs
	for _, t := range spotTickers {
		quote := getQuoteAsset(t.Symbol)
		if quote == "" {
			continue // unknown quote asset
		}
		usdMultiplier, ok := priceUSD[quote]
		if !ok {
			continue // can't convert to USD
		}
		volumeUSD := parseFloat(t.QuoteVolume) * usdMultiplier
		rows = append(rows, row{
			symbol:    t.Symbol,
			market:    "spot",
			volume:    volumeUSD,
			lastPrice: decimal.Parse(t.LastPrice),
		})
	}

	// process futures tickers (already USD-denominated)
	for _, t := range futuresTickers {
		if !isUSDQuote(t.Symbol) {
			continue
		}
		rows = append(rows, row{
			symbol:    t.Symbol,
			market:    "perp",
			volume:    parseFloat(t.QuoteVolume),
			lastPrice: decimal.Parse(t.LastPrice),
		})
	}

	// sort by volume descending
	slices.SortFunc(rows, func(a, b row) int {
		if b.volume > a.volume {
			return 1
		} else if b.volume < a.volume {
			return -1
		}
		return 0
	})

	// print results
	fmt.Printf("%-16s %-6s %16s %14s\n", "SYMBOL", "MARKET", "24H VOLUME USD", "LAST PRICE")
	fmt.Printf("%-16s %-6s %16s %14s\n", "------", "------", "--------------", "----------")
	for _, r := range rows {
		fmt.Printf("%-16s %-6s %16s %14s\n",
			r.symbol,
			r.market,
			formatVolume(r.volume),
			r.lastPrice.String())
	}
	fmt.Printf("\n%d symbols\n", len(rows))
}

func getQuoteAsset(symbol string) string {
	for _, q := range quoteAssets {
		if strings.HasSuffix(symbol, q) {
			return q
		}
	}
	return ""
}

func isUSDQuote(symbol string) bool {
	return strings.HasSuffix(symbol, "USDT") ||
		strings.HasSuffix(symbol, "USDC") ||
		strings.HasSuffix(symbol, "BUSD") ||
		strings.HasSuffix(symbol, "FDUSD") ||
		strings.HasSuffix(symbol, "TUSD") ||
		strings.HasSuffix(symbol, "USD")
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func formatVolume(v float64) string {
	switch {
	case v >= 1e9:
		return fmt.Sprintf("%.2fB", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.2fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.2fK", v/1e3)
	default:
		return fmt.Sprintf("%.2f", v)
	}
}
