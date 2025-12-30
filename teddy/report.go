package teddy

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"dropbear/teddy/metrics"
	"fmt"
	"math"
	"strings"
	"sync"
)

type report struct {
	lock              sync.RWMutex
	startTime         clocky.Time
	startEquity       decimal.Decimal
	endEquity         decimal.Decimal
	initialHoldings   []initialHolding
	benchmark         *Pair
	benchmarkQuantity decimal.Decimal
	benchmarkEquity   *metrics.Equity
	strategyEquity    *metrics.Equity
	strategyInvested  *metrics.Invested
}

type initialHolding struct {
	Symbol   string
	Quantity decimal.Decimal
}

func newReport(benchmark *Pair) *report {
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
		for _, holding := range broker.Holdings.All() {
			quantity := holding.Quantity.Load()
			if holding.Symbol == r.benchmark.BaseCurrency.Symbol {
				r.benchmarkQuantity = r.benchmarkQuantity.Add(quantity)
			} else {
				quantityUSD := broker.Pairs.GetPriceUSD(holding.Symbol).Mul(quantity)
				r.benchmarkQuantity = r.benchmarkQuantity.Add(quantityUSD.DivEven(r.benchmark.LastPrice.Load()))
			}
			r.initialHoldings = append(r.initialHoldings, initialHolding{
				Symbol:   holding.Symbol,
				Quantity: quantity,
			})
		}
	}
	if !Live && r.benchmarkQuantity.IsZero() {
		loggy.Fatalf("no initial cash or holdings set for backtest; forgot to call teddy.SetBalance()?")
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
	cb := Brokers.Get(ds.BrokerCoinbase)

	// get total volume from usd holding
	usd := cb.Holdings.Get("USD")
	endEquity := GetEquityUSD()
	riskFreeRate := GetRiskFreeRate().Float64()
	fees := cb.Fees.Load()
	rebate := cb.Rebate.Load()
	rebates := fees.Mul(rebate)
	usd.Lock.RLock()
	usdVolume := usd.Volume
	usd.Lock.RUnlock()

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
		endReturn = endEquity.Sub(r.startEquity).DivEven(r.startEquity)
		benchReturn = benchmarkValue.Sub(r.startEquity).DivEven(r.startEquity)
	}

	// cagr calculation - compound at quantum intervals then annualize
	cagr := 0.0
	if duration > 0 && r.startEquity.IsPositive() {
		quantums := float64(duration) / float64(*flagQuantum)
		quantumsPerYear := float64(365*24*clocky.Hour) / float64(*flagQuantum)
		if quantums > 0 {
			totalReturn := endEquity.DivEven(r.startEquity).Float64()
			cagr = (math.Pow(totalReturn, quantumsPerYear/quantums) - 1) * 100
			if cagr > 1e9 {
				cagr = math.Inf(1)
			}
		}
	}

	// print start state
	fmt.Println()
	fmt.Printf("start %s\n", r.startTime)
	fmt.Printf("start.equity %s\n", r.startEquity)
	for _, ih := range r.initialHoldings {
		fmt.Printf("start.%s %s\n", strings.ToLower(ih.Symbol), ih.Quantity)
	}

	// print end state
	fmt.Println()
	fmt.Printf("end %s\n", endTime)
	fmt.Printf("end.profit %s\n", endProfit)
	fmt.Printf("end.return %s\n", endReturn.MulInt(100))
	fmt.Printf("end.sharpe %.2f\n", r.strategyEquity.Sharpe(riskFreeRate))
	fmt.Printf("end.equity %s\n", endEquity)
	for _, broker := range Brokers.All() {
		for _, holding := range broker.Holdings.All() {
			qty := holding.Quantity.Load()
			if qty.IsPositive() {
				fmt.Printf("end.%s %s\n", strings.ToLower(holding.Symbol), qty)
			}
		}
	}
	fmt.Printf("end.fees %s\n", fees)
	fmt.Printf("end.rebates %s\n", rebates)
	fmt.Printf("end.vol30day %.6f\n", vol30day/1_000_000)
	fmt.Printf("end.cagr %.2f\n", cagr)
	fmt.Printf("end.maxdd %.2f\n", r.strategyEquity.MaxDrawdown()*100)

	// aggregate win/loss counts across all non-cash holdings
	var totalWins, totalLosses int
	for _, broker := range Brokers.All() {
		for _, holding := range broker.Holdings.All() {
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

	// print trade counts
	buys, sells := 0, 0
	for _, broker := range Brokers.All() {
		for _, pair := range broker.Pairs.All() {
			pair.Lock.RLock()
			buys += pair.Trades[ds.SideBuy]
			sells += pair.Trades[ds.SideSell]
			pair.Lock.RUnlock()
		}
	}
	fmt.Printf("end.buys %d\n", buys)
	fmt.Printf("end.sells %d\n", sells)
	fmt.Printf("end.trades %d\n", buys+sells)

	// usd invested efficiency metrics
	fmt.Printf("invested.min %s\n", r.strategyInvested.Min())
	fmt.Printf("invested.max %s\n", r.strategyInvested.Max())
	fmt.Printf("invested.avg %s\n", r.strategyInvested.Avg())

	// print benchmark
	fmt.Println()
	fmt.Printf("bench.profit %s\n", benchProfit)
	fmt.Printf("bench.return %s\n", benchReturn.MulInt(100))
	fmt.Printf("bench.sharpe %.2f\n", r.benchmarkEquity.Sharpe(riskFreeRate))
	fmt.Printf("bench.equity %s\n", benchmarkValue)
	fmt.Printf("bench.maxdd %.2f\n", r.benchmarkEquity.MaxDrawdown()*100)
}
