// cmd/profit analyzes the impact of different profit thresholds on sell decisions.
// Usage: go run ./cmd/profit -dataset chaos -symbol BTC
package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/klauspost/compress/zstd"
)

var (
	flagDataset = flag.String("dataset", "chaos", "dataset name")
	flagSymbol  = flag.String("symbol", "BTC", "coinbase product to analyze")
	flagSamples = flag.Int("samples", 7000, "number of samples for baseline ema")
	flagSpread  = decimal.FlagBPS("spread", "2", "spread threshold in basis points")
	flagSize    = decimal.Flag("size", "200", "order size in USD")
)

// SellRecord tracks a completed sell transaction
type SellRecord struct {
	Time        clocky.Time
	Quantity    decimal.Decimal
	Cost        decimal.Decimal
	Proceeds    decimal.Decimal
	Profit      decimal.Decimal
	ProfitBPS   decimal.Decimal
	HoldingTime clocky.Duration
}

// ThresholdSim simulates trading with a specific profit threshold
type ThresholdSim struct {
	Threshold   decimal.Decimal
	Lots        *ds.Lots
	SellRecords []SellRecord
	BuyCount    int
}

func main() {
	flag.Parse()

	home := os.Getenv("HOME")

	// Determine binance symbol
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

	// Initialize state
	coinbaseBook := ds.NewBook()
	var binancePrice decimal.Decimal
	var coinbasePrice decimal.Decimal
	warmedUp := false

	// Create simulations for different profit thresholds (in basis points)
	thresholds := []decimal.Decimal{
		decimal.Parse("0.0002"), // 2 bps
		decimal.Parse("0.0003"), // 3 bps
		decimal.Parse("0.0005"), // 5 bps (current default)
		decimal.Parse("0.0007"), // 7 bps
		decimal.Parse("0.0010"), // 10 bps
	}

	sims := make([]*ThresholdSim, len(thresholds))
	for i, threshold := range thresholds {
		sims[i] = &ThresholdSim{
			Threshold:   threshold,
			Lots:        ds.NewLots(ds.CostBasisMethodLIFO),
			SellRecords: []SellRecord{},
			BuyCount:    0,
		}
	}

	// Read and merge ticks by time
	var coinbaseTick, binanceTick ds.Tick
	coinbaseErr := coinbaseReader.Read(&coinbaseTick)
	binanceErr := binanceReader.Read(&binanceTick)

	spreadThreshold := *flagSpread

	for coinbaseErr == nil || binanceErr == nil {
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
				}
			}

			// Check for signals
			if binancePrice.IsPositive() && coinbasePrice.IsPositive() {
				spread := coinbasePrice.Sub(binancePrice).Div(binancePrice)
				spreadEMA.Add(spread)

				if spreadEMA.IsReady() {
					if !warmedUp {
						warmedUp = true
					}

					baseline := spreadEMA.Value
					deviation := spread.Sub(baseline)

					// Buy signal: deviation < -threshold
					if deviation.Neg().Cmp(spreadThreshold) > 0 {
						executeBuy(sims, now, coinbasePrice)
					}

					// Sell signal: deviation > threshold
					if deviation.Cmp(spreadThreshold) > 0 {
						executeSell(sims, now, coinbasePrice)
					}
				}
			}

			coinbaseErr = coinbaseReader.Read(&coinbaseTick)
		}

		if processBinance {
			for _, trade := range binanceTick.Trades {
				if trade.Price.IsPositive() {
					binancePrice = trade.Price
				}
			}
			binanceErr = binanceReader.Read(&binanceTick)
		}
	}

	// Print results
	printResults(*flagSymbol, *flagDataset, sims)
}

func executeBuy(sims []*ThresholdSim, now clocky.Time, price decimal.Decimal) {
	// Calculate quantity from fixed USD size
	quantity := (*flagSize).Div(price)
	cost := (*flagSize) // total cost in USD

	// Add lot to all simulations
	for _, sim := range sims {
		sim.Lots.Add(now, quantity, cost)
		sim.BuyCount++
	}
}

func executeSell(sims []*ThresholdSim, now clocky.Time, price decimal.Decimal) {
	// Try to sell in each simulation independently
	quantity := (*flagSize).Div(price)

	for _, sim := range sims {
		// Check if we have enough inventory by computing total
		if sim.Lots.Empty() {
			continue
		}

		// Get cost basis for this quantity
		costBasis, err := sim.Lots.GetCostBasis(quantity, now, decimal.Zero)
		if err != nil {
			// Insufficient lots for this sale
			continue
		}

		// Apply profit threshold
		profitAdjustedCost := costBasis.Mul(decimal.One.Add(sim.Threshold))

		// Calculate proceeds from selling at current price
		proceeds := quantity.Mul(price)

		// Only sell if proceeds exceed cost + profit margin
		if proceeds.Cmp(profitAdjustedCost) >= 0 {
			// Get the top lot time for holding time calculation
			var oldestLotTime clocky.Time
			it := sim.Lots.Iterator()
			if it.Next() {
				oldestLotTime = it.Value().Time
			}
			holdingTime := now.Sub(oldestLotTime)

			// Record the sell
			profit := proceeds.Sub(costBasis)
			profitBPS := profit.Div(costBasis)

			record := SellRecord{
				Time:        now,
				Quantity:    quantity,
				Cost:        costBasis,
				Proceeds:    proceeds,
				Profit:      profit,
				ProfitBPS:   profitBPS,
				HoldingTime: holdingTime,
			}
			sim.SellRecords = append(sim.SellRecords, record)

			// Consume the lots
			sim.Lots.Consume(quantity, now, decimal.Zero)
		}
	}
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

func printResults(symbol, dataset string, sims []*ThresholdSim) {
	fmt.Println()
	fmt.Printf("# Profit Threshold Analysis: %s/%s\n", dataset, symbol)
	fmt.Println()

	// Print simulation parameters
	fmt.Println("## Simulation Parameters")
	fmt.Printf("Spread threshold: %s bps\n", (*flagSpread).BPS().Format(1))
	fmt.Printf("Samples for EMA:  %d\n", *flagSamples)
	fmt.Printf("Order size:       $%s\n", *flagSize)
	fmt.Println()

	// Print results table
	fmt.Println("## Results by Profit Threshold")
	fmt.Println()
	fmt.Printf("%-10s  %6s  %12s  %10s  %10s  %10s  %12s\n",
		"Threshold", "Sells", "Avg Profit", "Win Rate", "Med Hold", "P95 Hold", "Total P&L")

	for _, sim := range sims {
		sellCount := len(sim.SellRecords)
		if sellCount == 0 {
			fmt.Printf("%5s bps  %6d  %12s  %10s  %10s  %10s  %12s\n",
				sim.Threshold.BPS(), 0, "N/A", "N/A", "N/A", "N/A", "$0.00")
			continue
		}

		// Calculate statistics
		var totalProfit decimal.Decimal
		var totalProfitBPS decimal.Decimal
		winCount := 0
		holdingTimes := make([]float64, sellCount)

		for i, record := range sim.SellRecords {
			totalProfit = totalProfit.Add(record.Profit)
			totalProfitBPS = totalProfitBPS.Add(record.ProfitBPS)
			if record.Profit.IsPositive() {
				winCount++
			}
			holdingTimes[i] = float64(record.HoldingTime)
		}

		avgProfit := totalProfitBPS.Div(decimal.FromInt(sellCount))
		winRate := float64(winCount) / float64(sellCount) * 100

		// Calculate holding time percentiles
		sort.Float64s(holdingTimes)
		medianHold := clocky.Duration(holdingTimes[sellCount/2])
		p95Hold := clocky.Duration(holdingTimes[sellCount*95/100])

		// Mark current default (5 bps)
		marker := ""
		if sim.Threshold.BPS().String() == "5" {
			marker = " ⭐"
		}

		fmt.Printf("%5s bps  %6d  %+8s bps  %9.1f%%  %10s  %10s  %12s%s\n",
			sim.Threshold.BPS(),
			sellCount,
			avgProfit.BPS().Format(2),
			winRate,
			formatDuration(medianHold),
			formatDuration(p95Hold),
			totalProfit.Format(2),
			marker)
	}

	fmt.Println()

	// Print trade-off analysis
	fmt.Println("## Trade-off Analysis")

	// Find best by total P&L
	var bestPL *ThresholdSim
	var bestPLValue decimal.Decimal
	for _, sim := range sims {
		if len(sim.SellRecords) == 0 {
			continue
		}
		var total decimal.Decimal
		for _, record := range sim.SellRecords {
			total = total.Add(record.Profit)
		}
		if bestPL == nil || total.Cmp(bestPLValue) > 0 {
			bestPL = sim
			bestPLValue = total
		}
	}

	if bestPL != nil {
		fmt.Printf("Lower threshold:  More trades, faster turnover, potentially higher total profit\n")
		fmt.Printf("Higher threshold: Fewer trades, bigger gains/trade, longer holding times\n")
		fmt.Println()
		fmt.Printf("Based on total P&L: %s bps maximizes profit for this dataset\n", bestPL.Threshold.BPS())
	}
}

func formatDuration(d clocky.Duration) string {
	if d == 0 {
		return "0s"
	}

	minutes := int(d / clocky.Minute)
	seconds := int((d % clocky.Minute) / clocky.Second)

	if minutes == 0 {
		return fmt.Sprintf("%ds", seconds)
	}

	hours := minutes / 60
	mins := minutes % 60

	if hours == 0 {
		return fmt.Sprintf("%dm %ds", mins, seconds)
	}

	return fmt.Sprintf("%dh %dm", hours, mins)
}
