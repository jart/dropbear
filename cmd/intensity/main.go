// cmd/intensity analyzes Avellaneda-Stoikov kappa (trading intensity) parameter.
// Usage: go run ./cmd/intensity -dataset chaos -symbol BTC
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
	flagDataset   = flag.String("dataset", "chaos", "dataset name")
	flagSymbol    = flag.String("symbol", "BTC", "coinbase product to analyze")
	flagSamples   = flag.Int("samples", 7000, "number of samples for baseline ema")
	flagSpread    = decimal.Flag("spread", "2", "spread threshold in basis points")
	flagHalfLife  = clocky.DurationFlag("halflife", "5m", "intensity decay half-life")
	flagRefKappa  = flag.Float64("refkappa", 20000, "reference kappa for scaling")
	flagLookahead = clocky.DurationFlag("lookahead", "5m", "time to look ahead for outcome")
)

// Signal represents a trading signal with kappa context
type Signal struct {
	Time      clocky.Time
	Side      ds.Side
	Price     decimal.Decimal
	Deviation decimal.Decimal
	Kappa     float64
	Scale     float64
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
	intensity := indicators.NewIntensity(*flagHalfLife)

	// Initialize state
	coinbaseBook := ds.NewBook()
	var binancePrice decimal.Decimal
	var coinbasePrice decimal.Decimal
	warmedUp := false

	// Track kappa values and signals
	var kappaValues []float64
	var signals []Signal

	// Price history for lookahead
	type pricePoint struct {
		time  clocky.Time
		price decimal.Decimal
	}
	var priceHistory []pricePoint

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

			// Update price and intensity from trades
			for _, trade := range coinbaseTick.Trades {
				if trade.Price.IsPositive() {
					coinbasePrice = trade.Price
					priceHistory = append(priceHistory, pricePoint{now, coinbasePrice})

					// Update intensity indicator
					mid := coinbaseBook.MidPrice()
					if mid.IsPositive() {
						intensity.SetMidPrice(mid)
						intensity.AddTrade(now, trade.Price, trade.Quantity)
					}
				}
			}

			// Check for signals
			if binancePrice.IsPositive() && coinbasePrice.IsPositive() {
				spread := coinbasePrice.Sub(binancePrice).Div(binancePrice)
				spreadEMA.Add(spread)

				if spreadEMA.IsReady() && intensity.IsReady() {
					if !warmedUp {
						warmedUp = true
					}

					baseline := spreadEMA.Value
					deviation := spread.Sub(baseline)

					// Track kappa values
					kappa := intensity.Kappa.Float64()
					if kappa > 0 {
						kappaValues = append(kappaValues, kappa)
					}

					// Compute intensity scale
					scale := 1.0
					if kappa > 0 {
						scale = *flagRefKappa / kappa
						if scale < 0.5 {
							scale = 0.5
						}
						if scale > 2.0 {
							scale = 2.0
						}
					}

					// Adjust spread by intensity
					adjustedThreshold := spreadThreshold.Mul(decimal.FromFloat64(scale))

					// Check for signals with intensity-adjusted spreads
					if deviation.Neg().Cmp(adjustedThreshold) > 0 {
						signals = append(signals, Signal{
							Time:      now,
							Side:      ds.SideBuy,
							Price:     coinbasePrice,
							Deviation: deviation,
							Kappa:     kappa,
							Scale:     scale,
						})
					} else if deviation.Cmp(adjustedThreshold) > 0 {
						signals = append(signals, Signal{
							Time:      now,
							Side:      ds.SideSell,
							Price:     coinbasePrice,
							Deviation: deviation,
							Kappa:     kappa,
							Scale:     scale,
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

	// Compute outcomes for signals
	var outcomes []Outcome
	for _, sig := range signals {
		futureTime := sig.Time.Add(*flagLookahead)
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
			outcomes = append(outcomes, Outcome{sig, futurePrice, profitBPS})
		}
	}

	// Print results
	printResults(*flagSymbol, *flagDataset, kappaValues, outcomes)
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

func printResults(symbol, dataset string, kappaValues []float64, outcomes []Outcome) {
	fmt.Println()
	fmt.Printf("# Intensity (Kappa) Analysis: %s/%s\n", dataset, symbol)
	fmt.Println()

	// Kappa distribution
	if len(kappaValues) > 0 {
		sort.Float64s(kappaValues)
		n := len(kappaValues)
		sum := 0.0
		for _, k := range kappaValues {
			sum += k
		}
		mean := sum / float64(n)

		variance := 0.0
		for _, k := range kappaValues {
			diff := k - mean
			variance += diff * diff
		}
		stddev := math.Sqrt(variance / float64(n))

		fmt.Println("## Kappa Distribution")
		fmt.Printf("Samples: %d\n", n)
		fmt.Printf("Mean:    %.0f\n", mean)
		fmt.Printf("StdDev:  %.0f\n", stddev)
		fmt.Printf("Min:     %.0f\n", kappaValues[0])
		fmt.Printf("Max:     %.0f\n", kappaValues[n-1])
		fmt.Printf("P5:      %.0f\n", kappaValues[n*5/100])
		fmt.Printf("P50:     %.0f\n", kappaValues[n/2])
		fmt.Printf("P95:     %.0f\n", kappaValues[n*95/100])
		fmt.Println()

		// Kappa zones
		fmt.Println("Time in Zone:")
		veryTight := 0  // > 50000
		tight := 0      // 20000-50000
		normal := 0     // 10000-20000
		loose := 0      // 5000-10000
		veryLoose := 0  // < 5000

		for _, k := range kappaValues {
			switch {
			case k > 50000:
				veryTight++
			case k > 20000:
				tight++
			case k > 10000:
				normal++
			case k > 5000:
				loose++
			default:
				veryLoose++
			}
		}

		fmt.Printf("  Very Tight (>50k):   %5.1f%%\n", float64(veryTight)/float64(n)*100)
		fmt.Printf("  Tight (20k-50k):     %5.1f%%\n", float64(tight)/float64(n)*100)
		fmt.Printf("  Normal (10k-20k):    %5.1f%%\n", float64(normal)/float64(n)*100)
		fmt.Printf("  Loose (5k-10k):      %5.1f%%\n", float64(loose)/float64(n)*100)
		fmt.Printf("  Very Loose (<5k):    %5.1f%%\n", float64(veryLoose)/float64(n)*100)
		fmt.Println()
	}

	// Outcome by kappa zone
	if len(outcomes) > 0 {
		fmt.Println("## Signal Outcomes by Kappa Zone")

		type stats struct {
			count      int
			sumProfit  float64
			profitable int
		}

		kappaZones := map[string]*stats{
			"very_tight": {},
			"tight":      {},
			"normal":     {},
			"loose":      {},
			"very_loose": {},
		}

		for _, o := range outcomes {
			var zone string
			switch {
			case o.Signal.Kappa > 50000:
				zone = "very_tight"
			case o.Signal.Kappa > 20000:
				zone = "tight"
			case o.Signal.Kappa > 10000:
				zone = "normal"
			case o.Signal.Kappa > 5000:
				zone = "loose"
			default:
				zone = "very_loose"
			}

			s := kappaZones[zone]
			s.count++
			s.sumProfit += o.ProfitBPS
			if o.ProfitBPS > 0 {
				s.profitable++
			}
		}

		fmt.Printf("%-18s  %6s  %10s  %8s\n", "Kappa Zone", "Count", "Avg Profit", "Win Rate")
		for _, name := range []string{"very_tight", "tight", "normal", "loose", "very_loose"} {
			s := kappaZones[name]
			if s.count > 0 {
				avgProfit := s.sumProfit / float64(s.count)
				winRate := float64(s.profitable) / float64(s.count) * 100
				label := map[string]string{
					"very_tight": "Very Tight (>50k)",
					"tight":      "Tight (20k-50k)",
					"normal":     "Normal (10k-20k)",
					"loose":      "Loose (5k-10k)",
					"very_loose": "Very Loose (<5k)",
				}[name]
				fmt.Printf("%-18s  %6d  %+10.2f  %7.1f%%\n", label, s.count, avgProfit, winRate)
			}
		}
		fmt.Println()

		// Overall stats
		fmt.Println("## Summary")
		total := len(outcomes)
		var sumProfit float64
		var profitable int
		for _, o := range outcomes {
			sumProfit += o.ProfitBPS
			if o.ProfitBPS > 0 {
				profitable++
			}
		}
		fmt.Printf("Total Signals: %d\n", total)
		fmt.Printf("Avg Profit:    %+.2fbps\n", sumProfit/float64(total))
		fmt.Printf("Win Rate:      %.1f%%\n", float64(profitable)/float64(total)*100)
	}
}
