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
	"maps"
	"os"
	"os/signal"
	"runtime/pprof"
	"strings"
)

var (
	liveFlag      = flag.Bool("live", false, "run in live trading mode")
	rtFlag        = flag.Bool("rt", false, "run backtest in real time mode")
	dryFlag       = flag.Bool("dry", false, "don't send new orders in live mode")
	hostileFlag   = flag.Bool("hostile", false, "simulate maximum hostility on fills")
	maxErrorFlag  = decimal.Flag("max-error", "1", "maximum acceptable accounting error before panic")
	slowdownFlag  = clocky.DurationFlag("slowdown", "200ms", "polling limit for web dashboard")
	heartbeatFlag = clocky.DurationFlag("heartbeat", "1m", "interval between status reports")
)

const (
	kMultiplier = 100
)

var (
	kFeePerContract     = decimal.Parse("1.2")
	kExpectedPriceRange = decimal.Parse("0.15")
	kRiskFreeRate       = decimal.Parse("0.035")
)

var (
	gSchwabClient *schwab.Client
)

func main() {
	loggy.Init()

	// parse flags
	webFlag := flag.Bool("web", false, "enable web dashboard feature")
	noneFlag := flag.Bool("none", false, "disable all strategies")
	closingFlag := flag.Bool("closing", false, "allow closing overlapping positions")
	dbnFlag := flag.String("dbn", "", "path to backtest data")
	symbolFlag := flag.String("symbol", "", "symbol to trade (e.g. XSP, SPXW)")
	dateFlag := clocky.TimeFlag("date", "", "date of the trades to report")
	sigmasFlag := decimal.Flag("sigmas", "2.5", "number of sigmas of strikes to consider")
	berthFlag := decimal.Flag("berth", "0.15", "percent of strikes to consider for box arbitrage")
	budgetFlag := flag.Float64("budget", 2_000, "maximum acceptable loss at current price")
	floorFlag := flag.Float64("floor", 10_000, "maximum acceptable loss in catastrophic scenario")
	spreadFlag := decimal.Flag("spread", "1", "spread crossing (-1=make, 0=mid, 1=take)")
	evalFlag := decimal.Flag("eval", "2", "spread aggressiveness for scoring (0=same as -spread)")
	minProfitFlag := decimal.Flag("min-profit", "5", "minimum structural edge for statistical arbitrage")
	thinkFlag := clocky.DurationFlag("think", "50ms", "interval between backtest trading analysis")
	patienceFlag := clocky.DurationFlag("patience", "30s", "how long to wait before canceling live orders")
	cooldownFlag := clocky.DurationFlag("cooldown", "30s", "interval between trading decisions")
	maxPendingFlag := flag.Int("max-pending", 1, "maximum number of pending orders")
	flagCPUProfile := flag.String("cpuprofile", "", "write cpu profile to file")
	startOfDayFlag := flag.Int("sod", 9_30_05, "start of day in HHMMSS")
	fullRiskTimeFlag := flag.Int("frt", 12_00_00, "full risk time in HHMMSS")
	stopTradingFlag := flag.Int("eodt", 13_00_00, "stop trading / begin EOD liquidation in HHMMSS")
	panicFlag := clocky.DurationFlag("panic", "0", "time before close to panic close positions")
	flag.Parse()

	// log what flags were passed
	log.Println("got flags " + strings.Join(os.Args, " "))

	// configure math
	// this is how thinkorswim computes greeks
	black76.DaysPerYear = 252
	black76.HoursPerDay = 6.5

	// configure strategies
	bts := map[string]bool{}
	for _, k := range kStrategies {
		bts[k] = false
	}
	if !*noneFlag {
		maps.Copy(bts, kStrategyDefault)
	}

	// support cpu profiling
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

	// use flags as default trader configuration
	newConfig := func() *Config {
		c := &Config{
			Eval:         *evalFlag,
			Spread:       *spreadFlag,
			Sigmas:       *sigmasFlag,
			Cooldown:     *cooldownFlag,
			Patience:     *patienceFlag,
			StartOfDay:   *startOfDayFlag,
			FullRiskTime: *fullRiskTimeFlag,
			StopTrading:  *stopTradingFlag,
			MaxPending:   *maxPendingFlag,
			MinProfit:    *minProfitFlag,
			Strategies:   map[string]bool{},
			AllowClosing: *closingFlag,
			Panic:        *panicFlag,
			Floor:        *floorFlag,
			Budget:       *budgetFlag,
			Berth:        *berthFlag,
			Think:        *thinkFlag,
		}
		for _, s := range kStrategies {
			c.Strategies[s] = false
		}
		return c
	}

	// run backtest when not in live mode
	if !*liveFlag {
		t := NewTrader(symbol.MustParse(*symbolFlag), newConfig())
		t.Config.Strategies = bts
		if *webFlag {
			t.Web = NewWeb()
			t.Web.Start(t)
		}
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
	log.Printf("volt is on the prowl")

	// SPXW strategy
	t = NewTrader(symbol.MustParse("SPXW"), newConfig())
	t.Config.Listen = "127.0.0.1:8484"
	t.Config.Floor = 10_000
	t.Config.Budget = 2_000
	t.Config.Eval = decimal.Ten
	t.Config.Sigmas = decimal.One
	t.Config.Spread = decimal.NegOne
	t.Config.StartOfDay = 10_00_00
	traders = append(traders, t)

	// subscribe to schwab order updates
	// they only let us have one connection
	gSchwabClient = schwab.NewClient()
	orderUpdates := gSchwabClient.OrderUpdates()
	go fanoutSchwabOrderUpdates(orderUpdates, traders)

	// start web server for each trader
	for _, t := range traders {
		t.Web = NewWeb()
		if *webFlag {
			t.Web.Start(t)
		}
	}

	// start trading
	for _, t := range traders {
		go t.Live()
	}

	// wait forever
	select {}
}

func fanoutSchwabOrderUpdates(orderUpdates <-chan *schwab.OrderEvent, traders []*Trader) {
	for orderEvent := range orderUpdates {
		for _, trader := range traders {
			trader.OrderEvents <- orderEvent
		}
	}
}
