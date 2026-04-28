package main

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
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
	result := Run(symbols)
	t.Logf("pnl=%s fees=%s net=%s fills=%d shares=%s",
		result.PnL, result.Fees, result.Net, result.Fills, result.Shares)
	return result
}

// go test ./cmd/maker/ -v -run TestBacktest20260428
func TestBacktest20260428(t *testing.T) {
	r := runBacktest(t, "data/2026-04-28T15:41:40.531844838.sip", symbols20260428)
	wantFills := 851
	wantShares := decimal.Parse("38063")
	wantNet := decimal.Parse("216.269265") // 125.878588
	tolerance := decimal.Parse("1")
	if r.Fills != wantFills {
		t.Errorf("fills: got %d, want %d", r.Fills, wantFills)
	}
	if r.Shares.Cmp(wantShares) != 0 {
		t.Errorf("shares: got %s, want %s", r.Shares, wantShares)
	}
	if r.Net.Sub(wantNet).Abs().Cmp(tolerance) > 0 {
		t.Errorf("net: got %s, want %s (±%s)", r.Net, wantNet, tolerance)
	}
}
