package main

import (
	"dropbear/black76"
	"dropbear/broker/alpaca"
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
	kExpectedPriceRange       = decimal.Parse("0.15")
	kRiskFreeRate             = decimal.Parse("0.04")
	kBrokerFeePerTrade        = decimal.Parse("0.0025") // alpaca elite smart router (first tier)
	kCatFeePerTrade           = decimal.Parse("0.0003")
	kTafFeePerShare           = decimal.Parse("0.000195")
	kExchangeTakerFeePerShare = decimal.Parse("0.0020")
	kExchangeMakerFeePerShare = decimal.Parse("-0.0018")
	kOptionsFeeTAF            = decimal.Parse("0.00329") // per contract, sells only
	kOptionsFeeORF            = decimal.Parse("0.02295") // per contract
	kOptionsFeeOCC            = decimal.Parse("0.025")   // per contract
	kOptionsFeeSchwab         = decimal.Parse("0.65")    // per contract
	kOptionsFeeSchwabCBOE     = decimal.Parse("1.2")     // per contract
)

var (
	liveFlag      = flag.Bool("live", false, "run in live trading mode")
	dryFlag       = flag.Bool("dry", false, "don't send new orders in live mode")
	maxErrorFlag  = decimal.Flag("max-error", "1", "maximum acceptable accounting error before panic")
	latencyFlag   = clocky.DurationFlag("latency", "10ms", "simulated order latency")
	heartbeatFlag = clocky.DurationFlag("heartbeat", "1m", "interval between status reports")
)

var (
	gSchwabClient *schwab.Client
	gAlpacaClient *alpaca.Client
)

func main() {
	loggy.Init()

	black76.DaysPerYear = 252
	black76.HoursPerDay = 6.5

	dbnFlag := flag.String("dbn", "", "path to backtest data")
	symbolFlag := symbol.Flag("symbol", "", "symbol to trade (e.g. NVDA)")
	strikesFlag := flag.Int("strikes", 2, "how many strikes wide strangle should be")
	dateFlag := clocky.TimeFlag("date", "", "date of the trades")
	schwabFlag := flag.Bool("schwab", false, "use schwab as broker instead of alpaca")
	toleranceFlag := decimal.Flag("tolerance", "-1", "delta tolerance for market making")
	directionFlag := decimal.Flag("direction", "-1", "direction of options trade (1=long, -1=short)")
	straddlesFlag := decimal.Flag("straddles", "5", "number of ATM straddles to buy at open")
	quantumFlag := decimal.Flag("quantum", "40", "share lot size for delta hedging")
	patienceFlag := clocky.DurationFlag("patience", "5s", "how long to wait for order execution")
	spreadFlag := decimal.Flag("spread", "0", "spread crossing for straddle (-1=make, 0=mid, 1=take)")
	startOfDayFlag := flag.Int("sod", 9_35_00, "start of day in HHMMSS")
	flagCPUProfile := flag.String("cpuprofile", "", "write cpu profile to file")

	newConfig := func() *Config {
		return &Config{
			Symbol:     *symbolFlag,
			Direction:  *directionFlag,
			Straddles:  *straddlesFlag,
			Quantum:    *quantumFlag,
			Spread:     *spreadFlag,
			Patience:   *patienceFlag,
			StartOfDay: *startOfDayFlag,
			Strikes:    *strikesFlag,
			Schwab:     *schwabFlag,
			Tolerance:  *toleranceFlag,
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

	// backtesting
	if !*liveFlag {
		t := NewTrader(newConfig())
		err := t.Backtest(*dbnFlag, *dateFlag)
		if err != nil {
			log.Fatalf("backtest error: %v", err)
		}
		return
	}

	// production
	var t *Trader
	var traders []*Trader
	loggy.AlsoLogToFile()
	log.Printf("prepare to be strangled")

	t = NewTrader(newConfig())
	t.Config.Symbol = symbol.NVDA
	t.Config.Tolerance = decimal.Parse("-4")
	t.Config.Patience = 30 * clocky.Second
	traders = append(traders, t)

	t = NewTrader(newConfig())
	t.Config.Symbol = symbol.TSLA
	t.Config.Strikes = 3
	t.Config.Tolerance = decimal.Parse("-4")
	t.Config.Patience = 30 * clocky.Second
	traders = append(traders, t)

	// t = NewTrader(newConfig())
	// t.Config.Symbol = symbol.AMZN
	// t.Config.Strikes = 3
	// t.Config.Tolerance = decimal.Parse("-4")
	// t.Config.Patience = 30 * clocky.Second
	// traders = append(traders, t)

	// t = NewTrader(newConfig())
	// t.Config.Symbol = symbol.MSFT
	// t.Config.Direction = decimal.One
	// t.Config.Tolerance = decimal.Parse("-3")
	// t.Config.Patience = 30 * clocky.Second
	// traders = append(traders, t)

	t = NewTrader(newConfig())
	t.Config.Symbol = symbol.AAPL
	t.Config.Strikes = 2
	t.Config.Quantum = decimal.Ten
	t.Config.Direction = decimal.One
	t.Config.Tolerance = decimal.Parse("-4")
	t.Config.Patience = 30 * clocky.Second
	traders = append(traders, t)

	// figure out what brokers we need
	needSchwab := false
	needAlpaca := false
	for _, t := range traders {
		if t.Config.Schwab {
			needSchwab = true
		} else {
			needAlpaca = true
		}
	}

	// subscribe to schwab order updates
	// they only let us have one connection
	if needSchwab {
		gSchwabClient = schwab.NewClient()
		orderUpdates := gSchwabClient.OrderUpdates()
		go fanoutSchwabOrderUpdates(orderUpdates, traders)
	}

	// subscribe to alpaca order updates
	if needAlpaca {
		gAlpacaClient = alpaca.NewClient()
		orderUpdates := alpaca.OrderUpdates()
		go fanoutAlpacaOrderUpdates(orderUpdates, traders)
	}

	// start trading
	for _, t := range traders {
		if t.Config.Schwab {
			go t.LiveSchwab()
		} else {
			go t.LiveAlpaca()
		}
	}

	// wait forever
	select {}
}

func fanoutSchwabOrderUpdates(orderEvents <-chan *schwab.OrderEvent, traders []*Trader) {
	for orderEvent := range orderEvents {
		for _, trader := range traders {
			trader.OrderEventsSchwab <- orderEvent
		}
	}
}

func fanoutAlpacaOrderUpdates(orderEvents <-chan *alpaca.OrderUpdate, traders []*Trader) {
	for orderEvent := range orderEvents {
		for _, trader := range traders {
			trader.OrderEventsAlpaca <- orderEvent
		}
	}
}
