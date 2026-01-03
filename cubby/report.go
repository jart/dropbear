package cubby

import (
	"dropbear/clocky"
	"dropbear/cubby/metrics"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"fmt"
	"math"
	"sync"
)

type report struct {
	lock              sync.RWMutex
	startTime         clocky.Time
	endTime           clocky.Time // last candle time (not scheduled event)
	startEquity       decimal.Decimal
	initialHoldings   []initialHolding
	benchmark         *Equity
	benchmarkQuantity decimal.Decimal
	benchmarkEquity   *metrics.Equity
	strategyEquity    *metrics.Equity
	strategyInvested  *metrics.Invested
}

type initialHolding struct {
	Symbol   string
	Quantity decimal.Decimal
}

func newReport(benchmark *Equity) *report {
	return &report{
		benchmark:        benchmark,
		benchmarkEquity:  metrics.NewEquity(*flagQuantum),
		strategyEquity:   metrics.NewEquity(*flagQuantum),
		strategyInvested: metrics.NewInvested(),
	}
}

// Init captures initial values for report.
func (r *report) Init() {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.startTime = clocky.Now()
	r.startEquity = GetEquityUSD()
	r.benchmarkQuantity = decimal.Zero
	for _, broker := range Brokers.All() {
		// handle cash from broker
		cash := broker.Cash.Load()
		if cash.IsPositive() {
			// convert USD to benchmark shares at current price
			if r.benchmark.LastPrice.Load().IsPositive() {
				r.benchmarkQuantity = r.benchmarkQuantity.Add(cash.Div(r.benchmark.LastPrice.Load()))
			}
			r.initialHoldings = append(r.initialHoldings, initialHolding{
				Symbol:   "USD",
				Quantity: cash,
			})
		}
		// handle stock holdings
		for _, holding := range broker.Holdings.All() {
			quantity := holding.Quantity.Load()
			if quantity.IsZero() {
				continue
			}
			if holding.Symbol == r.benchmark.Symbol {
				r.benchmarkQuantity = r.benchmarkQuantity.Add(quantity)
			} else {
				// convert other holdings to USD then to benchmark shares
				price := broker.Equities.GetPriceUSD(holding.Symbol)
				quantityUSD := price.Mul(quantity)
				if r.benchmark.LastPrice.Load().IsPositive() {
					r.benchmarkQuantity = r.benchmarkQuantity.Add(quantityUSD.Div(r.benchmark.LastPrice.Load()))
				}
			}
			r.initialHoldings = append(r.initialHoldings, initialHolding{
				Symbol:   holding.Symbol,
				Quantity: quantity,
			})
		}
	}
	if !Live && r.benchmarkQuantity.IsZero() {
		loggy.Fatalf("no initial cash or holdings set for backtest; forgot to call cubby.SetBalance()?")
	}
}

func (r *report) Sample(now clocky.Time) {
	// always sample invested on every tick for accurate min/max/avg
	invested := GetInvestedUSD()
	r.lock.Lock()
	r.endTime = now // track last candle time for CAGR calculation
	r.strategyInvested.Sample(invested)
	r.lock.Unlock()

	// sample equity on quantum intervals for Sharpe/drawdown calculations
	r.lock.RLock()
	shouldSample := r.strategyEquity.ShouldSample(now)
	r.lock.RUnlock()
	if shouldSample {
		equity := GetEquityUSD()
		r.lock.Lock()
		r.strategyEquity.Sample(now, equity)
		r.benchmarkEquity.Sample(now, r.benchmarkQuantity.Mul(r.benchmark.LastPrice.Load()))
		r.lock.Unlock()
	}
}

func (r *report) Print() {
	r.lock.Lock()
	endTime := r.endTime
	defer r.lock.Unlock()
	ex := Brokers.Get(ds.BrokerAlpaca)

	// get totals
	endEquity := GetEquityUSD()
	fees := ex.Fees.Load()

	// calculate usd volume from all holdings
	usdVolume := 0.0
	for _, holding := range ex.Holdings.All() {
		holding.Lock.RLock()
		usdVolume += holding.Volume
		holding.Lock.RUnlock()
	}

	// compute 30-day volume (scale from backtest duration)
	vol30day := 0.0
	duration := endTime.Sub(r.startTime)
	if duration > 0 {
		durationHours := float64(duration) / float64(clocky.Hour)
		if durationHours > 0 {
			vol30day = usdVolume * 30 * 24 / float64(durationHours)
		}
	}

	// benchmark: what if we just held the benchmark asset
	benchmarkValue := r.benchmarkQuantity.Mul(r.benchmark.LastPrice.Load())

	// profit/loss
	endProfit := decimal.Zero
	benchProfit := decimal.Zero
	if r.startEquity.IsPositive() {
		endProfit = endEquity.Sub(r.startEquity)
		benchProfit = benchmarkValue.Sub(r.startEquity)
	}

	// percent gain/loss
	endReturn := decimal.Zero
	benchReturn := decimal.Zero
	if r.startEquity.IsPositive() {
		endReturn = endEquity.Sub(r.startEquity).Div(r.startEquity)
		benchReturn = benchmarkValue.Sub(r.startEquity).Div(r.startEquity)
	}

	// compounding annual growth rate
	// the industry standard for comparing investment strategies
	cagr := 0.0
	benchCagr := 0.0
	if duration > 0 && r.startEquity.IsPositive() {
		years := float64(duration) / float64(clocky.Year)
		if years > 0 {
			totalReturn := endEquity.Div(r.startEquity).Float64()
			if totalReturn <= 0 {
				// lost everything (or more with margin) - CAGR is undefined
				cagr = math.Inf(-1)
			} else {
				cagr = (math.Pow(totalReturn, 1/years) - 1) * 100
				if cagr > 1e9 {
					cagr = math.Inf(1)
				}
			}
			benchReturn := benchmarkValue.Div(r.startEquity).Float64()
			if benchReturn <= 0 {
				benchCagr = math.Inf(-1)
			} else {
				benchCagr = (math.Pow(benchReturn, 1/years) - 1) * 100
				if benchCagr > 1e9 {
					benchCagr = math.Inf(1)
				}
			}
		}
	}

	// print start state
	fmt.Println()
	fmt.Printf("start %s\n", r.startTime)
	fmt.Printf("start.equity %s\n", r.startEquity.FormatThousand(2))
	for _, ih := range r.initialHoldings {
		if ih.Symbol == "USD" {
			fmt.Printf("start.usd %s\n", ih.Quantity.FormatThousand(2))
		}
	}

	// print end state
	fmt.Println()
	fmt.Printf("end %s\n", endTime)
	fmt.Printf("end.cagr %.2f\n", cagr)
	fmt.Printf("end.sharpe %s\n", r.strategyEquity.Sharpe(GetRiskFreeRate()).Format(3))
	fmt.Printf("end.profit %s\n", endProfit.FormatThousand(2))
	fmt.Printf("end.return %s\n", endReturn.MulInt(100).Format(2))
	fmt.Printf("end.equity %s\n", endEquity.FormatThousand(2))

	// print ending cash balance
	for _, broker := range Brokers.All() {
		cash := broker.Cash.Load()
		if cash.IsPositive() {
			fmt.Printf("end.usd %s\n", cash.FormatThousand(2))
		}
	}
	fmt.Printf("end.fees %s\n", fees.FormatThousand(2))
	// Margin interest (separate from trading fees/commissions)
	if ex.MarginInterest != nil {
		marginInterest := ex.MarginInterest.GetTotalCharged()
		if marginInterest.IsPositive() {
			fmt.Printf("end.interest %s\n", marginInterest.FormatThousand(2))
		}
	}
	fmt.Printf("end.vol30day %.6f\n", vol30day/1_000_000)
	fmt.Printf("end.drawdown %s\n", r.strategyEquity.MaxDrawdown().MulInt(100).Format(2))

	// aggregate win/loss counts
	var totalWins, totalLosses int
	for _, broker := range Brokers.All() {
		for _, holding := range broker.Holdings.All() {
			holding.Lock.RLock()
			totalWins += holding.WinCount
			totalLosses += holding.LossCount
			holding.Lock.RUnlock()
		}
	}
	if totalWins+totalLosses > 0 {
		winRate := float64(totalWins) / float64(totalWins+totalLosses) * 100
		fmt.Printf("end.wins %d\n", totalWins)
		fmt.Printf("end.losses %d\n", totalLosses)
		fmt.Printf("end.winrate %.1f\n", winRate)
	}

	// print trade counts
	buys, sells := 0, 0
	for _, broker := range Brokers.All() {
		for _, eq := range broker.Equities.All() {
			eq.Lock.RLock()
			buys += eq.Trades[ds.SideBuy]
			sells += eq.Trades[ds.SideSell]
			eq.Lock.RUnlock()
		}
	}
	fmt.Printf("end.buys %d\n", buys)
	fmt.Printf("end.sells %d\n", sells)
	fmt.Printf("end.trades %d\n", buys+sells)

	// invested metrics
	fmt.Printf("invested.min %s\n", r.strategyInvested.Min().FormatThousand(2))
	fmt.Printf("invested.avg %s\n", r.strategyInvested.Avg().FormatThousand(2))
	fmt.Printf("invested.max %s\n", r.strategyInvested.Max().FormatThousand(2))

	// margin metrics
	marginCalls := ex.MarginCallCount
	liquidatedValue := ex.LiquidatedValue.Load()
	if marginCalls > 0 || liquidatedValue.IsPositive() {
		fmt.Printf("margin.calls %d\n", marginCalls)
		fmt.Printf("margin.liquidated %s\n", liquidatedValue.FormatThousand(2))
	}

	// if you had just held the stock...
	fmt.Println()
	fmt.Printf("bench.cagr %.2f\n", benchCagr)
	fmt.Printf("bench.sharpe %s\n", r.benchmarkEquity.Sharpe(GetRiskFreeRate()).Format(3))
	fmt.Printf("bench.profit %s\n", benchProfit.FormatThousand(2))
	fmt.Printf("bench.return %s\n", benchReturn.MulInt(100).Format(2))
	fmt.Printf("bench.equity %s\n", benchmarkValue.FormatThousand(2))
	fmt.Printf("bench.maxdd %s\n", r.benchmarkEquity.MaxDrawdown().MulInt(100).Format(2))
}
