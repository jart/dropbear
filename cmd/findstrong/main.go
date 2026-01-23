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
	volTarget = flag.Float64("vol-target", 500e6, "target daily notional volume (center of bell curve)")
	volSigma  = flag.Float64("vol-sigma", 2.0, "log-scale sigma for volume bell curve (higher = wider)")
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

	// Calculate cutoff time
	cutoff := clocky.Now() - clocky.Time(clocky.Duration(*days)*clocky.Day*7/5)

	minutesDir := ds.EquityMinutesDir()

	// Load SPY bars for relative strength calculation
	spyBars, err := loadDailyBars(filepath.Join(minutesDir, "SPY"), cutoff)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load SPY bars: %v\n", err)
	}
	spyReturns := calcDailyReturns(spyBars)

	type result struct {
		symbol       string
		monotonicity float64 // upward monotonicity (1 = steady uptrend)
		avgNotional  float64
		volScore     float64
		sharpe       float64
		sharpeScore  float64
		// Filter metrics
		distFromHigh float64 // room to run (want 10-40% below high)
		distFromLow  float64 // bounced off bottom (want 10-30% above low)
		recovery20d  float64 // 20-day recovery from 60-day low
		acceleration float64 // positive = trend strengthening
		relStrength  float64 // vs SPY over 20 days
		volTrend     float64 // recent volume vs avg (buying interest)
		// Scores
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

		if len(dailyBars) < 60 {
			continue
		}

		r := result{symbol: sym}

		// Upward monotonicity (invert the downward calculation)
		r.monotonicity = 1.0 - calcDownwardMonotonicity(dailyBars)
		r.avgNotional = calcAvgDailyNotional(dailyBars)
		r.volScore = calcVolumeSweetSpot(r.avgNotional, *volTarget, *volSigma)
		r.sharpe = calcDailySharpe(dailyBars)

		// Filter metrics for longs
		r.distFromHigh = calcDistanceFromHigh(dailyBars, 252) // 52-week high
		r.distFromLow = calcDistanceFromLow(dailyBars, 60)    // 60-day low
		r.recovery20d = calcRecoveryFromLow(dailyBars, 20, 60)
		r.acceleration = calcAcceleration(dailyBars)
		r.relStrength = calcRelativeStrength(dailyBars, spyReturns, 20)
		r.volTrend = calcVolumeTrend(dailyBars, 10, 60)

		results = append(results, r)
	}

	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "no symbols with sufficient data")
		os.Exit(1)
	}

	// Normalize sharpe to 0-1 (highest = best for longs)
	slices.SortFunc(results, func(a, b result) int {
		return cmp.Compare(b.sharpe, a.sharpe) // descending
	})
	for i := range results {
		results[i].sharpeScore = 1.0 - float64(i)/float64(len(results)-1)
	}

	// Calculate base and adjusted scores
	for i := range results {
		r := &results[i]

		// Base score: upward monotonicity, volume, positive sharpe
		r.baseScore = (r.monotonicity + r.volScore + r.sharpeScore) / 3.0

		// === BONUS FACTORS (what we want) ===

		// "Down on its luck" bonus: 15-35% below 52-week high is the sweet spot
		// Too close to high = limited upside; too far = might be broken
		downtroddenBonus := 1.0
		if r.distFromHigh >= 0.15 && r.distFromHigh <= 0.35 {
			downtroddenBonus = 1.4 // sweet spot: room to run
		} else if r.distFromHigh >= 0.10 && r.distFromHigh <= 0.40 {
			downtroddenBonus = 1.2
		} else if r.distFromHigh < 0.05 {
			downtroddenBonus = 0.7 // near ATH, limited upside
		} else if r.distFromHigh > 0.50 {
			downtroddenBonus = 0.6 // down >50%, might be broken
		}

		// "Trend reversing" bonus: bounced off lows
		// Want stocks that found a bottom and are recovering
		recoveryBonus := 1.0
		if r.distFromLow >= 0.10 && r.distFromLow <= 0.30 {
			recoveryBonus = 1.3 // bounced nicely, not overextended
		} else if r.distFromLow >= 0.05 && r.distFromLow <= 0.40 {
			recoveryBonus = 1.15
		} else if r.distFromLow < 0.03 {
			recoveryBonus = 0.5 // still at lows, no reversal yet
		}

		// Acceleration bonus: positive = trend strengthening
		accelBonus := 1.0
		if r.acceleration > 0.02 {
			accelBonus = 1.3 // strongly accelerating up
		} else if r.acceleration > 0.01 {
			accelBonus = 1.15
		} else if r.acceleration < -0.02 {
			accelBonus = 0.5 // decelerating, trend weakening
		} else if r.acceleration < -0.01 {
			accelBonus = 0.7
		}

		// Relative strength bonus: outperforming SPY
		relStrBonus := 1.0
		if r.relStrength > 0.05 {
			relStrBonus = 1.25 // beating market by 5%+
		} else if r.relStrength > 0.02 {
			relStrBonus = 1.1
		} else if r.relStrength < -0.05 {
			relStrBonus = 0.7 // lagging market badly
		}

		// Volume trend bonus: increasing volume = buying interest
		volTrendBonus := 1.0
		if r.volTrend > 1.3 {
			volTrendBonus = 1.2 // 30%+ more volume recently
		} else if r.volTrend > 1.1 {
			volTrendBonus = 1.1
		} else if r.volTrend < 0.7 {
			volTrendBonus = 0.8 // volume drying up
		}

		// === PENALTY FACTORS (what we avoid) ===

		// Freefall penalty: if still making new lows, don't catch falling knife
		freefallPenalty := 1.0
		if r.recovery20d < 0.02 {
			freefallPenalty = 0.4 // barely bounced in 20 days
		} else if r.recovery20d < 0.05 {
			freefallPenalty = 0.7
		}

		r.adjustedScore = r.baseScore *
			downtroddenBonus *
			recoveryBonus *
			accelBonus *
			relStrBonus *
			volTrendBonus *
			freefallPenalty
	}

	// Sort by adjusted score descending
	slices.SortFunc(results, func(a, b result) int {
		if c := cmp.Compare(b.adjustedScore, a.adjustedScore); c != 0 {
			return c
		}
		return cmp.Compare(a.symbol, b.symbol)
	})

	// Print results
	fmt.Printf("%-6s %5s %5s %6s %5s %6s %5s %5s %6s %15s\n",
		"SYM", "MONO", "VOL", "SHARPE", "BASE", "ADJST", "FHIGH", "FLOW", "ACCEL", "AVG_NOTIONAL")
	for _, r := range results {
		fmt.Printf("%-6s %5.2f %5.2f %6.3f %5.2f %6.3f %4.0f%% %5.0f%% %+5.1f%% %15.0f\n",
			r.symbol,
			r.monotonicity,
			r.volScore,
			r.sharpe,
			r.baseScore,
			r.adjustedScore,
			r.distFromHigh*100,
			r.distFromLow*100,
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

// calcDownwardMonotonicity calculates Spearman rank correlation of price vs time
// Returns 0-1 where 1 = perfectly monotonic downward trend
func calcDownwardMonotonicity(bars []dailyBar) float64 {
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

// calcDistanceFromLow calculates how far price is above N-day low
func calcDistanceFromLow(bars []dailyBar, days int) float64 {
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

// calcRecoveryFromLow calculates recovery over recentDays from lowDays low
func calcRecoveryFromLow(bars []dailyBar, recentDays, lowDays int) float64 {
	if len(bars) < lowDays {
		return 0
	}

	// Find low over lowDays period
	lowBars := bars[len(bars)-lowDays:]
	minLow := lowBars[0].low
	for _, b := range lowBars {
		if b.low.Cmp(minLow) < 0 {
			minLow = b.low
		}
	}

	// Price recentDays ago
	if len(bars) < recentDays+1 {
		return 0
	}
	pastPrice := bars[len(bars)-recentDays-1].close.Float64()
	currentPrice := bars[len(bars)-1].close.Float64()
	low := minLow.Float64()

	if low <= 0 || pastPrice <= 0 {
		return 0
	}

	// How much of the recovery happened in the recent period
	return (currentPrice - pastPrice) / pastPrice
}

// calcAcceleration measures if trend is strengthening or weakening
func calcAcceleration(bars []dailyBar) float64 {
	if len(bars) < 21 {
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

	return momentum5d - (momentum20d / 4)
}

// calcRelativeStrength calculates stock return vs SPY return over N days
func calcRelativeStrength(bars []dailyBar, spyReturns []float64, days int) float64 {
	if len(bars) < days+1 || len(spyReturns) < days {
		return 0
	}

	current := bars[len(bars)-1].close.Float64()
	past := bars[len(bars)-days-1].close.Float64()
	if past <= 0 {
		return 0
	}
	stockReturn := (current - past) / past

	var spyReturn float64
	for i := len(spyReturns) - days; i < len(spyReturns); i++ {
		spyReturn += spyReturns[i]
	}

	return stockReturn - spyReturn
}

// calcVolumeTrend calculates recent volume vs average volume
func calcVolumeTrend(bars []dailyBar, recentDays, avgDays int) float64 {
	if len(bars) < avgDays {
		return 1.0
	}

	// Average volume
	avgBars := bars[len(bars)-avgDays:]
	var totalVol decimal.Decimal
	for _, b := range avgBars {
		totalVol = totalVol.Add(b.volume)
	}
	avgVol := totalVol.Float64() / float64(len(avgBars))

	// Recent average volume
	if recentDays > len(bars) {
		recentDays = len(bars)
	}
	recentBars := bars[len(bars)-recentDays:]
	var recentVol decimal.Decimal
	for _, b := range recentBars {
		recentVol = recentVol.Add(b.volume)
	}
	recentAvgVol := recentVol.Float64() / float64(len(recentBars))

	if avgVol < 1 {
		return 1.0
	}
	return recentAvgVol / avgVol
}
