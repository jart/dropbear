package main

import (
	"dropbear/black76"
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/loggy"
	"dropbear/symbol"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
)

var (
	liveFlag      = flag.Bool("live", false, "run in live trading mode")
	dryFlag       = flag.Bool("dry", false, "don't send new orders in live mode")
	hostileFlag   = flag.Bool("hostile", false, "simulate maximum hostility on fills")
	maxErrorFlag  = decimal.Flag("max-error", "1", "maximum acceptable accounting error before panic")
	heartbeatFlag = clocky.DurationFlag("heartbeat", "1m", "interval between status reports")
)

var (
	kFeePerContract     = decimal.Parse("1.2")
	kExpectedPriceRange = decimal.Parse("0.15")
	kRiskFreeRate       = decimal.Parse("0.035")
)

var gSchwabClient *schwab.Client

func main() {
	loggy.Init()

	dbnFlag := flag.String("dbn", "", "path to backtest data")
	symbolFlag := flag.String("symbol", "", "symbol to trade (e.g. NVDA)")
	dateFlag := clocky.TimeFlag("date", "", "date of the trades")
	straddlesFlag := decimal.Flag("straddles", "1", "number of ATM straddles to buy at open")
	quantumFlag := decimal.Flag("quantum", "10", "share lot size for delta hedging")
	spreadFlag := decimal.Flag("spread", "1", "spread crossing for straddle (-1=make, 0=mid, 1=take)")
	thinkFlag := clocky.DurationFlag("think", "50ms", "interval between trading analysis")
	startOfDayFlag := flag.Int("sod", 9_30_05, "start of day in HHMMSS")
	flagCPUProfile := flag.String("cpuprofile", "", "write cpu profile to file")
	flag.Parse()

	log.Println("got flags " + strings.Join(os.Args, " "))

	black76.DaysPerYear = 252
	black76.HoursPerDay = 6.5

	if *flagCPUProfile != "" {
		f, err := os.Create(*flagCPUProfile)
		if err != nil {
			log.Fatalf("could not create CPU profile: %v", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatalf("could not start CPU profile: %v", err)
		}
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		go func() {
			<-c
			pprof.StopCPUProfile()
			f.Close()
			os.Exit(0)
		}()
		defer pprof.StopCPUProfile()
		defer f.Close()
	}

	if *liveFlag {
		log.Fatal("live mode not yet implemented for scalp")
	}

	t := NewTrader(symbol.MustParse(*symbolFlag), &Config{
		Straddles:  *straddlesFlag,
		Quantum:    *quantumFlag,
		Spread:     *spreadFlag,
		Think:      *thinkFlag,
		StartOfDay: *startOfDayFlag,
	})
	err := t.Backtest(*dbnFlag, *dateFlag)
	if err != nil {
		log.Fatalf("backtest error: %v", err)
	}
}

func fanoutSchwabOrderUpdates(orderEvents <-chan *schwab.OrderEvent, traders []*Trader) {
	for orderEvent := range orderEvents {
		for _, trader := range traders {
			trader.OrderEvents <- orderEvent
		}
	}
}
