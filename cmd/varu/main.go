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
	allFlag := flag.Bool("all", false, "enable all strategies")
	noneFlag := flag.Bool("none", false, "disable all strategies")
	noliqFlag := flag.Bool("noliq", false, "disable liquidation strategies")
	comboFlag := flag.Bool("combo", false, "enable buying and selling combos")
	bearishFlag := flag.Bool("bearish", false, "only sell call verticals (bearish bias)")
	bullishFlag := flag.Bool("bullish", false, "only sell put verticals (bullish bias)")
	dbnFlag := flag.String("dbn", "", "path to backtest data")
	symbolFlag := flag.String("symbol", "", "symbol to trade (e.g. XSP, SPXW)")
	dateFlag := clocky.TimeFlag("date", "", "date of the trades to report")
	sigmasFlag := decimal.Flag("sigmas", "2.5", "number of sigmas of strikes to consider")
	budgetFlag := flag.Float64("budget", 5_000, "maximum acceptable loss at current price")
	floorFlag := flag.Float64("floor", 40_000, "maximum acceptable loss in catastrophic scenario")
	spreadFlag := decimal.Flag("spread", "0", "spread crossing (-1=make, 0=mid, 1=take)")
	evalFlag := decimal.Flag("eval", "2", "spread aggressiveness for scoring (0=same as -spread)")
	pruneFlag := flag.Float64("prune", .5, "probability of stochastic pruning in strategy search")
	demandFlag := decimal.Flag("demand", "1.1", "minimum ratio of open credit to close cost for liquidation")
	wPayoffFlag := decimal.Flag("w-payoff", "1", "scoring weight for expected payoff improvement")
	wRiskFlag := decimal.Flag("w-risk", "1", "scoring weight for risk reduction")
	wDeltaFlag := decimal.Flag("w-delta", "1", "scoring weight for delta neutrality improvement")
	thinkFlag := clocky.DurationFlag("think", "50ms", "interval between backtest trading analysis")
	patienceFlag := clocky.DurationFlag("patience", "30s", "how long to wait before canceling live orders")
	cooldownFlag := clocky.DurationFlag("cooldown", "30s", "interval between trading decisions")
	maxPendingFlag := flag.Int("max-pending", 1, "maximum number of pending orders")
	flagCPUProfile := flag.String("cpuprofile", "", "write cpu profile to file")
	startOfDayFlag := flag.Int("sod", 9_30_05, "start of day in HHMMSS")
	fullRiskTimeFlag := flag.Int("frt", 12_00_00, "full risk time in HHMMSS")
	stopTradingFlag := flag.Int("eodt", 13_00_00, "stop trading / begin EOD liquidation in HHMMSS")
	dumpFlag := clocky.DurationFlag("dump", "0", "time before close to close positions")
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
	if *comboFlag {
		bts[kStrategyBuyCombo] = true
		bts[kStrategySellCombo] = true
		kStrategyDefaultEOD[kStrategyLiquidateCall] = true
		kStrategyDefaultEOD[kStrategyLiquidatePut] = true
	}
	if *allFlag {
		for k := range bts {
			bts[k] = true
		}
	}
	if *bearishFlag {
		bts[kStrategySellPutVertical] = false
		bts[kStrategyBuyCallVertical] = false
	}
	if *bullishFlag {
		bts[kStrategySellCallVertical] = false
		bts[kStrategyBuyPutVertical] = false
	}
	if *noliqFlag {
		bts[kStrategyLiquidateCall] = false
		bts[kStrategyLiquidatePut] = false
		bts[kStrategyLiquidateCallVertical] = false
		bts[kStrategyLiquidatePutVertical] = false
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
			Demand:       *demandFlag,
			Sigmas:       *sigmasFlag,
			Cooldown:     *cooldownFlag,
			Patience:     *patienceFlag,
			StartOfDay:   *startOfDayFlag,
			FullRiskTime: *fullRiskTimeFlag,
			StopTrading:  *stopTradingFlag,
			MaxPending:   *maxPendingFlag,
			Strategies:   map[string]bool{},
			WeightRisk:   *wRiskFlag,
			WeightDelta:  *wDeltaFlag,
			WeightPayoff: *wPayoffFlag,
			Dump:         *dumpFlag,
			Prune:        *pruneFlag,
			Floor:        *floorFlag,
			Budget:       *budgetFlag,
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
	log.Printf("varu is on the prowl")

	// NVDA strategy (sharpe 10.40)
	t = NewTrader(symbol.MustParse("NVDA"), newConfig())
	t.Config.Listen = "127.0.0.1:8486"
	t.Config.Strategies[kStrategySellCallVertical] = true // bearish
	t.Config.Floor = 15_000
	t.Config.Budget = 3_000
	t.Config.Eval = decimal.FromInt(3)
	t.Config.Spread = decimal.FromInt(1)
	t.Config.WeightDelta = decimal.Zero
	t.Config.WeightRisk = decimal.Zero
	t.Config.Dump = 5 * clocky.Minute
	traders = append(traders, t)

	// TSLA strategy (sharpe 5.86)
	t = NewTrader(symbol.MustParse("TSLA"), newConfig())
	t.Config.Listen = "127.0.0.1:8485"
	t.Config.Strategies[kStrategySellCallVertical] = true // bearish
	t.Config.Floor = 15_000
	t.Config.Budget = 3_000
	t.Config.Eval = decimal.FromInt(2)
	t.Config.Spread = decimal.FromInt(1)
	t.Config.WeightDelta = decimal.Zero
	t.Config.WeightRisk = decimal.Zero
	t.Config.StartOfDay = 10_00_00
	t.Config.Dump = 5 * clocky.Minute
	traders = append(traders, t)

	// SPXW strategy (sharpe 2.85)
	t = NewTrader(symbol.MustParse("SPXW"), newConfig())
	t.Config.Listen = "127.0.0.1:8484"
	t.Config.Strategies[kStrategySellCallVertical] = true // bearish
	t.Config.Floor = 15_000
	t.Config.Budget = 3_000
	t.Config.Eval = decimal.FromInt(3)
	t.Config.Spread = decimal.Half
	t.Config.WeightDelta = decimal.Zero
	t.Config.WeightRisk = decimal.Zero
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

func fanoutSchwabOrderUpdates(orderEvents <-chan *schwab.OrderEvent, traders []*Trader) {
	for orderEvent := range orderEvents {
		for _, trader := range traders {
			trader.OrderEvents <- orderEvent
		}
	}
}
