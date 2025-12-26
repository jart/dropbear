package teddy

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"fmt"
	"math"
	"strings"
	"sync"
)

type report struct {
	lock        sync.Mutex
	startTime   clocky.Time
	endTime     clocky.Time
	startEquity decimal.Decimal
	endEquity   decimal.Decimal
	trades      map[ds.Side]int
}

func newReport() *report {
	return &report{
		trades: make(map[ds.Side]int),
	}
}

func (r *report) Print() {
	cb := Exchanges.Get(ds.ExchangeCoinbase)

	// get total volume from usd holding
	usd := cb.Holdings.Get("USD")
	usd.Lock.RLock()
	usdVolume := usd.Volume
	usd.Lock.RUnlock()

	endEquity := GetEquityUSD()
	riskFreeRate := GetRiskFreeRate().Float64()

	cb.Lock.RLock()
	fees := cb.Fees
	rebate := cb.Rebate
	cb.Lock.RUnlock()

	// compute rebates
	rebates := fees.Mul(rebate)

	// compute 30-day volume (scale from backtest duration)
	duration := r.endTime.Sub(r.startTime)
	vol30day := decimal.Zero
	if duration > 0 {
		durationHours := int(duration / clocky.Hour)
		if durationHours > 0 {
			vol30day = usdVolume.MulInt(30 * 24).DivInt(durationHours)
		}
	}

	// benchmark: what if we just held the benchmark asset
	benchmarkValue := gBenchmarkQty.Mul(gBenchmark.LastPrice)

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

	// cagr calculation - compound at quantum intervals then annualize
	cagr := 0.0
	if duration > 0 && r.startEquity.IsPositive() {
		quantums := float64(duration) / float64(*flagQuantum)
		quantumsPerYear := float64(365*24*clocky.Hour) / float64(*flagQuantum)
		if quantums > 0 {
			totalReturn := endEquity.Div(r.startEquity).Float64()
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
	for _, h := range gInitialHoldings {
		fmt.Printf("start.%s %s\n", strings.ToLower(h.Symbol), h.Quantity)
	}

	// print end state
	fmt.Println()
	fmt.Printf("end %s\n", r.endTime)
	fmt.Printf("end.profit %s\n", endProfit)
	fmt.Printf("end.return %s\n", endReturn.MulInt(100))
	fmt.Printf("end.sharpe %.2f\n", gStrategyEquity.Sharpe(riskFreeRate))
	fmt.Printf("end.equity %s\n", endEquity)
	for _, exchange := range Exchanges.All() {
		for _, holding := range exchange.Holdings.All() {
			holding.Lock.RLock()
			qty := holding.Quantity
			holding.Lock.RUnlock()
			if qty.IsPositive() {
				fmt.Printf("end.%s %s\n", strings.ToLower(holding.Symbol), qty)
			}
		}
	}
	fmt.Printf("end.fees %s\n", fees)
	fmt.Printf("end.buys %d\n", r.trades[ds.SideBuy])
	fmt.Printf("end.sells %d\n", r.trades[ds.SideSell])
	fmt.Printf("end.trades %d\n", r.trades[ds.SideBuy]+r.trades[ds.SideSell])

	// aggregate win/loss counts across all non-cash holdings
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
	fmt.Printf("end.rebates %s\n", rebates)
	fmt.Printf("end.vol30day %s\n", vol30day.DivInt(1_000_000))
	fmt.Printf("end.cagr %.2f\n", cagr)
	fmt.Printf("end.maxdd %.2f\n", gStrategyEquity.MaxDrawdown()*100)

	// usd invested efficiency metrics
	fmt.Printf("invested.min %.0f\n", gStrategyInvested.Min())
	fmt.Printf("invested.max %.0f\n", gStrategyInvested.Max())
	fmt.Printf("invested.avg %.0f\n", gStrategyInvested.Avg())

	// print benchmark
	fmt.Println()
	fmt.Printf("bench.profit %s\n", benchProfit)
	fmt.Printf("bench.return %s\n", benchReturn.MulInt(100))
	fmt.Printf("bench.sharpe %.2f\n", gBenchmarkEquity.Sharpe(riskFreeRate))
	fmt.Printf("bench.equity %s\n", benchmarkValue)
	fmt.Printf("bench.%s %s\n", strings.ToLower(gBenchmark.BaseCurrency), gBenchmarkQty)
	fmt.Printf("bench.maxdd %.2f\n", gBenchmarkEquity.MaxDrawdown()*100)
}
