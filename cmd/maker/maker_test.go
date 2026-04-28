package main

import (
	"bytes"
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/golden"
	"dropbear/netty"
	"dropbear/symbol"
	"io"
	"log"
	"os"
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
	var logBuf bytes.Buffer
	log.SetOutput(io.MultiWriter(&logBuf, os.Stderr))
	defer log.SetOutput(os.Stderr)
	result := Run(symbols)
	result.Log = logBuf.String()
	return result
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
        ...VZ: pos=0[64]...realized=23[11]...unrealized=0[6]...fees=-2.38[3]...bought=1000[636]...sold=1000[700]
        ...NKE: pos=-500[300]...realized=-1.2[3]...unrealized=-8.3[4]...fees=-0.81[1]...bought=200[100]...sold=700[400]
        ...XLE: pos=400[99]...realized=2[4]...unrealized=2[3]...fees=-1.36[1]...bought=700[300]...sold=300[201]
        ...AAL: pos=-100[0]...realized=0[0]...unrealized=1.5[0]...fees=-0.13[0]...bought=0[0]...sold=100[0]
        ...INTC: pos=0[100]...realized=83.09[72]...unrealized=0[1]...fees=-15.68[4]...bought=10400[500]...sold=10400[600]
        ...HOOD: pos=-300[300]...realized=-13.21[62]...unrealized=-9.5[13]...fees=-5.23[2]...bought=4100[500]...sold=4400[800]
        ...DKNG: pos=-300[200]...realized=2[4]...unrealized=-7.5[11]...fees=-1.19[1]...bought=300[100]...sold=600[100]
        ...SOFI: pos=700[0]...realized=-31.59[10]...unrealized=-110.91[28]...fees=-2.13[0]...bought=1000[0]...sold=300[0]
        ...PYPL: pos=0[0]...realized=6.73[20]...unrealized=0[0]...fees=-2.1[1]...bought=900[100]...sold=900[100]
        ...RIVN: pos=-600[400]...realized=-3.25[6]...unrealized=-6.75[12]...fees=-0.97[1]...bought=200[100]...sold=800[300]
        ...CMCSA: pos=0[100]...realized=24.5[13]...unrealized=0[5]...fees=-1.84[2]...bought=800[200]...sold=800[300]
        ...TOTAL realized P&L: 92.061234[143]  fees: -33.817354[7]  net: 125.878588[136]
        ...total fills: 477[364]  symbols tracked: 12
`)
}
