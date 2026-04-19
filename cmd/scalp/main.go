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
	kExpectedPriceRange        = decimal.Parse("0.15")
	kRiskFreeRate              = decimal.Parse("0.04")
	kFeePerOptionsContract     = decimal.Parse("0.65")
	kFeePerOptionsContractSPXW = decimal.Parse("1.2")
	kBrokerFeePerTrade         = decimal.Parse("0.0025")
	kCatFeePerTrade            = decimal.Parse("0.0003")
	kTafFeePerShare            = decimal.Parse("0.0002")
	kExchangeTakerFeePerShare  = decimal.Parse("0.0020")
	kExchangeMakerFeePerShare  = decimal.Parse("-0.0018")
)

var (
	liveFlag      = flag.Bool("live", false, "run in live trading mode")
	dryFlag       = flag.Bool("dry", false, "don't send new orders in live mode")
	hostileFlag   = flag.Bool("hostile", false, "simulate maximum hostility on fills")
	maxErrorFlag  = decimal.Flag("max-error", "1", "maximum acceptable accounting error before panic")
	latencyFlag   = clocky.DurationFlag("latency", "10ms", "simulated order latency")
	heartbeatFlag = clocky.DurationFlag("heartbeat", "1m", "interval between status reports")
)

var gSchwabClient *schwab.Client

func main() {
	loggy.Init()

	black76.DaysPerYear = 252
	black76.HoursPerDay = 6.5

	dbnFlag := flag.String("dbn", "", "path to backtest data")
	symbolFlag := symbol.Flag("symbol", "", "symbol to trade (e.g. NVDA)")
	strikesFlag := flag.Int("strikes", 0, "how many strikes wide strangle should be")
	dateFlag := clocky.TimeFlag("date", "", "date of the trades")
	directionFlag := decimal.Flag("direction", "1", "direction of options trade (1=long, -1=short)")
	straddlesFlag := decimal.Flag("straddles", "5", "number of ATM straddles to buy at open")
	quantumFlag := decimal.Flag("quantum", "100", "share lot size for delta hedging")
	spreadFlag := decimal.Flag("spread", "1", "spread crossing for straddle (-1=make, 0=mid, 1=take)")
	startOfDayFlag := flag.Int("sod", 9_30_05, "start of day in HHMMSS")
	flagCPUProfile := flag.String("cpuprofile", "", "write cpu profile to file")

	newConfig := func() *Config {
		return &Config{
			Symbol:     *symbolFlag,
			Direction:  *directionFlag,
			Straddles:  *straddlesFlag,
			Quantum:    *quantumFlag,
			Spread:     *spreadFlag,
			StartOfDay: *startOfDayFlag,
			Strikes:    *strikesFlag,
		}
	}

	flag.Parse()
	log.Println("got flags " + strings.Join(os.Args, " "))

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

	if !*liveFlag {
		t := NewTrader(newConfig())
		err := t.Backtest(*dbnFlag, *dateFlag)
		if err != nil {
			log.Fatalf("backtest error: %v", err)
		}
		return
	}

	log.Fatal("live mode not yet implemented for scalp")
}

func fanoutSchwabOrderUpdates(orderEvents <-chan *schwab.OrderEvent, traders []*Trader) {
	for orderEvent := range orderEvents {
		for _, trader := range traders {
			trader.OrderEvents <- orderEvent
		}
	}
}
