package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/loggy"
	"dropbear/netty"
	"flag"
	"log"
	"os"
	"runtime/pprof"
)

var (
	flagVerbose    = flag.Bool("v", false, "verbose logging")
	flagPaper      = flag.Bool("paper", false, "simulate order execution on live data")
	flagBacktest   = flag.Bool("backtest", false, "run backtest on historical data")
	flagCash       = decimal.Flag("cash", "100_000", "initial USD balance")
	flagStart      = clocky.TimeFlag("start", "1980-01-01", "backtest start date")
	flagEnd        = clocky.TimeFlag("end", "2099-12-31", "backtest end date")
	flagCPUProfile = flag.String("cpuprofile", "", "write cpu profile to file")
	flagRFR        = decimal.FlagBPS("rfr", "487", "annualized risk-free rate in basis points")
	FlagQuantum    = clocky.DurationFlag("quantum", "1d", "metric sampling interval while backtesting")
	flagSlippage   = decimal.FlagPercent("slippage", "100", "VWAP deviation multiplier (100 = use full VWAP deviation)")
	flagImpact     = decimal.FlagPercent("impact", "50", "market impact multiplier (% of participation * range)")
	flagRekt       = decimal.Flag("rekt", "25_000", "portfolio value at which to consider the account liquidated")
	FlagBuffer     = decimal.FlagPercent("buffer", "1", "percent of buying power to leave untapped")
	FlagVWAP       = decimal.FlagPercent("vwap", "20", "percent of minute volume we can take")
	FlagPatience   = clocky.DurationFlag("patience", "15m", "time to wait for order fills")
)

var (
	Live        bool // true means we're in live trading or paper trading mode
	Paper       bool // true means orders are simulated
	Verbose     bool // true means verbose logging is enabled
	IsWarmingUp bool // true means we're backfilling historical data
	Liquidated  bool // true if portfolio value dropped to $25k or below
	Client      *alpaca.Client
	Running     bool
	Benchmark   *Equity
	Cash        decimal.Decimal
)

var (
	gMaxMarginAvailable decimal.Decimal
	gPowerLevel         decimal.Decimal
	gMarginHold         decimal.Decimal // margin reserved by pending orders
	gFeeCalculator      *alpaca.FeeCalculator
	gLiveInvested       decimal.Decimal
	gLiveMarginUsed     decimal.Decimal
)

func Init() {
	Live = !*flagBacktest
	if Live {
		Verbose = true // life is slow, so don't keep secrets
		if *flagPaper {
			Paper = true
		}
		Client = alpaca.NewClient()
		err := Client.SyncAssets()
		if err != nil {
			panic(err)
		}
		loggy.AlsoLogToFile()
	} else {
		Verbose = *flagVerbose // backtests are fast, so avoid spam
		if *flagPaper {
			panic("-paper is implied by -backtest")
		}
		Paper = true
		netty.SetOffline()
		clocky.Now = clocky.FakeNow
		clocky.Sleep = clocky.FakeSleep
		clocky.NewTicker = clocky.FakeNewTicker
	}
}

// Run runs the main event loop.
func Run() {
	Running = true
	defer onRunEnd()
	if Benchmark == nil {
		panic("you forgot to set cubby.Benchmark")
	}
	gFeeCalculator = alpaca.NewFeeCalculator()
	if Live {
		liveTrader := newLiveTrader()
		defer liveTrader.Close()
		liveTrader.Run()
	} else {
		Cash = *flagCash
		backtest := newBacktest()
		defer backtest.Close()
		if *flagCPUProfile != "" {
			f, err := os.Create(*flagCPUProfile)
			if err != nil {
				log.Fatalf("could not create CPU profile: %v", err)
			}
			defer f.Close()
			if err := pprof.StartCPUProfile(f); err != nil {
				log.Fatalf("could not start CPU profile: %v", err)
			}
			defer pprof.StopCPUProfile()
		}
		backtest.Run()
	}
}

func GetPortfolioValue() decimal.Decimal {
	return Cash.Add(GetInvestedValue())
}

func GetInvestedValue() decimal.Decimal {
	if !gLiveInvested.IsZero() {
		return gLiveInvested
	}
	total := decimal.Zero
	for _, equity := range Equities {
		if equity.Quantity.IsZero() {
			continue
		}
		if !equity.Price.IsPositive() {
			loggy.Fatalf("can't compute GetInvestedValue() because %s has non-positive price %s", equity.Symbol, equity.Price)
		}
		total = total.Add(equity.Price.Mul(equity.Quantity.Abs()))
	}
	return total
}

func GetMarginUsed() decimal.Decimal {
	if !gLiveMarginUsed.IsZero() {
		return gLiveMarginUsed
	}
	total := decimal.Zero
	for _, equity := range Equities {
		if equity.Quantity.IsZero() {
			continue
		}
		if !equity.Price.IsPositive() {
			loggy.Fatalf("can't compute GetMarginUsed() because %s has non-positive price %s", equity.Symbol, equity.Price)
		}
		margin := equity.Asset.GetMaintenanceMargin(equity.Quantity, equity.Price)
		total = total.Add(margin)
	}
	return total
}

func onRunEnd() {
	Running = false
}
