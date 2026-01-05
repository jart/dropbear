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
	"os/signal"
	"runtime/pprof"
	"syscall"
)

var (
	FlagVerbose    = flag.Bool("v", false, "verbose logging")
	flagPaper      = flag.Bool("paper", false, "simulate order execution on live data")
	flagBacktest   = flag.Bool("backtest", false, "run backtest on historical data")
	flagStart      = clocky.TimeFlag("start", "1980-01-01", "backtest start date")
	flagEnd        = clocky.TimeFlag("end", "2099-12-31", "backtest end date")
	flagCPUProfile = flag.String("cpuprofile", "", "write cpu profile to file")
	flagRFR        = decimal.FlagBPS("rfr", "487", "annualized risk-free rate in basis points")
	flagQuantum    = clocky.DurationFlag("quantum", "1d", "metric sampling interval while backtesting")
	flagSlippage   = decimal.FlagPercent("slippage", "100", "VWAP deviation multiplier (100 = use full VWAP deviation)")
	flagImpact     = decimal.FlagPercent("impact", "50", "market impact multiplier (% of participation * range)")
	flagRekt       = decimal.Flag("rekt", "25_000", "portfolio value at which to consider the account liquidated")
	flagBuffer     = decimal.FlagPercent("buffer", "1", "percent of buying power to leave untapped")
)

var (
	Live      bool // true means we're in live trading or paper trading mode
	Paper     bool // true means orders are simulated
	Client    *alpaca.Client
	Running   bool
	Benchmark *Equity
	Cash      decimal.Decimal = decimal.FromInt(100_000)
	Hold      decimal.Decimal
)

var (
	gSigChan            chan os.Signal
	gMaxMarginAvailable decimal.Decimal
	gPowerLevel         decimal.Decimal
	gFeeCalculator      *alpaca.FeeCalculator
	gTotalSlippage      decimal.Decimal
	Liquidated          bool // true if portfolio value dropped to $25k or below
)

func Init() {
	Live = !*flagBacktest
	if Live {
		if *flagPaper {
			Paper = true
		}
		Client = alpaca.NewClient()
		err := Client.SyncAssets()
		if err != nil {
			panic(err)
		}
		gSigChan = make(chan os.Signal, 1)
		signal.Notify(gSigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)
		loggy.AlsoLogToFile()
		log.Printf("running %s", loggy.CommandLine())
	} else {
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
		defer Client.CancelAllOrders()
		for {
			if <-gSigChan == syscall.SIGUSR1 {
				// report.Print()
			} else {
				break
			}
		}
		log.Printf("goodbye")
	} else {
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
	// report.Print()
}

func GetPortfolioValue() decimal.Decimal {
	return Cash.Add(GetInvestedValue())
}

func GetInvestedValue() decimal.Decimal {
	total := decimal.Zero
	for _, equity := range Equities {
		total = total.Add(equity.Price.Mul(equity.Quantity.Abs()))
	}
	return total
}

func GetMarginUsed() decimal.Decimal {
	total := decimal.Zero
	for _, equity := range Equities {
		margin := equity.Asset.GetMaintenanceMargin(equity.Quantity, equity.Price)
		total = total.Add(margin)
	}
	return total
}

func onRunEnd() {
	Running = false
}
