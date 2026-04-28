package main

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/golden"
	"dropbear/netty"
	"dropbear/symbol"
	"testing"
)

// symbols20260428 is the portfolio from the 2026-04-28 live session.
// Live result: realized=$92, fees=-$34, net=$126, 477 fills, 39900 shares.
var symbols20260428 = []SymbolEntry{
	{symbol.INTC, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.PYPL, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("200"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.CMCSA, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.SOFI, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("400"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.HOOD, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.DKNG, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-400"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.RIVN, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-500"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.AAL, Config{
		venue:  alpaca.OrderDestinationNASDAQ,
		target: decimal.Parse("-700"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.XLE, Config{
		venue:  alpaca.OrderDestinationARCA,
		target: decimal.Parse("300"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.FXI, Config{
		venue:  alpaca.OrderDestinationARCA,
		target: decimal.Parse("-500"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.VZ, Config{
		venue:  alpaca.OrderDestinationNYSE,
		target: decimal.Parse("400"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
	{symbol.NKE, Config{
		venue:  alpaca.OrderDestinationNYSE,
		target: decimal.Parse("-400"),
		qty:    decimal.Parse("100"),
		spread: decimal.Parse("0.02"),
		drift:  decimal.Parse("0.02"),
	}},
}

func init() {
	netty.SetOffline()
}

func runBacktest(t *testing.T, dataFile string, symbols []SymbolEntry) Result {
	t.Helper()
	clocky.Now = clocky.FakeNow
	clocky.Sleep = clocky.FakeSleep
	clocky.NewTicker = clocky.FakeNewTicker
	*flagData = dataFile
	*flagLive = false
	return Run(symbols)
}

// go test ./cmd/maker/ -v -run TestBacktest20260428
//
// Golden values are from the 2026-04-28 live production run.
// Tolerances reflect how far the backtest fill model may diverge from reality.
// As fill modeling improves, tighten these toward zero.
func TestBacktest20260428(t *testing.T) {
	r := runBacktest(t, "data/2026-04-28T15:41:40.531844838.sip", symbols20260428)
	// Golden values are from the 2026-04-28 live production run.
	// Tolerances are wide because fill modeling is approximate.
	// Tighten these as the backtest engine improves.
	golden.Check(t, r.Log, `
...=== P&L SUMMARY ===
...VZ: pos=0[200]...realized=23.00[15]...unrealized=0.00[10]...fees=-2.38[7]...bought=1000[700]...sold=1000[700]
...NKE: pos=-500[300]...realized=-1.20[5]...unrealized=-8.30[15]...fees=-0.81[2]...bought=200[200]...sold=700[400]
...XLE: pos=400[200]...realized=2.00[5]...unrealized=2.00[10]...fees=-1.36[2]...bought=700[400]...sold=300[300]
...AAL: pos=-100[100]...realized=0.00[5]...unrealized=1.50[5]...fees=-0.13[2]...bought=0[200]...sold=100[200]
...INTC: pos=0[200]...realized=83.09[50]...unrealized=0.00[20]...fees=-15.68[15]...bought=10400[2000]...sold=10400[2000]
...HOOD: pos=-300[300]...realized=-13.21[60]...unrealized=-9.50[30]...fees=-5.23[10]...bought=4100[2000]...sold=4400[2000]
...DKNG: pos=-300[200]...realized=2.00[5]...unrealized=-7.50[15]...fees=-1.19[3]...bought=300[300]...sold=600[400]
...SOFI: pos=700[300]...realized=-31.59[10]...unrealized=-110.91[80]...fees=-2.13[3]...bought=1000[600]...sold=300[400]
...PYPL: pos=0[100]...realized=6.73[25]...unrealized=0.00[5]...fees=-2.10[3]...bought=900[500]...sold=900[500]
...RIVN: pos=-600[400]...realized=-3.25[10]...unrealized=-6.75[15]...fees=-0.97[2]...bought=200[300]...sold=800[500]
...CMCSA: pos=0[200]...realized=24.50[15]...unrealized=0.00[10]...fees=-1.84[3]...bought=800[500]...sold=800[500]
...TOTAL realized P&L: 92.061234[100]  fees: -33.817354[30]  net: 125.878588[120]
...total fills: 477[600]  symbols tracked: 12
`)
}
