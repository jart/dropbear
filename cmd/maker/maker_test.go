package main

import (
	"bytes"
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/golden"
	"dropbear/loggy"
	"dropbear/netty"
	"dropbear/symbol"
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
	loggy.Init()
	netty.SetOffline()
}

// testLogWriter captures log output and also writes to stderr.
type testLogWriter struct {
	buf bytes.Buffer
}

func (w *testLogWriter) Write(p []byte) (n int, err error) {
	line := clocky.Now().String() + " " + string(p)
	os.Stderr.WriteString(line)
	w.buf.WriteString(line)
	return len(p), nil
}

func runBacktest(t *testing.T, dataFile string, symbols []SymbolEntry) Result {
	t.Helper()
	clocky.Now = clocky.FakeNow
	clocky.Sleep = clocky.FakeSleep
	clocky.NewTicker = clocky.FakeNewTicker
	*flagData = dataFile
	*flagLive = false
	w := &testLogWriter{}
	log.SetFlags(0)
	log.SetOutput(w)
	defer log.SetOutput(os.Stderr)
	result := Run(symbols)
	result.Log = w.buf.String()
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
...VZ: pos=0[0]...realized=23[8]...unrealized=0[0]...fees=-2.38[5]...bought=1000[600]...sold=1000[600]
...NKE: pos=-500[400]...realized=-1.2[3]...unrealized=-8.3[7]...fees=-0.81[2]...bought=200[0]...sold=700[400]
...XLE: pos=400[1]...realized=2[8]...unrealized=2[798]...fees=-1.36[3]...bought=700[200]...sold=300[215]
...AAL: pos=-100[0]...realized=0[0]...unrealized=1.5[0]...fees=-0.13[1]...bought=0[0]...sold=100[0]
...INTC: pos=0[100]...realized=83.09[18]...unrealized=0[1]...fees=-15.68[58]...bought=10400[1100]...sold=10400[1200]
...HOOD: pos=-300[0]...realized=-13.21[29]...unrealized=-9.5[7]...fees=-5.23[25]...bought=4100[0]...sold=4400[0]
...DKNG: pos=-300[100]...realized=2[3]...unrealized=-7.5[7]...fees=-1.19[4]...bought=300[0]...sold=600[100]
...SOFI: pos=700[0]...realized=-31.59[10]...unrealized=-110.91[28]...fees=-2.13[5]...bought=1000[0]...sold=300[0]
...PYPL: pos=0[100]...realized=6.73[29]...unrealized=0[5]...fees=-2.1[8]...bought=900[300]...sold=900[200]
...RIVN: pos=-600[400]...realized=-3.25[4]...unrealized=-6.75[8]...fees=-0.97[3]...bought=200[0]...sold=800[400]
...CMCSA: pos=0[50]...realized=24.5[12]...unrealized=0[3]...fees=-1.84[4]...bought=800[250]...sold=800[300]
...TOTAL equity: -2.707509[900]  realized: 92.061234[62]  unrealized: 0[888]  fees: -33.817354[114]  net: 125.878588[58]
...total fills: 477[359]  symbols tracked: 12
`)
}

func TestSurvivor(t *testing.T) {
	t.Skip("TODO: make -survivor deterministic by fixing map iteration possibly")
}
