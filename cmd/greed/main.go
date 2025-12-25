// cmd/greed analyzes the exponential greed/inventory skew parameters.
// Usage: go run ./cmd/greed -dataset chaos -symbol BTC
package main

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/klauspost/compress/zstd"
)

var (
	flagDataset = flag.String("dataset", "chaos", "dataset name")
	flagSymbol  = flag.String("symbol", "BTC", "coinbase product to analyze")
	flagSamples = flag.Int("samples", 7000, "number of samples for baseline ema")
	flagSpread  = decimal.Flag("spread", "2", "spread threshold in basis points")
	flagTarget  = decimal.Flag("target", "5000", "target inventory in USD")
	flagSkew    = decimal.Flag("skew", "1", "exponential skew coefficient")
)

// Trade represents a simulated trade
type Trade struct {
	Time         clocky.Time
	Side         ds.Side
	Price        decimal.Decimal
	Deviation    decimal.Decimal
	Greed        decimal.Decimal
	InventoryPre decimal.Decimal
	Imbalance    decimal.Decimal
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

	// Simulation state
	inventoryQty := decimal.Zero   // quantity held
	inventoryValue := decimal.Zero // value of inventory
	target := *flagTarget

	// Track trades and metrics
	var trades []Trade
	var greedFactors []float64
	var imbalances []float64

	// Read and merge ticks by time
	var coinbaseTick, binanceTick ds.Tick
	coinbaseErr := coinbaseReader.Read(&coinbaseTick)
	binanceErr := binanceReader.Read(&binanceTick)

	spreadThreshold := (*flagSpread).Div(decimal.Parse("10000"))

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

					// Calculate inventory metrics
					inventoryValue = inventoryQty.Mul(coinbasePrice)
					imbalance := decimal.Zero
					greed := decimal.One
					buySpread := spreadThreshold
					sellSpread := spreadThreshold

					if target.IsPositive() {
						imbalance = inventoryValue.Sub(target).Div(target)
						greed = imbalance.Mul(*flagSkew).Exp()
						buySpread = spreadThreshold.Mul(greed)
						sellSpread = spreadThreshold.Div(greed)
					}

					greedFactors = append(greedFactors, greed.Float64())
					imbalances = append(imbalances, imbalance.Float64())

					// Check for trade signals with greed-adjusted spreads
					if deviation.Neg().Cmp(buySpread) > 0 {
						// Buy signal
						// Simulate buying $100 worth
						buyQty := decimal.FromInt(100).Div(coinbasePrice)
						inventoryQty = inventoryQty.Add(buyQty)

						trades = append(trades, Trade{
							Time:         now,
							Side:         ds.SideBuy,
							Price:        coinbasePrice,
							Deviation:    deviation,
							Greed:        greed,
							InventoryPre: inventoryValue,
							Imbalance:    imbalance,
						})
					} else if deviation.Cmp(sellSpread) > 0 && inventoryQty.IsPositive() {
						// Sell signal (only if we have inventory)
						sellQty := decimal.FromInt(100).Div(coinbasePrice)
						if sellQty.Cmp(inventoryQty) > 0 {
							sellQty = inventoryQty
						}
						inventoryQty = inventoryQty.Sub(sellQty)

						trades = append(trades, Trade{
							Time:         now,
							Side:         ds.SideSell,
							Price:        coinbasePrice,
							Deviation:    deviation,
							Greed:        greed,
							InventoryPre: inventoryValue,
							Imbalance:    imbalance,
						})
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
	printResults(*flagSymbol, *flagDataset, trades, greedFactors, imbalances, *flagSkew)
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

func printResults(symbol, dataset string, trades []Trade, greedFactors, imbalances []float64, skew decimal.Decimal) {
	fmt.Println()
	fmt.Printf("# Greed/Skew Analysis: %s/%s (skew=%.1f)\n", dataset, symbol, skew.Float64())
	fmt.Println()

	// Greed factor distribution
	fmt.Println("## Greed Factor Distribution")
	if len(greedFactors) > 0 {
		sort.Float64s(greedFactors)
		n := len(greedFactors)
		sum := 0.0
		for _, g := range greedFactors {
			sum += g
		}
		mean := sum / float64(n)

		variance := 0.0
		for _, g := range greedFactors {
			diff := g - mean
			variance += diff * diff
		}
		stddev := math.Sqrt(variance / float64(n))

		fmt.Printf("Samples: %d\n", n)
		fmt.Printf("Mean:    %.4f\n", mean)
		fmt.Printf("StdDev:  %.4f\n", stddev)
		fmt.Printf("Min:     %.4f\n", greedFactors[0])
		fmt.Printf("Max:     %.4f\n", greedFactors[n-1])
		fmt.Printf("P5:      %.4f\n", greedFactors[n*5/100])
		fmt.Printf("P50:     %.4f\n", greedFactors[n/2])
		fmt.Printf("P95:     %.4f\n", greedFactors[n*95/100])
	}
	fmt.Println()

	// Imbalance distribution
	fmt.Println("## Imbalance Distribution")
	if len(imbalances) > 0 {
		sort.Float64s(imbalances)
		n := len(imbalances)
		sum := 0.0
		for _, i := range imbalances {
			sum += i
		}
		mean := sum / float64(n)

		// Count time in each zone
		under50 := 0 // -100% to -50%
		under25 := 0 // -50% to -25%
		near := 0    // -25% to +25%
		over25 := 0  // +25% to +50%
		over50 := 0  // +50% to +100%

		for _, imb := range imbalances {
			switch {
			case imb < -0.5:
				under50++
			case imb < -0.25:
				under25++
			case imb < 0.25:
				near++
			case imb < 0.5:
				over25++
			default:
				over50++
			}
		}

		fmt.Printf("Mean Imbalance: %+.2f%%\n", mean*100)
		fmt.Printf("Min:            %+.2f%%\n", imbalances[0]*100)
		fmt.Printf("Max:            %+.2f%%\n", imbalances[n-1]*100)
		fmt.Println()
		fmt.Println("Time in Zone:")
		fmt.Printf("  Very Underweight (<-50%%):  %5.1f%%\n", float64(under50)/float64(n)*100)
		fmt.Printf("  Underweight (-50%% to -25%%): %5.1f%%\n", float64(under25)/float64(n)*100)
		fmt.Printf("  Near Target (-25%% to +25%%): %5.1f%%\n", float64(near)/float64(n)*100)
		fmt.Printf("  Overweight (+25%% to +50%%):  %5.1f%%\n", float64(over25)/float64(n)*100)
		fmt.Printf("  Very Overweight (>+50%%):   %5.1f%%\n", float64(over50)/float64(n)*100)
	}
	fmt.Println()

	// Trade analysis by imbalance zone
	fmt.Println("## Trades by Imbalance Zone")
	type zoneStats struct {
		buys  int
		sells int
	}
	zones := map[string]*zoneStats{
		"under50": {},
		"under25": {},
		"near":    {},
		"over25":  {},
		"over50":  {},
	}

	for _, t := range trades {
		imb := t.Imbalance.Float64()
		var zone string
		switch {
		case imb < -0.5:
			zone = "under50"
		case imb < -0.25:
			zone = "under25"
		case imb < 0.25:
			zone = "near"
		case imb < 0.5:
			zone = "over25"
		default:
			zone = "over50"
		}
		if t.Side == ds.SideBuy {
			zones[zone].buys++
		} else {
			zones[zone].sells++
		}
	}

	fmt.Printf("%-20s  %6s  %6s  %8s\n", "Zone", "Buys", "Sells", "Ratio")
	for _, name := range []string{"under50", "under25", "near", "over25", "over50"} {
		z := zones[name]
		total := z.buys + z.sells
		ratio := "N/A"
		if z.sells > 0 {
			ratio = fmt.Sprintf("%.2f", float64(z.buys)/float64(z.sells))
		}
		label := map[string]string{
			"under50": "Very Underweight",
			"under25": "Underweight",
			"near":    "Near Target",
			"over25":  "Overweight",
			"over50":  "Very Overweight",
		}[name]
		if total > 0 {
			fmt.Printf("%-20s  %6d  %6d  %8s\n", label, z.buys, z.sells, ratio)
		}
	}
	fmt.Println()

	// Summary
	fmt.Println("## Trade Summary")
	buyCount := 0
	sellCount := 0
	for _, t := range trades {
		if t.Side == ds.SideBuy {
			buyCount++
		} else {
			sellCount++
		}
	}
	fmt.Printf("Total Buys:  %d\n", buyCount)
	fmt.Printf("Total Sells: %d\n", sellCount)
	fmt.Printf("Buy/Sell:    %.2f\n", float64(buyCount)/float64(sellCount))
}
