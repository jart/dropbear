package main

import (
	"dropbear/black76"
	"dropbear/broker/alpaca"
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/loggy"
	"dropbear/osi"
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
	dateFlag := clocky.TimeFlag("date", "", "date of the trades")
	symbolFlag := symbol.Flag("symbol", "", "symbol to trade (e.g. NVDA)")
	webFlag := flag.Bool("web", false, "enable web dashboard feature")
	schwabFlag := flag.Bool("schwab", false, "use schwab as broker instead of alpaca")
	strikesFlag := flag.Int("strikes", 0, "how many strikes wide strangle should be")
	wingFlag := decimal.Flag("wing", "0.05", "how much to pay for each wing of short strangle insurance, where zero means naked")
	toleranceFlag := decimal.Flag("tolerance", "-75_000", "delta imbalance tolerance in usd (negative minimizes trading; positive does symmetrical market making)")
	directionFlag := decimal.Flag("direction", "1", "direction of options trade (+1 is long; -1 is short)")
	contractsFlag := decimal.Flag("contracts", "2", "max number of strangles to create")
	riskFlag := decimal.Flag("risk", "5000", "maximum amount of options risk to tolerate")
	chaseFlag := clocky.DurationFlag("chase", "1m", "how long to wait before chasing nbbo")
	patienceFlag := clocky.DurationFlag("patience", "60s", "how long to wait for order execution")
	spreadFlag := decimal.Flag("spread", "-1", "spread crossing for contract (-1=make, 0=mid, 1=take)")
	startOfDayFlag := flag.Int("sod", 9_35_00, "start of day in HHMMSS")
	ageFlag := clocky.DurationFlag("age", "5m", "how much volatility history is needed to trade")
	lookbackFlag := clocky.DurationFlag("lookback", "30m", "how far indicators should look back")
	samplesFlag := flag.Int("samples", 13, "number of samples for moving average indicators")
	dmaFlag := alpaca.OrderDestinationFlag("dma", "", "directly route order to lit exchange (NYSE, NASDAQ, ARCA)")
	flagCPUProfile := flag.String("cpuprofile", "", "write cpu profile to file")

	newConfig := func() *Config {
		return &Config{
			StartOfDay: *startOfDayFlag,
			Contracts:  *contractsFlag,
			Direction:  *directionFlag,
			Tolerance:  *toleranceFlag,
			Patience:   *patienceFlag,
			Lookback:   *lookbackFlag,
			Samples:    *samplesFlag,
			Strikes:    *strikesFlag,
			Symbol:     *symbolFlag,
			Spread:     *spreadFlag,
			Schwab:     *schwabFlag,
			Chase:      *chaseFlag,
			Risk:       *riskFlag,
			Wing:       *wingFlag,
			DMA:        *dmaFlag,
			Age:        *ageFlag,
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
		if *webFlag {
			t.Web.Start(t)
		}
		err := t.Backtest(*dbnFlag, *dateFlag)
		if err != nil {
			log.Fatalf("backtest error: %v", err)
		}
		t.Web.broadcastState()
		return
	}

	// production
	var t *Trader
	var traders []*Trader
	loggy.AlsoLogToFile()
	log.Printf("prepare to be strangled")

	t = NewTrader(newConfig())
	t.Config.Symbol = symbol.NVDA
	t.Config.Listen = "127.0.0.1:8486"
	t.Config.Tolerance = decimal.Zero
	t.Config.Spread = decimal.Parse("-3")
	traders = append(traders, t)

	t = NewTrader(newConfig())
	t.Config.Symbol = symbol.TSLA
	t.Config.Listen = "127.0.0.1:8485"
	traders = append(traders, t)

	// t = NewTrader(newConfig())
	// t.Config.Symbol = symbol.GOOGL
	// t.Config.Listen = "127.0.0.1:8491"
	// traders = append(traders, t)

	// t = NewTrader(newConfig())
	// t.Config.Symbol = symbol.AMZN
	// t.Config.Listen = "127.0.0.1:8494"
	// traders = append(traders, t)

	t = NewTrader(newConfig())
	t.Config.Symbol = symbol.MSFT
	t.Config.Listen = "127.0.0.1:8492"
	t.Config.Tolerance = decimal.Zero
	t.Config.Spread = decimal.Parse("-3")
	traders = append(traders, t)

	// t = NewTrader(newConfig())
	// t.Config.Symbol = symbol.AAPL
	// t.Config.Listen = "127.0.0.1:8493"
	// traders = append(traders, t)

	t = NewTrader(newConfig())
	t.Config.Symbol = symbol.AVGO
	t.Config.Listen = "127.0.0.1:8490"
	t.Config.Tolerance = decimal.Parse("-84_000")
	traders = append(traders, t)

	// t = NewTrader(newConfig())
	// t.Config.Symbol = symbol.IBIT
	// t.Config.Listen = "127.0.0.1:8495"
	// traders = append(traders, t)

	// t = NewTrader(newConfig())
	// t.Config.Symbol = symbol.SLV
	// t.Config.Listen = "127.0.0.1:8496"
	// traders = append(traders, t)

	// t = NewTrader(newConfig())
	// t.Config.Symbol = symbol.AKAM
	// t.Config.Listen = "127.0.0.1:8497"
	// traders = append(traders, t)

	// t = NewTrader(newConfig())
	// t.Config.Symbol = symbol.ADBE
	// t.Config.Listen = "127.0.0.1:8498"
	// traders = append(traders, t)

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

	// start web server for each trader
	for _, t := range traders {
		if *webFlag {
			t.Web.Start(t)
		}
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
	tradersByUnderlyingSymbol := map[symbol.Symbol]*Trader{}
	for _, trader := range traders {
		tradersByUnderlyingSymbol[trader.Config.Symbol] = trader
	}
	for orderEvent := range orderEvents {
		if trader := tradersByUnderlyingSymbol[tickerSymbol(orderEvent.Order.Symbol)]; trader != nil {
			trader.OrderEventsAlpaca <- orderEvent
		}
	}
}

func tickerSymbol(ticker string) symbol.Symbol {
	underlyingSymbol, _, _, _, _, _, err := osi.Parse(ticker)
	if err == nil {
		return underlyingSymbol
	}
	symbol, err := symbol.Parse(ticker)
	if err == nil {
		return symbol
	}
	return 0
}
