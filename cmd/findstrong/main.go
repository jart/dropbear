package main

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/netty"
	"flag"
	"fmt"
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
	volTarget = decimal.Flag("vol-target", "500", "target daily notional volume in millions (center of bell curve)")
	volSigma  = decimal.Flag("vol-sigma", "2", "log-scale sigma for volume bell curve (higher = wider)")
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
		symbol        string
		monotonicity  decimal.Decimal
		avgNotional   decimal.Decimal // in millions
		volScore      decimal.Decimal
		sharpe        decimal.Decimal
		sharpeScore   decimal.Decimal
		distFromHigh  decimal.Decimal
		distFromLow   decimal.Decimal
		recovery20d   decimal.Decimal
		acceleration  decimal.Decimal
		relStrength   decimal.Decimal
		volTrend      decimal.Decimal
		baseScore     decimal.Decimal
		adjustedScore decimal.Decimal
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

		r.monotonicity = decimal.One.Sub(calcDownwardMonotonicity(dailyBars))
		r.avgNotional = calcAvgDailyNotional(dailyBars)
		r.volScore = calcVolumeSweetSpot(r.avgNotional, *volTarget, *volSigma)
		r.sharpe = calcDailySharpe(dailyBars)

		r.distFromHigh = calcDistanceFromHigh(dailyBars, 252)
		r.distFromLow = calcDistanceFromLow(dailyBars, 60)
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
		return b.sharpe.Cmp(a.sharpe)
	})
	n := decimal.FromInt(len(results) - 1)
	for i := range results {
		results[i].sharpeScore = decimal.One.Sub(decimal.FromInt(i).Div(n))
	}

	d015 := decimal.Parse("0.15")
	d035 := decimal.Parse("0.35")
	d010 := decimal.Parse("0.10")
	d040 := decimal.Parse("0.40")
	d005 := decimal.Parse("0.05")
	d050 := decimal.Parse("0.50")
	d003 := decimal.Parse("0.03")
	d030 := decimal.Parse("0.30")
	d002 := decimal.Parse("0.02")
	d001 := decimal.Parse("0.01")

	b140 := decimal.Parse("1.4")
	b130 := decimal.Parse("1.3")
	b125 := decimal.Parse("1.25")
	b120 := decimal.Parse("1.2")
	b115 := decimal.Parse("1.15")
	b110 := decimal.Parse("1.1")
	b080 := decimal.Parse("0.8")
	b070 := decimal.Parse("0.7")
	b060 := decimal.Parse("0.6")
	b050 := decimal.Parse("0.5")
	b040 := decimal.Parse("0.4")

	// Calculate base and adjusted scores
	for i := range results {
		r := &results[i]

		r.baseScore = r.monotonicity.Add(r.volScore).Add(r.sharpeScore).DivInt(3)

		// "Down on its luck" bonus: 15-35% below 52-week high
		downtrodden := decimal.One
		if r.distFromHigh.Cmp(d015) >= 0 && r.distFromHigh.Cmp(d035) <= 0 {
			downtrodden = b140
		} else if r.distFromHigh.Cmp(d010) >= 0 && r.distFromHigh.Cmp(d040) <= 0 {
			downtrodden = b120
		} else if r.distFromHigh.Cmp(d005) < 0 {
			downtrodden = b070
		} else if r.distFromHigh.Cmp(d050) > 0 {
			downtrodden = b060
		}

		// "Trend reversing" bonus: bounced off lows
		recovery := decimal.One
		if r.distFromLow.Cmp(d010) >= 0 && r.distFromLow.Cmp(d030) <= 0 {
			recovery = b130
		} else if r.distFromLow.Cmp(d005) >= 0 && r.distFromLow.Cmp(d040) <= 0 {
			recovery = b115
		} else if r.distFromLow.Cmp(d003) < 0 {
			recovery = b050
		}

		// Acceleration bonus
		accel := decimal.One
		if r.acceleration.Cmp(d002) > 0 {
			accel = b130
		} else if r.acceleration.Cmp(d001) > 0 {
			accel = b115
		} else if r.acceleration.Cmp(d002.Neg()) < 0 {
			accel = b050
		} else if r.acceleration.Cmp(d001.Neg()) < 0 {
			accel = b070
		}

		// Relative strength bonus: outperforming SPY
		relStr := decimal.One
		if r.relStrength.Cmp(d005) > 0 {
			relStr = b125
		} else if r.relStrength.Cmp(d002) > 0 {
			relStr = b110
		} else if r.relStrength.Cmp(d005.Neg()) < 0 {
			relStr = b070
		}

		// Volume trend bonus
		volTrend := decimal.One
		if r.volTrend.Cmp(b130) > 0 {
			volTrend = b120
		} else if r.volTrend.Cmp(b110) > 0 {
			volTrend = b110
		} else if r.volTrend.Cmp(b070) < 0 {
			volTrend = b080
		}

		// Freefall penalty
		freefall := decimal.One
		if r.recovery20d.Cmp(d002) < 0 {
			freefall = b040
		} else if r.recovery20d.Cmp(d005) < 0 {
			freefall = b070
		}

		r.adjustedScore = r.baseScore.
			Mul(downtrodden).
			Mul(recovery).
			Mul(accel).
			Mul(relStr).
			Mul(volTrend).
			Mul(freefall)
	}

	// Sort by adjusted score descending
	slices.SortFunc(results, func(a, b result) int {
		if c := b.adjustedScore.Cmp(a.adjustedScore); c != 0 {
			return c
		}
		if a.symbol < b.symbol {
			return -1
		}
		if a.symbol > b.symbol {
			return 1
		}
		return 0
	})

	// Print results
	fmt.Printf("%-6s %5s %5s %6s %5s %6s %5s %5s %7s %10s\n",
		"SYM", "MONO", "VOL", "SHARPE", "BASE", "ADJST", "FHIGH", "FLOW", "ACCEL", "NOTIONAL_M")
	for _, r := range results {
		fmt.Printf("%-6s %5s %5s %6s %5s %6s %4s%% %4s%% %+6s%% %10s\n",
			r.symbol,
			r.monotonicity.Format(2),
			r.volScore.Format(2),
			r.sharpe.Format(3),
			r.baseScore.Format(2),
			r.adjustedScore.Format(3),
			r.distFromHigh.MulInt(100).Format(0),
			r.distFromLow.MulInt(100).Format(0),
			r.acceleration.MulInt(100).Format(1),
			r.avgNotional.Format(2))
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

// calcDailyReturns calculates daily returns for relative strength.
// Returns are scaled by 1,000,000 to preserve precision in decimal.
func calcDailyReturns(bars []dailyBar) []decimal.Decimal {
	if len(bars) < 2 {
		return nil
	}
	returns := make([]decimal.Decimal, len(bars))
	for i := 1; i < len(bars); i++ {
		prev := bars[i-1].close
		curr := bars[i].close
		if prev.IsPositive() {
			returns[i] = curr.Sub(prev).Div(prev)
		}
	}
	return returns
}

// calcDownwardMonotonicity calculates Spearman rank correlation of price vs time.
// Returns 0-1 where 1 = perfectly monotonic downward trend.
func calcDownwardMonotonicity(bars []dailyBar) decimal.Decimal {
	n := len(bars)
	if n < 2 {
		return decimal.Parse("0.5")
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

	priceRanks := make([]decimal.Decimal, n)
	for rank, c := range closes {
		priceRanks[c.idx] = decimal.FromInt(rank + 1)
	}

	var sumD2 decimal.Decimal
	for i := 0; i < n; i++ {
		d := priceRanks[i].Sub(decimal.FromInt(i + 1))
		sumD2 = sumD2.Add(d.Sqr())
	}

	// spearman = 1 - 6*sumD2 / (n*(n*n-1))
	nn := decimal.FromInt(n)
	denom := nn.Mul(decimal.FromInt(n*n - 1))
	spearman := decimal.One.Sub(sumD2.MulInt(6).Div(denom))
	return decimal.One.Sub(spearman).DivInt(2)
}

// calcAvgDailyNotional calculates average daily notional volume in millions.
func calcAvgDailyNotional(bars []dailyBar) decimal.Decimal {
	if len(bars) == 0 {
		return decimal.Zero
	}
	var total decimal.Decimal
	for _, b := range bars {
		total = total.Add(b.notional.DivInt64(1_000_000))
	}
	return total.DivInt(len(bars))
}

// calcVolumeSweetSpot scores volume using a log-normal bell curve.
func calcVolumeSweetSpot(avgNotional, target, sigma decimal.Decimal) decimal.Decimal {
	if !avgNotional.IsPositive() || !target.IsPositive() {
		return decimal.Zero
	}
	logVol := avgNotional.Log()
	logTarget := target.Log()
	z := logVol.Sub(logTarget).Div(sigma)
	// exp(-0.5 * z * z)
	return z.Sqr().DivInt(2).Neg().Exp()
}

// calcDailySharpe calculates the daily Sharpe ratio.
func calcDailySharpe(bars []dailyBar) decimal.Decimal {
	if len(bars) < 2 {
		return decimal.Zero
	}

	n := len(bars) - 1
	returns := make([]decimal.Decimal, n)
	for i := 1; i < len(bars); i++ {
		prev := bars[i-1].close
		curr := bars[i].close
		if prev.IsPositive() {
			returns[i-1] = curr.Sub(prev).Div(prev)
		}
	}

	var sum decimal.Decimal
	for _, r := range returns {
		sum = sum.Add(r)
	}
	mean := sum.DivInt(n)

	var sumSq decimal.Decimal
	for _, r := range returns {
		d := r.Sub(mean)
		sumSq = sumSq.Add(d.Sqr())
	}
	variance := sumSq.DivInt(n)
	std := variance.Sqrt()

	if std.IsZero() {
		return decimal.Zero
	}
	return mean.Div(std)
}

// calcDistanceFromHigh calculates how far price is from N-day high.
func calcDistanceFromHigh(bars []dailyBar, days int) decimal.Decimal {
	if len(bars) < days {
		days = len(bars)
	}
	recent := bars[len(bars)-days:]

	var maxHigh decimal.Decimal
	for _, b := range recent {
		maxHigh = maxHigh.Max(b.high)
	}

	if !maxHigh.IsPositive() {
		return decimal.Zero
	}
	current := bars[len(bars)-1].close
	return maxHigh.Sub(current).Div(maxHigh)
}

// calcDistanceFromLow calculates how far price is above N-day low.
func calcDistanceFromLow(bars []dailyBar, days int) decimal.Decimal {
	if len(bars) < days {
		days = len(bars)
	}
	recent := bars[len(bars)-days:]

	minLow := recent[0].low
	for _, b := range recent {
		minLow = minLow.Min(b.low)
	}

	if !minLow.IsPositive() {
		return decimal.Zero
	}
	current := bars[len(bars)-1].close
	return current.Sub(minLow).Div(minLow)
}

// calcRecoveryFromLow calculates recovery over recentDays from lowDays low.
func calcRecoveryFromLow(bars []dailyBar, recentDays, lowDays int) decimal.Decimal {
	if len(bars) < lowDays || len(bars) < recentDays+1 {
		return decimal.Zero
	}

	pastPrice := bars[len(bars)-recentDays-1].close
	currentPrice := bars[len(bars)-1].close

	if !pastPrice.IsPositive() {
		return decimal.Zero
	}

	return currentPrice.Sub(pastPrice).Div(pastPrice)
}

// calcAcceleration measures if trend is strengthening or weakening.
func calcAcceleration(bars []dailyBar) decimal.Decimal {
	if len(bars) < 21 {
		return decimal.Zero
	}

	current := bars[len(bars)-1].close
	close5d := bars[len(bars)-6].close
	close20d := bars[len(bars)-21].close

	if !close5d.IsPositive() || !close20d.IsPositive() {
		return decimal.Zero
	}

	momentum5d := current.Sub(close5d).Div(close5d)
	momentum20d := current.Sub(close20d).Div(close20d)

	return momentum5d.Sub(momentum20d.DivInt(4))
}

// calcRelativeStrength calculates stock return vs SPY return over N days.
func calcRelativeStrength(bars []dailyBar, spyReturns []decimal.Decimal, days int) decimal.Decimal {
	if len(bars) < days+1 || len(spyReturns) < days {
		return decimal.Zero
	}

	current := bars[len(bars)-1].close
	past := bars[len(bars)-days-1].close
	if !past.IsPositive() {
		return decimal.Zero
	}
	stockReturn := current.Sub(past).Div(past)

	var spyReturn decimal.Decimal
	for i := len(spyReturns) - days; i < len(spyReturns); i++ {
		spyReturn = spyReturn.Add(spyReturns[i])
	}

	return stockReturn.Sub(spyReturn)
}

// calcVolumeTrend calculates recent volume vs average volume.
func calcVolumeTrend(bars []dailyBar, recentDays, avgDays int) decimal.Decimal {
	if len(bars) < avgDays {
		return decimal.One
	}

	avgBars := bars[len(bars)-avgDays:]
	var totalVol decimal.Decimal
	for _, b := range avgBars {
		totalVol = totalVol.Add(b.volume)
	}
	avgVol := totalVol.DivInt(len(avgBars))

	if !avgVol.IsPositive() {
		return decimal.One
	}

	if recentDays > len(bars) {
		recentDays = len(bars)
	}
	recentBars := bars[len(bars)-recentDays:]
	var recentVol decimal.Decimal
	for _, b := range recentBars {
		recentVol = recentVol.Add(b.volume)
	}
	recentAvgVol := recentVol.DivInt(len(recentBars))

	return recentAvgVol.Div(avgVol)
}
