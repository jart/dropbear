package cubby

import (
	"dropbear/clocky"
	"dropbear/cubby/metrics"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"fmt"
	"math"
	"strings"
	"sync"
)

type report struct {
	lock              sync.RWMutex
	startTime         clocky.Time
	startEquity       decimal.Decimal
	initialHoldings   []initialHolding
	benchmark         *Equity
	benchmarkQuantity decimal.Decimal
	benchmarkEquity   *metrics.Equity
	strategyEquity    *metrics.Equity
	strategyInvested  *metrics.Invested
}

type initialHolding struct {
	IsCash   bool
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
	for _, exchange := range Exchanges.All() {
		for _, holding := range exchange.Holdings.All() {
			quantity := holding.Quantity.Load()
			switch holding.Symbol {
			case r.benchmark.Symbol:
				r.benchmarkQuantity = r.benchmarkQuantity.Add(quantity)
			case "USD":
				// Convert USD to benchmark shares at current price
				if r.benchmark.LastPrice.Load().IsPositive() {
					r.benchmarkQuantity = r.benchmarkQuantity.Add(quantity.Div(r.benchmark.LastPrice.Load()))
				}
			default:
				// Convert other holdings to USD then to benchmark shares
				price := exchange.Equities.GetPriceUSD(holding.Symbol)
				quantityUSD := price.Mul(quantity)
				if r.benchmark.LastPrice.Load().IsPositive() {
					r.benchmarkQuantity = r.benchmarkQuantity.Add(quantityUSD.Div(r.benchmark.LastPrice.Load()))
				}
			}
			r.initialHoldings = append(r.initialHoldings, initialHolding{
				IsCash:   holding.IsCash,
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
	shouldSample := func() bool {
		r.lock.RLock()
		defer r.lock.RUnlock()
		return r.strategyEquity.ShouldSample(now)
	}()
	if shouldSample {
		invested := GetInvestedUSD()
		equity := GetEquityUSD()
		r.lock.Lock()
		defer r.lock.Unlock()
		r.strategyInvested.Sample(invested)
		r.strategyEquity.Sample(now, equity)
		r.benchmarkEquity.Sample(now, r.benchmarkQuantity.Mul(r.benchmark.LastPrice.Load()))
	}
}

func (r *report) Print() {
	endTime := clocky.Now()
	r.lock.Lock()
	defer r.lock.Unlock()
	ex := Exchanges.Get(ds.ExchangeAlpaca)

	// Get totals from USD holding
	usd := ex.Holdings.Get("USD")
	endEquity := GetEquityUSD()
	riskFreeRate := GetRiskFreeRate().Float64()
	fees := ex.Fees.Load()
	usd.Lock.RLock()
	usdVolume := usd.Volume
	usd.Lock.RUnlock()

	// Compute 30-day volume (scale from backtest duration)
	vol30day := 0.0
	duration := endTime.Sub(r.startTime)
	if duration > 0 {
		durationHours := float64(duration) / float64(clocky.Hour)
		if durationHours > 0 {
			vol30day = usdVolume * 30 * 24 / float64(durationHours)
		}
	}

	// Benchmark: what if we just held the benchmark asset
	benchmarkValue := r.benchmarkQuantity.Mul(r.benchmark.LastPrice.Load())

	// Profit/loss
	endProfit := decimal.Zero
	benchProfit := decimal.Zero
	if r.startEquity.IsPositive() {
		endProfit = endEquity.Sub(r.startEquity)
		benchProfit = benchmarkValue.Sub(r.startEquity)
	}

	// Percent gain/loss
	endReturn := decimal.Zero
	benchReturn := decimal.Zero
	if r.startEquity.IsPositive() {
		endReturn = endEquity.Sub(r.startEquity).Div(r.startEquity)
		benchReturn = benchmarkValue.Sub(r.startEquity).Div(r.startEquity)
	}

	// CAGR calculation (uses calendar years, not trading time)
	cagr := 0.0
	if duration > 0 && r.startEquity.IsPositive() {
		years := float64(duration) / float64(clocky.Year)
		if years > 0 {
			totalReturn := endEquity.Div(r.startEquity).Float64()
			cagr = (math.Pow(totalReturn, 1/years) - 1) * 100
			if cagr > 1e9 {
				cagr = math.Inf(1)
			}
		}
	}

	// Print start state
	fmt.Println()
	fmt.Printf("start %s\n", r.startTime)
	fmt.Printf("start.equity %s\n", r.startEquity)
	for _, ih := range r.initialHoldings {
		if ih.IsCash {
			fmt.Printf("start.%s %s\n", strings.ToLower(ih.Symbol), ih.Quantity)
		}
	}

	// Print end state
	fmt.Println()
	fmt.Printf("end %s\n", endTime)
	fmt.Printf("end.profit %s\n", endProfit)
	fmt.Printf("end.return %s\n", endReturn.MulInt(100))
	fmt.Printf("end.sharpe %.2f\n", r.strategyEquity.Sharpe(riskFreeRate))
	fmt.Printf("end.equity %s\n", endEquity)
	for _, exchange := range Exchanges.All() {
		for _, holding := range exchange.Holdings.All() {
			qty := holding.Quantity.Load()
			if qty.IsPositive() && holding.IsCash {
				fmt.Printf("end.%s %s\n", strings.ToLower(holding.Symbol), qty)
			}
		}
	}
	fmt.Printf("end.fees %s\n", fees)
	fmt.Printf("end.vol30day %.6f\n", vol30day/1_000_000)
	fmt.Printf("end.cagr %.2f\n", cagr)
	fmt.Printf("end.maxdd %.2f\n", r.strategyEquity.MaxDrawdown()*100)

	// Aggregate win/loss counts
	var totalWins, totalLosses int
	for _, exchange := range Exchanges.All() {
		for _, holding := range exchange.Holdings.All() {
			if holding.IsCash {
				continue
			}
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

	// Print trade counts
	buys, sells := 0, 0
	for _, exchange := range Exchanges.All() {
		for _, eq := range exchange.Equities.All() {
			eq.Lock.RLock()
			buys += eq.Trades[ds.SideBuy]
			sells += eq.Trades[ds.SideSell]
			eq.Lock.RUnlock()
		}
	}
	fmt.Printf("end.buys %d\n", buys)
	fmt.Printf("end.sells %d\n", sells)
	fmt.Printf("end.trades %d\n", buys+sells)

	// Invested metrics
	fmt.Printf("invested.min %s\n", r.strategyInvested.Min())
	fmt.Printf("invested.max %s\n", r.strategyInvested.Max())
	fmt.Printf("invested.avg %s\n", r.strategyInvested.Avg())

	// Print benchmark
	fmt.Println()
	fmt.Printf("bench.profit %s\n", benchProfit)
	fmt.Printf("bench.return %s\n", benchReturn.MulInt(100))
	fmt.Printf("bench.sharpe %.2f\n", r.benchmarkEquity.Sharpe(riskFreeRate))
	fmt.Printf("bench.equity %s\n", benchmarkValue)
	fmt.Printf("bench.maxdd %.2f\n", r.benchmarkEquity.MaxDrawdown()*100)
}
