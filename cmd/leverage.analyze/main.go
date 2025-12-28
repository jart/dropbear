// Command leverage.analyze analyzes leveraged ETF data for margin arbitrage opportunities.
//
// Computes:
//   - Daily volatility and annualized volatility
//   - Leveraged ETF decay vs underlying
//   - Liquidity metrics (volume by time of day)
//   - Correlation between products
//
// Usage:
//
//	go run ./cmd/leverage.analyze
package main

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/indicators"
	"dropbear/loggy"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

// Symbol categories
var (
	// Tier 1: 3x MicroSectors ETNs with 30% margin (should be 75%)
	tier1 = []string{"FNGU", "BNKU", "NRGU", "BNKD", "NRGD"}

	// Tier 2: 2x Single-Stock ETFs with 30% margin
	tier2 = []string{"NVDB", "PLTA", "TSLI", "QQQU"}

	// Tier 3: Other 2x products with 30% margin
	tier3 = []string{"DAMD", "CONX", "ORCU", "ARMU"}

	// Control: Properly margined 3x ETFs (75% margin)
	control = []string{"TQQQ", "SOXL", "TECL", "SPXL"}

	// Underlying for comparison
	underlying = []string{"QQQ", "SPY", "NVDA", "AAPL", "TSLA", "PLTR", "AMD", "GOOGL"}

	// Pairs for decay analysis: leveraged -> underlying
	decayPairs = map[string]string{
		"FNGU": "QQQ",  // FANG+ vs QQQ (tech proxy)
		"TQQQ": "QQQ",  // 3x QQQ vs QQQ
		"NVDB": "NVDA", // 2x NVDA vs NVDA
		"PLTA": "PLTR", // 2x PLTR vs PLTR
		"TSLI": "TSLA", // 2x TSLA vs TSLA
		"SOXL": "QQQ",  // 3x semis vs QQQ (tech proxy)
	}

	// Leverage multipliers
	leverage = map[string]int{
		"FNGU": 3, "BNKU": 3, "NRGU": 3, "BNKD": -3, "NRGD": -3,
		"TQQQ": 3, "SOXL": 3, "TECL": 3, "SPXL": 3,
		"NVDB": 2, "PLTA": 2, "TSLI": 2, "QQQU": 2,
		"DAMD": -2, "CONX": 2, "ORCU": 2, "ARMU": 2,
	}

	// Margin requirements
	margin = map[string]int{
		"FNGU": 30, "BNKU": 30, "NRGU": 30, "BNKD": 30, "NRGD": 30,
		"TQQQ": 75, "SOXL": 75, "TECL": 75, "SPXL": 75,
		"NVDB": 30, "PLTA": 30, "TSLI": 30, "QQQU": 30,
		"DAMD": 30, "CONX": 30, "ORCU": 30, "ARMU": 30,
	}
)

type symbolData struct {
	symbol  string
	candles []indicators.Candle
	days    []dayData
}

type dayData struct {
	date       time.Time
	open       decimal.Decimal
	high       decimal.Decimal
	low        decimal.Decimal
	close      decimal.Decimal
	volume     decimal.Decimal
	candles    int
	volatility float64 // intraday volatility (high-low)/open
}

func main() {
	loggy.Init()

	fmt.Println("========================================")
	fmt.Println("Leveraged ETF Margin Arbitrage Analysis")
	fmt.Println("========================================")
	fmt.Println()

	// Load all data
	allSymbols := append(append(append(append(tier1, tier2...), tier3...), control...), underlying...)
	data := make(map[string]*symbolData)

	fmt.Println("Loading data...")
	for _, sym := range allSymbols {
		sd := loadSymbol(sym)
		if sd != nil {
			data[sym] = sd
			fmt.Printf("  %s: %d candles, %d days\n", sym, len(sd.candles), len(sd.days))
		} else {
			fmt.Printf("  %s: NO DATA\n", sym)
		}
	}
	fmt.Println()

	// 1. Volatility Analysis
	fmt.Println("========================================")
	fmt.Println("1. VOLATILITY ANALYSIS")
	fmt.Println("========================================")
	fmt.Println()
	analyzeVolatility(data)

	// 2. Decay Analysis
	fmt.Println("========================================")
	fmt.Println("2. DECAY ANALYSIS (vs Underlying)")
	fmt.Println("========================================")
	fmt.Println()
	analyzeDecay(data)

	// 3. Liquidity Analysis
	fmt.Println("========================================")
	fmt.Println("3. LIQUIDITY ANALYSIS")
	fmt.Println("========================================")
	fmt.Println()
	analyzeLiquidity(data)

	// 4. Effective Leverage Comparison
	fmt.Println("========================================")
	fmt.Println("4. EFFECTIVE LEVERAGE COMPARISON")
	fmt.Println("========================================")
	fmt.Println()
	analyzeLeverage(data)

	// 5. Day Trading Metrics
	fmt.Println("========================================")
	fmt.Println("5. DAY TRADING OPPORTUNITY SCORE")
	fmt.Println("========================================")
	fmt.Println()
	analyzeDayTrading(data)
}

func loadSymbol(symbol string) *symbolData {
	base := ds.EquityMinutesDir()
	symbolDir := filepath.Join(base, symbol)

	entries, err := os.ReadDir(symbolDir)
	if err != nil {
		return nil
	}

	var candles []indicators.Candle

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(symbolDir, entry.Name())
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		reader, err := zstd.NewReader(file)
		if err != nil {
			file.Close()
			continue
		}

		for {
			var c indicators.Candle
			err := c.Deserialize(reader)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			if err != nil {
				break
			}
			candles = append(candles, c)
		}
		reader.Close()
		file.Close()
	}

	if len(candles) == 0 {
		return nil
	}

	// Sort by time
	sort.Slice(candles, func(i, j int) bool {
		return candles[i].Start < candles[j].Start
	})

	// Aggregate into days
	days := aggregateDays(candles)

	return &symbolData{
		symbol:  symbol,
		candles: candles,
		days:    days,
	}
}

func aggregateDays(candles []indicators.Candle) []dayData {
	if len(candles) == 0 {
		return nil
	}

	loc, _ := time.LoadLocation("America/New_York")
	var days []dayData
	var currentDay *dayData

	for _, c := range candles {
		t := time.UnixMicro(int64(c.Start)).In(loc)
		date := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)

		if currentDay == nil || !currentDay.date.Equal(date) {
			if currentDay != nil {
				// Calculate intraday volatility
				if !currentDay.open.IsZero() {
					range_ := currentDay.high.Sub(currentDay.low)
					currentDay.volatility = range_.Div(currentDay.open).Float64()
				}
				days = append(days, *currentDay)
			}
			currentDay = &dayData{
				date:   date,
				open:   c.Open,
				high:   c.High,
				low:    c.Low,
				close:  c.Close,
				volume: c.Volume,
			}
		} else {
			if c.High.Cmp(currentDay.high) > 0 {
				currentDay.high = c.High
			}
			if c.Low.Cmp(currentDay.low) < 0 {
				currentDay.low = c.Low
			}
			currentDay.close = c.Close
			currentDay.volume = currentDay.volume.Add(c.Volume)
		}
		currentDay.candles++
	}

	if currentDay != nil {
		if !currentDay.open.IsZero() {
			range_ := currentDay.high.Sub(currentDay.low)
			currentDay.volatility = range_.Div(currentDay.open).Float64()
		}
		days = append(days, *currentDay)
	}

	return days
}

func analyzeVolatility(data map[string]*symbolData) {
	type volResult struct {
		symbol       string
		avgDaily     float64 // average daily (high-low)/open
		annualized   float64 // annualized from daily returns
		maxDaily     float64 // max single day range
		avgDailyMove float64 // avg |close-open|/open
		days         int
	}

	var results []volResult

	for sym, sd := range data {
		if len(sd.days) < 10 {
			continue
		}

		var sumVol, sumMove, maxVol float64
		var returns []float64

		for i, d := range sd.days {
			sumVol += d.volatility
			if d.volatility > maxVol {
				maxVol = d.volatility
			}

			if !d.open.IsZero() {
				move := math.Abs(d.close.Sub(d.open).Div(d.open).Float64())
				sumMove += move
			}

			if i > 0 && !sd.days[i-1].close.IsZero() {
				ret := d.close.Sub(sd.days[i-1].close).Div(sd.days[i-1].close).Float64()
				returns = append(returns, ret)
			}
		}

		avgDaily := sumVol / float64(len(sd.days))
		avgMove := sumMove / float64(len(sd.days))

		// Calculate annualized volatility from daily returns
		var annualized float64
		if len(returns) > 1 {
			var sumSq float64
			mean := 0.0
			for _, r := range returns {
				mean += r
			}
			mean /= float64(len(returns))
			for _, r := range returns {
				sumSq += (r - mean) * (r - mean)
			}
			dailyStd := math.Sqrt(sumSq / float64(len(returns)-1))
			annualized = dailyStd * math.Sqrt(252) * 100 // percentage
		}

		results = append(results, volResult{
			symbol:       sym,
			avgDaily:     avgDaily * 100,
			annualized:   annualized,
			maxDaily:     maxVol * 100,
			avgDailyMove: avgMove * 100,
			days:         len(sd.days),
		})
	}

	// Sort by annualized volatility descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].annualized > results[j].annualized
	})

	fmt.Printf("%-8s %6s %8s %8s %8s %8s %5s\n",
		"Symbol", "Lev", "AnnVol%", "AvgRng%", "MaxRng%", "AvgMov%", "Days")
	fmt.Println(strings.Repeat("-", 60))

	for _, r := range results {
		lev := leverage[r.symbol]
		levStr := ""
		if lev != 0 {
			levStr = fmt.Sprintf("%dx", lev)
		} else {
			levStr = "1x"
		}
		fmt.Printf("%-8s %6s %8.1f %8.2f %8.2f %8.2f %5d\n",
			r.symbol, levStr, r.annualized, r.avgDaily, r.maxDaily, r.avgDailyMove, r.days)
	}
	fmt.Println()
}

func analyzeDecay(data map[string]*symbolData) {
	fmt.Printf("Decay = how much the leveraged ETF underperforms (leverage × underlying return)\n")
	fmt.Printf("Negative decay means it underperformed; positive means it outperformed\n\n")

	fmt.Printf("%-8s %-8s %6s %10s %10s %10s %8s\n",
		"Lev.ETF", "Under.", "Lev", "ETF Ret%", "Und×Lev%", "Decay%", "Days")
	fmt.Println(strings.Repeat("-", 70))

	for levSym, undSym := range decayPairs {
		levData := data[levSym]
		undData := data[undSym]

		if levData == nil || undData == nil {
			continue
		}

		// Find overlapping date range
		levDays := make(map[string]dayData)
		for _, d := range levData.days {
			levDays[d.date.Format("2006-01-02")] = d
		}

		var levRets, undRets []float64
		var prevLev, prevUnd dayData
		first := true

		for _, ud := range undData.days {
			dateStr := ud.date.Format("2006-01-02")
			ld, ok := levDays[dateStr]
			if !ok {
				continue
			}

			if !first && !prevLev.close.IsZero() && !prevUnd.close.IsZero() {
				levRet := ld.close.Sub(prevLev.close).Div(prevLev.close).Float64()
				undRet := ud.close.Sub(prevUnd.close).Div(prevUnd.close).Float64()
				levRets = append(levRets, levRet)
				undRets = append(undRets, undRet)
			}
			prevLev = ld
			prevUnd = ud
			first = false
		}

		if len(levRets) < 10 {
			continue
		}

		// Calculate cumulative returns
		levCum := 1.0
		undCum := 1.0
		lev := float64(abs(leverage[levSym]))

		for i := range levRets {
			levCum *= (1 + levRets[i])
			undCum *= (1 + undRets[i])
		}

		levReturn := (levCum - 1) * 100
		expectedReturn := (math.Pow(undCum, lev) - 1) * 100
		decay := levReturn - expectedReturn

		fmt.Printf("%-8s %-8s %6dx %10.2f %10.2f %10.2f %8d\n",
			levSym, undSym, int(lev), levReturn, expectedReturn, decay, len(levRets))
	}
	fmt.Println()
}

func analyzeLiquidity(data map[string]*symbolData) {
	type liqResult struct {
		symbol     string
		avgVolume  float64
		totalDays  int
		avgCandles int // candles per day (proxy for trading activity)
	}

	var results []liqResult

	for sym, sd := range data {
		if len(sd.days) < 10 {
			continue
		}

		var sumVol float64
		var sumCandles int
		for _, d := range sd.days {
			sumVol += d.volume.Float64()
			sumCandles += d.candles
		}

		results = append(results, liqResult{
			symbol:     sym,
			avgVolume:  sumVol / float64(len(sd.days)),
			totalDays:  len(sd.days),
			avgCandles: sumCandles / len(sd.days),
		})
	}

	// Sort by average volume descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].avgVolume > results[j].avgVolume
	})

	fmt.Printf("%-8s %15s %8s %8s\n",
		"Symbol", "Avg Daily Vol", "Days", "Candles")
	fmt.Println(strings.Repeat("-", 45))

	for _, r := range results {
		fmt.Printf("%-8s %15s %8d %8d\n",
			r.symbol, formatVolume(r.avgVolume), r.totalDays, r.avgCandles)
	}
	fmt.Println()
}

func analyzeLeverage(data map[string]*symbolData) {
	fmt.Printf("Effective leverage calculation:\n")
	fmt.Printf("  Day Trade: 4x buying power / MM%% × product leverage\n")
	fmt.Printf("  Overnight: 2x buying power / MM%% × product leverage\n\n")

	type levResult struct {
		symbol      string
		leverage    int
		margin      int
		effDay      float64
		effOvernight float64
		annVol      float64
	}

	var results []levResult

	for sym, sd := range data {
		lev := leverage[sym]
		mar := margin[sym]
		if lev == 0 || mar == 0 {
			continue
		}

		// Calculate annualized volatility
		var returns []float64
		for i := 1; i < len(sd.days); i++ {
			if !sd.days[i-1].close.IsZero() {
				ret := sd.days[i].close.Sub(sd.days[i-1].close).Div(sd.days[i-1].close).Float64()
				returns = append(returns, ret)
			}
		}

		var annVol float64
		if len(returns) > 1 {
			var sumSq, mean float64
			for _, r := range returns {
				mean += r
			}
			mean /= float64(len(returns))
			for _, r := range returns {
				sumSq += (r - mean) * (r - mean)
			}
			dailyStd := math.Sqrt(sumSq / float64(len(returns)-1))
			annVol = dailyStd * math.Sqrt(252) * 100
		}

		absLev := float64(abs(lev))
		effDay := (4.0 / (float64(mar) / 100.0)) * absLev
		effOvn := (2.0 / (float64(mar) / 100.0)) * absLev

		results = append(results, levResult{
			symbol:       sym,
			leverage:     lev,
			margin:       mar,
			effDay:       effDay,
			effOvernight: effOvn,
			annVol:       annVol,
		})
	}

	// Sort by effective day leverage descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].effDay > results[j].effDay
	})

	fmt.Printf("%-8s %6s %6s %10s %10s %8s\n",
		"Symbol", "Lev", "MM%", "EffDay", "EffOvn", "AnnVol%")
	fmt.Println(strings.Repeat("-", 55))

	for _, r := range results {
		fmt.Printf("%-8s %6dx %6d%% %10.1fx %10.1fx %8.1f%%\n",
			r.symbol, r.leverage, r.margin, r.effDay, r.effOvernight, r.annVol)
	}
	fmt.Println()
}

func analyzeDayTrading(data map[string]*symbolData) {
	fmt.Printf("Day Trading Score = (Effective Leverage × Avg Daily Range) / 100\n")
	fmt.Printf("Higher score = more profit potential per dollar of capital\n\n")

	type dtResult struct {
		symbol   string
		effLev   float64
		avgRange float64
		score    float64
		volume   float64
	}

	var results []dtResult

	for sym, sd := range data {
		lev := leverage[sym]
		mar := margin[sym]
		if lev == 0 || mar == 0 {
			continue
		}
		if len(sd.days) < 20 {
			continue
		}

		absLev := float64(abs(lev))
		effDay := (4.0 / (float64(mar) / 100.0)) * absLev

		var sumRange, sumVol float64
		for _, d := range sd.days {
			sumRange += d.volatility
			sumVol += d.volume.Float64()
		}
		avgRange := (sumRange / float64(len(sd.days))) * 100
		avgVol := sumVol / float64(len(sd.days))

		// Score: effective leverage × average intraday range
		score := effDay * avgRange / 100

		results = append(results, dtResult{
			symbol:   sym,
			effLev:   effDay,
			avgRange: avgRange,
			score:    score,
			volume:   avgVol,
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	fmt.Printf("%-8s %8s %8s %8s %15s  %s\n",
		"Symbol", "EffLev", "AvgRng%", "Score", "AvgVolume", "Assessment")
	fmt.Println(strings.Repeat("-", 70))

	for _, r := range results {
		assessment := assessDayTrade(r.score, r.volume)
		fmt.Printf("%-8s %8.1fx %8.2f%% %8.2f %15s  %s\n",
			r.symbol, r.effLev, r.avgRange, r.score, formatVolume(r.volume), assessment)
	}
	fmt.Println()

	fmt.Println("RECOMMENDATIONS:")
	fmt.Println("----------------")
	if len(results) > 0 {
		fmt.Printf("1. Top pick: %s (Score: %.2f, %s effective day leverage)\n",
			results[0].symbol, results[0].score, formatFloat(results[0].effLev))
	}
	if len(results) > 1 {
		fmt.Printf("2. Runner up: %s (Score: %.2f)\n", results[1].symbol, results[1].score)
	}
	if len(results) > 2 {
		fmt.Printf("3. Third: %s (Score: %.2f)\n", results[2].symbol, results[2].score)
	}
	fmt.Println()
}

func assessDayTrade(score, volume float64) string {
	if volume < 50000 {
		return "LOW LIQUIDITY"
	}
	if score > 1.5 {
		return "EXCELLENT"
	}
	if score > 1.0 {
		return "GOOD"
	}
	if score > 0.5 {
		return "MODERATE"
	}
	return "LOW"
}

func formatVolume(vol float64) string {
	if vol >= 1e9 {
		return fmt.Sprintf("%.1fB", vol/1e9)
	}
	if vol >= 1e6 {
		return fmt.Sprintf("%.1fM", vol/1e6)
	}
	if vol >= 1e3 {
		return fmt.Sprintf("%.1fK", vol/1e3)
	}
	return fmt.Sprintf("%.0f", vol)
}

func formatFloat(f float64) string {
	return fmt.Sprintf("%.1fx", f)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
