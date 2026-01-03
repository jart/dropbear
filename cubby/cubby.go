package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"
)

var (
	flagPaper      = flag.Bool("paper", false, "simulate order execution on live data")
	flagBacktest   = flag.Bool("backtest", false, "run backtest on historical data")
	flagStart      = clocky.TimeFlag("start", "2016-01-01", "backtest start date")
	flagEnd        = clocky.TimeFlag("end", "2099-12-31", "backtest end date")
	flagCPUProfile = flag.String("cpuprofile", "", "write cpu profile to file")
	flagRFR        = decimal.FlagBPS("rfr", "487", "annualized risk-free rate in basis points")
	flagQuantum    = clocky.DurationFlag("quantum", "1d", "metric sampling interval while backtesting")
	flagVerbose    = flag.Bool("cubby-verbose", false, "log order simulation decisions")
	flagMargin     = flag.Int("margin", 1, "day trading buying power multiplier (4 for PDT)")
	flagPDT        = flag.Bool("pdt", true, "enable pattern day trader mode (4x intraday leverage)")
)

var (
	Live         bool // true means we're in live trading or paper trading mode
	Paper        bool // true means orders are simulated
	AlpacaClient *alpaca.Client
	Running      bool
	Benchmark    *Equity
	gSigChan     chan os.Signal
	Cash         decimal.Decimal = decimal.FromInt(100_000)
)

func Init() {
	Live = !*flagBacktest
	if Live {
		if *flagPaper {
			Paper = true
		}
		AlpacaClient = alpaca.NewClient()
		err := AlpacaClient.SyncAssets()
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
		ds.SetOffline()
		clocky.SetLive(false)
		clocky.Now = clocky.FakeNow
	}
}

// Run runs the main event loop.
func Run() {
	Running = true
	defer onRunEnd()
	if Benchmark == nil {
		panic("you forgot to set cubby.Benchmark")
	}
	report := newReport(Benchmark)
	if Live {
		defer AlpacaClient.CancelAllOrders()
		for {
			if <-gSigChan == syscall.SIGUSR1 {
				report.Print()
			} else {
				break
			}
		}
		log.Printf("goodbye")
	} else {
		manager := newManager(report)
		for _, broker := range Brokers.All() {
			for _, eq := range broker.Equities.All() {
				manager.Register(eq)
			}
		}
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
		manager.Run()
	}
	report.Print()
}

func onRunEnd() {
	Running = false
}
