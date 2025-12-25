// cmd/disposition analyzes effectiveness of the price-range disposition filter.
// Usage: go run ./cmd/disposition -dataset chaos -symbol BTC
package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"flag"
	"fmt"
	"os"

	"github.com/klauspost/compress/zstd"
)

var (
	flagDataset   = flag.String("dataset", "chaos", "dataset name")
	flagSymbol    = flag.String("symbol", "BTC", "coinbase product to analyze")
	flagWindow    = clocky.DurationFlag("window", "42m", "time window for min/max range")
	flagComfort   = decimal.FlagPercent("comfort", "20", "comfort zone percentage")
	flagSamples   = flag.Int("samples", 7000, "number of samples for baseline ema")
	flagSpread    = decimal.Flag("spread", "2", "spread threshold in basis points")
	flagLookahead = clocky.DurationFlag("lookahead", "5m", "time to look ahead for outcome")
)

// Disposition represents price position in recent range
type Disposition int

const (
	TooCheap Disposition = iota
	Cheap
	Normal
	Expensive
	TooExpensive
)

func (d Disposition) String() string {
	return [...]string{"TooCheap", "Cheap", "Normal", "Expensive", "TooExpensive"}[d]
}

// Signal represents a trading signal
type Signal struct {
	Time        clocky.Time
	Disposition Disposition
	Deviation   decimal.Decimal
	Side        ds.Side // SideBuy or SideSell
	Price       decimal.Decimal
}

// Outcome tracks what happened after a signal
type Outcome struct {
	Signal      Signal
	FuturePrice decimal.Decimal
	ProfitBPS   float64
}

func main() {
	flag.Parse()

	home := os.Getenv("HOME")

	// Determine binance symbol (no dash, e.g., BTCFDUSD)
	binanceSymbol := *flagSymbol + "FDUSD"
	if *flagSymbol == "ZEC" {
		binanceSymbol = *flagSymbol + "USDT"
	}

	// Open market data files
	coinbasePath := home + "/marketdata/" + *flagDataset + "/coinbase/" + *flagSymbol + "-USD"
	binancePath := home + "/marketdata/" + *flagDataset + "/binance/" + binanceSymbol

	coinbaseReader := openMarketData(coinbasePath)
	binanceReader := openMarketData(binancePath)
	defer coinbaseReader.Close()
	defer binanceReader.Close()

	// Initialize indicators
	spreadEMA := indicators.NewWWMA(*flagSamples)
	priceMin := indicators.NewMin(*flagWindow)
	priceMax := indicators.NewMax(*flagWindow)

	// Initialize state
	coinbaseBook := ds.NewBook()
	var binancePrice decimal.Decimal
	var coinbasePrice decimal.Decimal
	warmedUp := false

	// Track disposition distribution and signals
	dispCounts := make(map[Disposition]int)
	var signals []Signal
	type pricePoint struct {
		time  clocky.Time
		price decimal.Decimal
	}
	var priceHistory []pricePoint // for lookahead

	// Read and merge ticks by time
	var coinbaseTick, binanceTick ds.Tick
	coinbaseErr := coinbaseReader.Read(&coinbaseTick)
	binanceErr := binanceReader.Read(&binanceTick)

	spreadThreshold := (*flagSpread).Div(decimal.Parse("10000")) // convert bps to decimal

	for coinbaseErr == nil || binanceErr == nil {
		// Process the earlier tick
		processCoinbase := false
		processBinance := false

		if coinbaseErr != nil {
			processBinance = true
		} else if binanceErr != nil {
			processCoinbase = true
		} else if coinbaseTick.Time.Before(binanceTick.Time) {
			processCoinbase = true
		} else {
			processBinance = true
		}

		if processCoinbase {
			now := coinbaseTick.Time

			// Update order book
			coinbaseBook.Lock.Lock()
			if coinbaseTick.Snap {
				coinbaseBook.Clear()
			}
			for _, bid := range coinbaseTick.Bids {
				coinbaseBook.UpdateBid(bid.Price, bid.Size)
			}
			for _, ask := range coinbaseTick.Asks {
				coinbaseBook.UpdateAsk(ask.Price, ask.Size)
			}
			coinbaseBook.Lock.Unlock()

			// Update price from trades
			for _, trade := range coinbaseTick.Trades {
				if trade.Price.IsPositive() {
					coinbasePrice = trade.Price
					priceMin.Add(now, coinbasePrice)
					priceMax.Add(now, coinbasePrice)
					priceHistory = append(priceHistory, pricePoint{now, coinbasePrice})
				}
			}

			// Record signal if conditions met
			if binancePrice.IsPositive() && coinbasePrice.IsPositive() && priceMin.IsReady() {
				// Compute spread and deviation
				spread := coinbasePrice.Sub(binancePrice).Div(binancePrice)
				spreadEMA.Add(spread)

				if spreadEMA.IsReady() {
					if !warmedUp {
						warmedUp = true
					}

					baseline := spreadEMA.Value
					deviation := spread.Sub(baseline)

					// Compute disposition
					minPrice := priceMin.Value
					maxPrice := priceMax.Value
					disposition := computeDisposition(coinbasePrice, minPrice, maxPrice, *flagComfort)
					dispCounts[disposition]++

					// Check for signals (ignoring disposition filter to compare)
					if deviation.Neg().Cmp(spreadThreshold) > 0 {
						// Buy signal
						signals = append(signals, Signal{
							Time:        now,
							Disposition: disposition,
							Deviation:   deviation,
							Side:        ds.SideBuy,
							Price:       coinbasePrice,
						})
					} else if deviation.Cmp(spreadThreshold) > 0 {
						// Sell signal
						signals = append(signals, Signal{
							Time:        now,
							Disposition: disposition,
							Deviation:   deviation,
							Side:        ds.SideSell,
							Price:       coinbasePrice,
						})
					}
				}
			}

			coinbaseErr = coinbaseReader.Read(&coinbaseTick)
		}

		if processBinance {
			// Update binance price from trades
			for _, trade := range binanceTick.Trades {
				if trade.Price.IsPositive() {
					binancePrice = trade.Price
				}
			}

			binanceErr = binanceReader.Read(&binanceTick)
		}
	}

	// Compute outcomes for signals using binary search
	var outcomes []Outcome
	for _, sig := range signals {
		futureTime := sig.Time.Add(*flagLookahead)
		// Binary search for first price at or after futureTime
		lo, hi := 0, len(priceHistory)
		for lo < hi {
			mid := (lo + hi) / 2
			if priceHistory[mid].time.Before(futureTime) {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		if lo < len(priceHistory) {
			futurePrice := priceHistory[lo].price
			var profitBPS float64
			if sig.Side == ds.SideBuy {
				profitBPS = futurePrice.Sub(sig.Price).Div(sig.Price).BPS().Float64()
			} else {
				profitBPS = sig.Price.Sub(futurePrice).Div(sig.Price).BPS().Float64()
			}
			outcomes = append(outcomes, Outcome{
				Signal:      sig,
				FuturePrice: futurePrice,
				ProfitBPS:   profitBPS,
			})
		}
	}

	// Print results
	printResults(*flagSymbol, *flagDataset, dispCounts, signals, outcomes)
}

func computeDisposition(price, minPrice, maxPrice decimal.Decimal, comfort decimal.Decimal) Disposition {
	rangeSize := maxPrice.Sub(minPrice)
	if !rangeSize.IsPositive() {
		return Normal
	}
	rangePosition := price.Sub(minPrice).Div(rangeSize)

	if rangePosition.Cmp(comfort) < 0 {
		return Cheap
	}
	if rangePosition.Cmp(decimal.One.Sub(comfort)) > 0 {
		return Expensive
	}
	return Normal
}

type marketDataReader struct {
	file   *os.File
	reader *zstd.Decoder
}

func openMarketData(path string) *marketDataReader {
	file, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open %s: %v\n", path, err)
		os.Exit(1)
	}
	reader, err := zstd.NewReader(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create zstd reader: %v\n", err)
		os.Exit(1)
	}
	return &marketDataReader{file: file, reader: reader}
}

func (m *marketDataReader) Read(tick *ds.Tick) error {
	return tick.Deserialize(m.reader)
}

func (m *marketDataReader) Close() {
	m.reader.Close()
	m.file.Close()
}

func printResults(symbol, dataset string, dispCounts map[Disposition]int, signals []Signal, outcomes []Outcome) {
	fmt.Println()
	fmt.Printf("# Disposition Effectiveness Analysis: %s/%s\n", dataset, symbol)
	fmt.Println()

	// Disposition distribution
	fmt.Println("## Disposition Distribution")
	total := 0
	for _, c := range dispCounts {
		total += c
	}
	for d := TooCheap; d <= TooExpensive; d++ {
		count := dispCounts[d]
		pct := float64(count) / float64(total) * 100
		fmt.Printf("%-12s: %6.2f%% (%d samples)\n", d, pct, count)
	}
	fmt.Println()

	// Signal distribution by disposition
	fmt.Println("## Signals by Disposition")
	buyByDisp := make(map[Disposition]int)
	sellByDisp := make(map[Disposition]int)
	for _, sig := range signals {
		if sig.Side == ds.SideBuy {
			buyByDisp[sig.Disposition]++
		} else {
			sellByDisp[sig.Disposition]++
		}
	}
	fmt.Printf("%-12s  %8s  %8s\n", "Disposition", "Buy", "Sell")
	for d := TooCheap; d <= TooExpensive; d++ {
		fmt.Printf("%-12s  %8d  %8d\n", d, buyByDisp[d], sellByDisp[d])
	}
	fmt.Println()

	// Outcome analysis by disposition
	fmt.Println("## Outcome by Disposition (profit in bps)")
	type stats struct {
		count      int
		sumProfit  float64
		profitable int
	}
	buyStats := make(map[Disposition]*stats)
	sellStats := make(map[Disposition]*stats)
	for d := TooCheap; d <= TooExpensive; d++ {
		buyStats[d] = &stats{}
		sellStats[d] = &stats{}
	}

	for _, o := range outcomes {
		var s *stats
		if o.Signal.Side == ds.SideBuy {
			s = buyStats[o.Signal.Disposition]
		} else {
			s = sellStats[o.Signal.Disposition]
		}
		s.count++
		s.sumProfit += o.ProfitBPS
		if o.ProfitBPS > 0 {
			s.profitable++
		}
	}

	fmt.Println()
	fmt.Println("### Buy Signals")
	fmt.Printf("%-12s  %6s  %10s  %8s\n", "Disposition", "Count", "Avg Profit", "Win Rate")
	for d := TooCheap; d <= TooExpensive; d++ {
		s := buyStats[d]
		if s.count > 0 {
			avgProfit := s.sumProfit / float64(s.count)
			winRate := float64(s.profitable) / float64(s.count) * 100
			fmt.Printf("%-12s  %6d  %+10.2f  %7.1f%%\n", d, s.count, avgProfit, winRate)
		}
	}

	fmt.Println()
	fmt.Println("### Sell Signals")
	fmt.Printf("%-12s  %6s  %10s  %8s\n", "Disposition", "Count", "Avg Profit", "Win Rate")
	for d := TooCheap; d <= TooExpensive; d++ {
		s := sellStats[d]
		if s.count > 0 {
			avgProfit := s.sumProfit / float64(s.count)
			winRate := float64(s.profitable) / float64(s.count) * 100
			fmt.Printf("%-12s  %6d  %+10.2f  %7.1f%%\n", d, s.count, avgProfit, winRate)
		}
	}

	// Summary comparison
	fmt.Println()
	fmt.Println("## Filter Effectiveness")
	filteredBuys := buyStats[Cheap]
	allBuys := &stats{}
	for _, s := range buyStats {
		allBuys.count += s.count
		allBuys.sumProfit += s.sumProfit
		allBuys.profitable += s.profitable
	}

	filteredSells := sellStats[Expensive]
	allSells := &stats{}
	for _, s := range sellStats {
		allSells.count += s.count
		allSells.sumProfit += s.sumProfit
		allSells.profitable += s.profitable
	}

	if allBuys.count > 0 && filteredBuys.count > 0 {
		allBuyAvg := allBuys.sumProfit / float64(allBuys.count)
		filtBuyAvg := filteredBuys.sumProfit / float64(filteredBuys.count)
		allBuyWin := float64(allBuys.profitable) / float64(allBuys.count) * 100
		filtBuyWin := float64(filteredBuys.profitable) / float64(filteredBuys.count) * 100

		fmt.Printf("Buys  ALL:    n=%5d avg=%+.2fbps win=%.1f%%\n", allBuys.count, allBuyAvg, allBuyWin)
		fmt.Printf("Buys  CHEAP:  n=%5d avg=%+.2fbps win=%.1f%% (filter keeps %.1f%%)\n",
			filteredBuys.count, filtBuyAvg, filtBuyWin,
			float64(filteredBuys.count)/float64(allBuys.count)*100)
	}

	if allSells.count > 0 && filteredSells.count > 0 {
		allSellAvg := allSells.sumProfit / float64(allSells.count)
		filtSellAvg := filteredSells.sumProfit / float64(filteredSells.count)
		allSellWin := float64(allSells.profitable) / float64(allSells.count) * 100
		filtSellWin := float64(filteredSells.profitable) / float64(filteredSells.count) * 100

		fmt.Printf("Sells ALL:    n=%5d avg=%+.2fbps win=%.1f%%\n", allSells.count, allSellAvg, allSellWin)
		fmt.Printf("Sells EXPENSIVE: n=%5d avg=%+.2fbps win=%.1f%% (filter keeps %.1f%%)\n",
			filteredSells.count, filtSellAvg, filtSellWin,
			float64(filteredSells.count)/float64(allSells.count)*100)
	}
}
