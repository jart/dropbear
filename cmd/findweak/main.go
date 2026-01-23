package main

import (
	"cmp"
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/netty"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

func init() {
	netty.SetOffline()
}

var (
	days      = flag.Int("days", 200, "number of trading days to analyze")
	volTarget = flag.Float64("vol-target", 100e6, "target daily notional volume (center of bell curve)")
	volSigma  = flag.Float64("vol-sigma", 1.5, "log-scale sigma for volume bell curve (higher = wider)")
)

func main() {
	flag.Parse()

	// Get healthy symbols
	var symbols []string
	for _, a := range alpaca.Assets {
		if a.IsHealthy() {
			symbols = append(symbols, a.Symbol.String())
		}
	}

	// Calculate cutoff time (approximate, using calendar days)
	cutoff := clocky.Now() - clocky.Time(clocky.Duration(*days)*clocky.Day*7/5)

	minutesDir := ds.EquityMinutesDir()

	// Load SPY bars for relative strength calculation
	spyBars, err := loadDailyBars(filepath.Join(minutesDir, "SPY"), cutoff)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load SPY bars: %v\n", err)
	}
	spyReturns := calcDailyReturns(spyBars)

	// Score each symbol
	type result struct {
		symbol       string
		monotonicity float64
		avgNotional  float64
		volScore     float64
		sharpe       float64
		sharpeScore  float64
		// New filter metrics
		distFromHigh  float64 // distance from 60-day high
		volSpikeRatio float64 // max 10d vol / avg 60d vol
		daysBelowMA   int     // days below 50-day MA
		bouncePct     float64 // bounce from 10-day low
		acceleration  float64 // momentum acceleration
		distTo52wLow  float64 // distance to 52-week low
		relStrength   float64 // vs SPY over 20 days
		// Final scores
		baseScore     float64
		adjustedScore float64
	}

	var results []result

	for _, sym := range symbols {
		path := filepath.Join(minutesDir, sym)
		dailyBars, err := loadDailyBars(path, cutoff)
		if err != nil {
			continue
		}

		if len(dailyBars) < 60 { // need enough data for all metrics
			continue
		}

		r := result{symbol: sym}

		// Existing metrics
		r.monotonicity = calcMonotonicity(dailyBars)
		r.avgNotional = calcAvgDailyNotional(dailyBars)
		r.volScore = calcVolumeSweetSpot(r.avgNotional, *volTarget, *volSigma)
		r.sharpe = calcDailySharpe(dailyBars)

		// New filter metrics
		r.distFromHigh = calcDistanceFromHigh(dailyBars, 60)
		r.volSpikeRatio = calcVolumeSpikeRatio(dailyBars, 10, 60)
		r.daysBelowMA = calcDaysBelowMA(dailyBars, 50)
		r.bouncePct = calcBounceFromLow(dailyBars, 10)
		r.acceleration = calcAcceleration(dailyBars)
		r.distTo52wLow = calcDistanceTo52wLow(dailyBars)
		r.relStrength = calcRelativeStrength(dailyBars, spyReturns, 20)

		results = append(results, r)
	}

	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "no symbols with sufficient data")
		os.Exit(1)
	}

	// Normalize sharpe to 0-1 using rank percentile
	slices.SortFunc(results, func(a, b result) int {
		return cmp.Compare(a.sharpe, b.sharpe)
	})
	for i := range results {
		results[i].sharpeScore = 1.0 - float64(i)/float64(len(results)-1)
	}

	// Calculate base and adjusted scores
	for i := range results {
		r := &results[i]

		// Base score (33/33/33 blend)
		r.baseScore = (r.monotonicity + r.volScore + r.sharpeScore) / 3.0

		// Penalty factors
		crashPenalty := 1.0
		if r.distFromHigh > 0.20 {
			crashPenalty = 0.5 // already down >20%
		} else if r.distFromHigh > 0.15 {
			crashPenalty = 0.7
		}

		volSpikePenalty := 1.0
		if r.volSpikeRatio > 3.0 {
			volSpikePenalty = 0.3 // capitulation volume
		} else if r.volSpikeRatio > 2.5 {
			volSpikePenalty = 0.6
		}

		recoveryPenalty := 1.0
		if r.bouncePct > 0.08 {
			recoveryPenalty = 0.4 // bouncing off lows
		} else if r.bouncePct > 0.05 {
			recoveryPenalty = 0.7
		}

		freshnessPenalty := 1.0
		if r.daysBelowMA > 45 {
			freshnessPenalty = 0.6 // stale trend
		} else if r.daysBelowMA > 30 {
			freshnessPenalty = 0.8
		}

		lowProximityPenalty := 1.0
		if r.distTo52wLow < 0.05 {
			lowProximityPenalty = 0.5 // at 52-week low, squeeze risk
		} else if r.distTo52wLow < 0.10 {
			lowProximityPenalty = 0.7
		}

		// Bonus factors
		accelerationBonus := 1.0
		if r.acceleration < -0.02 {
			accelerationBonus = 1.3 // decline speeding up
		} else if r.acceleration < -0.01 {
			accelerationBonus = 1.15
		} else if r.acceleration > 0.01 {
			accelerationBonus = 0.7 // decline slowing
		}

		relWeaknessBonus := 1.0
		if r.relStrength < -0.05 {
			relWeaknessBonus = 1.2 // underperforming SPY
		} else if r.relStrength < -0.03 {
			relWeaknessBonus = 1.1
		} else if r.relStrength > 0 {
			relWeaknessBonus = 0.8 // outperforming SPY
		}

		r.adjustedScore = r.baseScore *
			crashPenalty *
			volSpikePenalty *
			recoveryPenalty *
			freshnessPenalty *
			lowProximityPenalty *
			accelerationBonus *
			relWeaknessBonus
	}

	// Sort by adjusted score descending
	slices.SortFunc(results, func(a, b result) int {
		if c := cmp.Compare(b.adjustedScore, a.adjustedScore); c != 0 {
			return c
		}
		return cmp.Compare(a.symbol, b.symbol)
	})

	// Print results
	fmt.Printf("%-6s %5s %5s %6s %5s %6s %5s %5s %5s %6s %15s\n",
		"SYM", "MONO", "VOL", "SHARPE", "BASE", "ADJST", "CRASH", "SPIKE", "BONCE", "ACCEL", "AVG_NOTIONAL")
	for _, r := range results {
		fmt.Printf("%-6s %5.2f %5.2f %6.3f %5.2f %6.3f %5.0f%% %5.1fx %5.0f%% %+5.1f%% %15.0f\n",
			r.symbol,
			r.monotonicity,
			r.volScore,
			r.sharpe,
			r.baseScore,
			r.adjustedScore,
			r.distFromHigh*100,
			r.volSpikeRatio,
			r.bouncePct*100,
			r.acceleration*100,
			r.avgNotional)
	}
}

// dailyBar holds aggregated daily data
type dailyBar struct {
	date     clocky.Time
	open     decimal.Decimal
	high     decimal.Decimal
	low      decimal.Decimal
	close    decimal.Decimal
	volume   decimal.Decimal
	notional decimal.Decimal
}

// loadDailyBars loads and aggregates minute bars to daily bars
func loadDailyBars(path string, cutoff clocky.Time) ([]dailyBar, error) {
	bars, err := ds.OpenBars(path)
	if err != nil {
		return nil, err
	}
	defer bars.Close()

	bars.Seek(cutoff)

	var daily []dailyBar
	var currentDay clocky.Time
	var dayBar dailyBar

	for !bars.EOF() {
		bar := bars.Read()
		day := bar.Timestamp - bar.Timestamp%clocky.Time(clocky.Day)

		if day != currentDay {
			if currentDay != 0 {
				daily = append(daily, dayBar)
			}
			currentDay = day
			dayBar = dailyBar{
				date:   day,
				open:   bar.Open,
				high:   bar.High,
				low:    bar.Low,
				volume: decimal.Zero,
			}
		}

		if bar.High.Cmp(dayBar.high) > 0 {
			dayBar.high = bar.High
		}
		if bar.Low.Cmp(dayBar.low) < 0 {
			dayBar.low = bar.Low
		}
		dayBar.close = bar.Close
		dayBar.volume = dayBar.volume.Add(bar.Volume)

		price := bar.VWAP
		if price.IsZero() {
			price = bar.Close
		}
		dayBar.notional = dayBar.notional.Add(bar.Volume.Mul(price))
	}

	if currentDay != 0 {
		daily = append(daily, dayBar)
	}

	return daily, nil
}

// calcDailyReturns calculates daily returns for relative strength
func calcDailyReturns(bars []dailyBar) []float64 {
	if len(bars) < 2 {
		return nil
	}
	returns := make([]float64, len(bars))
	returns[0] = 0
	for i := 1; i < len(bars); i++ {
		prev := bars[i-1].close.Float64()
		curr := bars[i].close.Float64()
		if prev > 0 {
			returns[i] = (curr - prev) / prev
		}
	}
	return returns
}

// calcMonotonicity calculates Spearman rank correlation of price vs time
func calcMonotonicity(bars []dailyBar) float64 {
	n := len(bars)
	if n < 2 {
		return 0.5
	}

	type indexed struct {
		idx   int
		close decimal.Decimal
	}
	closes := make([]indexed, n)
	for i, b := range bars {
		closes[i] = indexed{i, b.close}
	}

	sort.Slice(closes, func(i, j int) bool {
		return closes[i].close.Cmp(closes[j].close) < 0
	})

	priceRanks := make([]float64, n)
	for rank, c := range closes {
		priceRanks[c.idx] = float64(rank + 1)
	}

	var sumD2 float64
	for i := 0; i < n; i++ {
		d := priceRanks[i] - float64(i+1)
		sumD2 += d * d
	}

	spearman := 1.0 - (6.0*sumD2)/(float64(n)*float64(n*n-1))
	return (1.0 - spearman) / 2.0
}

// calcAvgDailyNotional calculates average daily notional volume
func calcAvgDailyNotional(bars []dailyBar) float64 {
	if len(bars) == 0 {
		return 0
	}
	var total decimal.Decimal
	for _, b := range bars {
		total = total.Add(b.notional)
	}
	return float64(total.Int64()) / float64(len(bars))
}

// calcVolumeSweetSpot scores volume using a log-normal bell curve
func calcVolumeSweetSpot(avgNotional, target, sigma float64) float64 {
	if avgNotional <= 0 || target <= 0 {
		return 0
	}
	logVol := math.Log(avgNotional)
	logTarget := math.Log(target)
	z := (logVol - logTarget) / sigma
	return math.Exp(-0.5 * z * z)
}

// calcDailySharpe calculates the daily Sharpe ratio
func calcDailySharpe(bars []dailyBar) float64 {
	if len(bars) < 2 {
		return 0
	}

	returns := make([]float64, len(bars)-1)
	for i := 1; i < len(bars); i++ {
		prev := bars[i-1].close.Float64()
		curr := bars[i].close.Float64()
		if prev > 0 {
			returns[i-1] = (curr - prev) / prev
		}
	}

	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))

	var sumSq float64
	for _, r := range returns {
		d := r - mean
		sumSq += d * d
	}
	std := math.Sqrt(sumSq / float64(len(returns)))

	if std < 1e-10 {
		return 0
	}
	return mean / std
}

// calcDistanceFromHigh calculates how far price is from N-day high
func calcDistanceFromHigh(bars []dailyBar, days int) float64 {
	if len(bars) < days {
		days = len(bars)
	}
	recent := bars[len(bars)-days:]

	var maxHigh decimal.Decimal
	for _, b := range recent {
		if b.high.Cmp(maxHigh) > 0 {
			maxHigh = b.high
		}
	}

	current := bars[len(bars)-1].close.Float64()
	high := maxHigh.Float64()
	if high <= 0 {
		return 0
	}
	return (high - current) / high
}

// calcVolumeSpikeRatio calculates max recent volume / avg volume
func calcVolumeSpikeRatio(bars []dailyBar, recentDays, avgDays int) float64 {
	if len(bars) < avgDays {
		return 1.0
	}

	// Average volume over avgDays
	avgBars := bars[len(bars)-avgDays:]
	var totalVol decimal.Decimal
	for _, b := range avgBars {
		totalVol = totalVol.Add(b.volume)
	}
	avgVol := totalVol.Float64() / float64(len(avgBars))

	// Max volume in recent days
	if recentDays > len(bars) {
		recentDays = len(bars)
	}
	recentBars := bars[len(bars)-recentDays:]
	var maxVol decimal.Decimal
	for _, b := range recentBars {
		if b.volume.Cmp(maxVol) > 0 {
			maxVol = b.volume
		}
	}

	if avgVol < 1 {
		return 1.0
	}
	return maxVol.Float64() / avgVol
}

// calcDaysBelowMA counts days where close < N-day moving average
func calcDaysBelowMA(bars []dailyBar, maPeriod int) int {
	if len(bars) < maPeriod {
		return 0
	}

	count := 0
	for i := maPeriod; i < len(bars); i++ {
		// Calculate MA at this point
		var sum decimal.Decimal
		for j := i - maPeriod; j < i; j++ {
			sum = sum.Add(bars[j].close)
		}
		ma := sum.Float64() / float64(maPeriod)
		if bars[i].close.Float64() < ma {
			count++
		}
	}
	return count
}

// calcBounceFromLow calculates bounce from N-day low
func calcBounceFromLow(bars []dailyBar, days int) float64 {
	if len(bars) < days {
		days = len(bars)
	}
	recent := bars[len(bars)-days:]

	minLow := recent[0].low
	for _, b := range recent {
		if b.low.Cmp(minLow) < 0 {
			minLow = b.low
		}
	}

	current := bars[len(bars)-1].close.Float64()
	low := minLow.Float64()
	if low <= 0 {
		return 0
	}
	return (current - low) / low
}

// calcAcceleration measures if decline is speeding up or slowing
func calcAcceleration(bars []dailyBar) float64 {
	if len(bars) < 20 {
		return 0
	}

	current := bars[len(bars)-1].close.Float64()
	close5d := bars[len(bars)-6].close.Float64()
	close20d := bars[len(bars)-21].close.Float64()

	if close5d <= 0 || close20d <= 0 {
		return 0
	}

	momentum5d := (current - close5d) / close5d
	momentum20d := (current - close20d) / close20d

	// Normalize 20d momentum to 5d scale
	return momentum5d - (momentum20d / 4)
}

// calcDistanceTo52wLow calculates distance from 52-week (252 trading day) low
func calcDistanceTo52wLow(bars []dailyBar) float64 {
	days := 252
	if len(bars) < days {
		days = len(bars)
	}
	recent := bars[len(bars)-days:]

	minLow := recent[0].low
	for _, b := range recent {
		if b.low.Cmp(minLow) < 0 {
			minLow = b.low
		}
	}

	current := bars[len(bars)-1].close.Float64()
	low := minLow.Float64()
	if low <= 0 {
		return 1.0
	}
	return (current - low) / low
}

// calcRelativeStrength calculates stock return vs SPY return over N days
func calcRelativeStrength(bars []dailyBar, spyReturns []float64, days int) float64 {
	if len(bars) < days || len(spyReturns) < days {
		return 0
	}

	// Stock return over period
	current := bars[len(bars)-1].close.Float64()
	past := bars[len(bars)-days-1].close.Float64()
	if past <= 0 {
		return 0
	}
	stockReturn := (current - past) / past

	// SPY return over same period (sum of daily returns)
	var spyReturn float64
	for i := len(spyReturns) - days; i < len(spyReturns); i++ {
		spyReturn += spyReturns[i]
	}

	return stockReturn - spyReturn
}
