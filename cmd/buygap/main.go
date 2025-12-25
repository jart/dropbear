// cmd/buygap analyzes buy gap protection and decay function effectiveness.
// Usage: go run ./cmd/buygap -dataset chaos -symbol BTC
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
	flagDataset  = flag.String("dataset", "chaos", "dataset name")
	flagSymbol   = flag.String("symbol", "BTC", "coinbase product to analyze")
	flagSamples  = flag.Int("samples", 7000, "number of samples for baseline ema")
	flagSpread   = decimal.Flag("spread", "2", "spread threshold in basis points")
	flagTarget   = decimal.Flag("target", "5000", "target inventory in USD")
	flagBuyGap   = decimal.FlagBPS("buygap", "5", "only buy if price is this many basis points below last buy")
	flagBuyDecay = clocky.DurationFlag("decay", "30s", "base decay period for buygap after sells")
)

// BuyEvent represents a potential buy with gap protection logic
type BuyEvent struct {
	Time           clocky.Time
	Price          decimal.Decimal
	Deviation      decimal.Decimal
	TopCost        decimal.Decimal
	InventoryValue decimal.Decimal
	InventoryScale decimal.Decimal
	DecayFactor    decimal.Decimal
	EffectiveGap   decimal.Decimal
	ActualGap      decimal.Decimal
	Blocked        bool
	TimeSinceSell  clocky.Duration
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

	// Simulation state (tracking inventory like the real bot)
	inventoryQty := decimal.Zero
	var topCost decimal.Decimal
	var lastSellTime clocky.Time

	// Track buy events
	var buyEvents []BuyEvent

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

			// Check for buy signals
			if binancePrice.IsPositive() && coinbasePrice.IsPositive() {
				spread := coinbasePrice.Sub(binancePrice).Div(binancePrice)
				spreadEMA.Add(spread)

				if spreadEMA.IsReady() {
					if !warmedUp {
						warmedUp = true
					}

					baseline := spreadEMA.Value
					deviation := spread.Sub(baseline)

					// Check for buy signal (negative deviation = coinbase cheaper than expected)
					if deviation.Neg().Cmp(spreadThreshold) > 0 {
						// This is a buy signal - analyze gap protection
						buyPrice := coinbasePrice
						inventoryValue := inventoryQty.Mul(buyPrice)

						// Calculate inventory scale
						inventoryScale := decimal.One
						if (*flagTarget).IsPositive() {
							invRatio := inventoryValue.Div(*flagTarget)
							inventoryScale = decimal.One.Add(invRatio) // 1 + inv/target
						}

						// Calculate decay factor
						decayFactor := decimal.One
						timeSinceSell := clocky.Duration(0)
						if (*flagTarget).IsPositive() && !lastSellTime.IsZero() {
							invRatio := inventoryValue.Div(*flagTarget)
							timeSinceSell = now.Sub(lastSellTime)
							if timeSinceSell > 0 {
								timeRatio := decimal.FromInt(int(timeSinceSell)).Div(decimal.FromInt(int(*flagBuyDecay)))
								periodScale := invRatio.MulInt(2).Neg().Exp()                // e^(-invRatio*2)
								decayFactor = timeRatio.Mul(periodScale).Neg().Exp()         // e^(-timeRatio*periodScale)
								decayFactor = decayFactor.Max(decimal.Zero).Min(decimal.One) // clamp [0,1]
							}
						}

						// Effective buygap
						effectiveBuygap := (*flagBuyGap).Mul(inventoryScale).Mul(decayFactor)

						// Check if blocked by gap
						blocked := false
						actualGap := decimal.Zero
						if !topCost.IsZero() && effectiveBuygap.BPS().Cmp(decimal.Tenth) > 0 {
							maxBuyPrice := topCost.Mul(decimal.One.Sub(effectiveBuygap))
							actualGap = topCost.Sub(buyPrice).Div(topCost)
							if buyPrice.Cmp(maxBuyPrice) > 0 {
								blocked = true
							}
						}

						buyEvents = append(buyEvents, BuyEvent{
							Time:           now,
							Price:          buyPrice,
							Deviation:      deviation,
							TopCost:        topCost,
							InventoryValue: inventoryValue,
							InventoryScale: inventoryScale,
							DecayFactor:    decayFactor,
							EffectiveGap:   effectiveBuygap,
							ActualGap:      actualGap,
							Blocked:        blocked,
							TimeSinceSell:  timeSinceSell,
						})

						// Execute buy if not blocked
						if !blocked {
							buyQty := decimal.FromInt(100).Div(buyPrice) // $100 per buy
							inventoryQty = inventoryQty.Add(buyQty)
							topCost = buyPrice // Update top of stack
						}
					} else if deviation.Cmp(spreadThreshold) > 0 && inventoryQty.IsPositive() {
						// Sell signal - execute if we have inventory
						sellQty := decimal.FromInt(100).Div(coinbasePrice)
						if sellQty.Cmp(inventoryQty) > 0 {
							sellQty = inventoryQty
						}
						inventoryQty = inventoryQty.Sub(sellQty)
						lastSellTime = now

						// If sold everything, reset top cost
						if inventoryQty.IsZero() {
							topCost = decimal.Zero
						}
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
	printResults(*flagSymbol, *flagDataset, buyEvents, *flagBuyGap, *flagBuyDecay)
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

func printResults(symbol, dataset string, events []BuyEvent, buygap decimal.Decimal, decay clocky.Duration) {
	fmt.Println()
	fmt.Printf("# Buy Gap Analysis: %s/%s (gap=%sbps decay=%s)\n",
		dataset, symbol, buygap.BPS().Format(1), decay)
	fmt.Println()

	totalSignals := len(events)
	if totalSignals == 0 {
		fmt.Println("No buy signals found")
		return
	}

	// Count blocked vs executed
	blocked := 0
	executed := 0
	for _, e := range events {
		if e.Blocked {
			blocked++
		} else {
			executed++
		}
	}

	fmt.Println("## Signal Overview")
	fmt.Printf("Total Buy Signals:  %d\n", totalSignals)
	fmt.Printf("Executed:           %d (%.1f%%)\n", executed, float64(executed)/float64(totalSignals)*100)
	fmt.Printf("Blocked by Gap:     %d (%.1f%%)\n", blocked, float64(blocked)/float64(totalSignals)*100)
	fmt.Println()

	// Analyze gap distribution for blocked signals
	if blocked > 0 {
		fmt.Println("## Blocked Signals Analysis")
		var blockedGaps []float64
		var blockedEffectiveGaps []float64
		for _, e := range events {
			if e.Blocked {
				blockedGaps = append(blockedGaps, e.ActualGap.BPS().Float64())
				blockedEffectiveGaps = append(blockedEffectiveGaps, e.EffectiveGap.BPS().Float64())
			}
		}

		sort.Float64s(blockedGaps)
		sort.Float64s(blockedEffectiveGaps)

		fmt.Println("### Actual Gap (how far below last buy)")
		printDistribution(blockedGaps, "bps")

		fmt.Println()
		fmt.Println("### Effective Gap Threshold")
		printDistribution(blockedEffectiveGaps, "bps")
		fmt.Println()
	}

	// Analyze decay factor distribution
	var decayFactors []float64
	var inventoryScales []float64
	for _, e := range events {
		if !e.DecayFactor.IsZero() {
			decayFactors = append(decayFactors, e.DecayFactor.Float64())
		}
		if !e.InventoryScale.IsZero() && e.InventoryScale.Cmp(decimal.One) != 0 {
			inventoryScales = append(inventoryScales, e.InventoryScale.Float64())
		}
	}

	if len(decayFactors) > 0 {
		fmt.Println("## Decay Factor Distribution")
		sort.Float64s(decayFactors)
		printDistribution(decayFactors, "")
		fmt.Println()
	}

	if len(inventoryScales) > 0 {
		fmt.Println("## Inventory Scale Distribution")
		sort.Float64s(inventoryScales)
		printDistribution(inventoryScales, "x")
		fmt.Println()
	}

	// Time since sell analysis
	var timeSinceSells []float64
	for _, e := range events {
		if e.TimeSinceSell > 0 {
			timeSinceSells = append(timeSinceSells, float64(e.TimeSinceSell))
		}
	}

	if len(timeSinceSells) > 0 {
		fmt.Println("## Time Since Last Sell")
		sort.Float64s(timeSinceSells)
		// Convert to seconds for display
		n := len(timeSinceSells)
		fmt.Printf("Samples: %d\n", n)
		fmt.Printf("Min:     %.1fs\n", timeSinceSells[0]/1e9)
		fmt.Printf("P50:     %.1fs\n", timeSinceSells[n/2]/1e9)
		fmt.Printf("P95:     %.1fs\n", timeSinceSells[n*95/100]/1e9)
		fmt.Printf("Max:     %.1fs\n", timeSinceSells[n-1]/1e9)
		fmt.Println()
	}

	// Gap effectiveness zones
	fmt.Println("## Gap Protection Effectiveness")
	type zoneStats struct {
		total   int
		blocked int
	}
	zones := map[string]*zoneStats{
		"early": {}, // inventory < 25% target
		"mid":   {}, // 25-75%
		"late":  {}, // 75-100%
		"over":  {}, // > 100%
	}

	for _, e := range events {
		var zone string
		invPct := e.InventoryValue.Div(decimal.Parse("5000")).Float64() * 100
		switch {
		case invPct < 25:
			zone = "early"
		case invPct < 75:
			zone = "mid"
		case invPct < 100:
			zone = "late"
		default:
			zone = "over"
		}

		z := zones[zone]
		z.total++
		if e.Blocked {
			z.blocked++
		}
	}

	fmt.Printf("%-12s  %8s  %8s  %10s\n", "Inv Zone", "Signals", "Blocked", "Block Rate")
	for _, name := range []string{"early", "mid", "late", "over"} {
		z := zones[name]
		if z.total > 0 {
			blockRate := float64(z.blocked) / float64(z.total) * 100
			label := map[string]string{
				"early": "Early (<25%)",
				"mid":   "Mid (25-75%)",
				"late":  "Late (75-100%)",
				"over":  "Over (>100%)",
			}[name]
			fmt.Printf("%-12s  %8d  %8d  %9.1f%%\n", label, z.total, z.blocked, blockRate)
		}
	}
}

func printDistribution(values []float64, unit string) {
	if len(values) == 0 {
		return
	}

	n := len(values)
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(n)

	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	stddev := math.Sqrt(variance / float64(n))

	fmt.Printf("Samples: %d\n", n)
	fmt.Printf("Mean:    %.2f%s\n", mean, unit)
	fmt.Printf("StdDev:  %.2f%s\n", stddev, unit)
	fmt.Printf("Min:     %.2f%s\n", values[0], unit)
	fmt.Printf("P5:      %.2f%s\n", values[n*5/100], unit)
	fmt.Printf("P50:     %.2f%s\n", values[n/2], unit)
	fmt.Printf("P95:     %.2f%s\n", values[n*95/100], unit)
	fmt.Printf("Max:     %.2f%s\n", values[n-1], unit)
}
