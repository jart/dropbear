package cubby

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/exchange/alpaca"
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
)

var (
	Live         bool // true means we're in live trading or paper trading mode
	Paper        bool // true means orders are simulated
	AlpacaClient *alpaca.Client
	gSigChan     chan os.Signal
	gRunning     bool
	gBenchmark   *Equity
	gRateLimiter *rateLimiter
)

func Init() {
	Live = !*flagBacktest
	AlpacaClient = alpaca.NewClient()
	if Live {
		if *flagPaper {
			Paper = true
			gRateLimiter = newRateLimiter()
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
		gRateLimiter = newRateLimiter()
	}
}

// StartTime returns the backtest start time.
func StartTime() clocky.Time {
	return *flagStart
}

// EndTime returns the backtest end time.
func EndTime() clocky.Time {
	return *flagEnd
}

// SetBalance sets the simulated balance for the given exchange and symbol.
// For USD, this sets the cash balance on the Exchange.
// For stocks, this sets the position quantity on the Holding.
func SetBalance(exchange ds.Exchange, symbol string, quantity decimal.Decimal) {
	if quantity.IsNegative() {
		panic("cannot set negative balance")
	}
	if Live {
		return
	}
	ex := Exchanges.Get(exchange)
	if symbol == "USD" {
		ex.Lock.Lock()
		ex.Cash.Store(quantity)
		// Set buying power based on margin flag
		ex.RegTBuyingPower.Store(quantity.MulInt(2))
		ex.DayTradingBuyingPower.Store(quantity.MulInt(4))
		ex.Lock.Unlock()
	} else {
		if quantity.IsZero() {
			return
		}
		ho := ex.Holdings.Get(symbol)
		ho.Lock.Lock()
		ho.Quantity.Store(quantity)
		ho.Available.Store(quantity)
		ho.Lots.Add(clocky.Now(), quantity, decimal.Zero)
		ho.Check()
		ho.Lock.Unlock()
	}
}

// SetBenchmark sets the asset against which performance is judged.
func SetBenchmark(equity *Equity) {
	gBenchmark = equity
}

// Run starts the main event loop.
func Run() {
	gRunning = true
	defer func() {
		gRunning = false
	}()
	if gBenchmark == nil {
		panic("you forgot to call cubby.SetBenchmark()")
	}
	report := newReport(gBenchmark)
	if Live {
		defer cancelAllOpenOrders()
		sampler := newSamplerDaemon(*flagQuantum, report)
		defer sampler.Close()
		sampler.Run()
		for _, exchange := range Exchanges.All() {
			exchange.run()
			for _, eq := range exchange.Equities.All() {
				eq.run()
			}
		}
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
		for _, exchange := range Exchanges.All() {
			for _, eq := range exchange.Equities.All() {
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

// GetEquityUSD returns value of all holdings in USD across all exchanges.
func GetEquityUSD() decimal.Decimal {
	equity := decimal.Zero
	for _, exchange := range Exchanges.All() {
		equity = equity.Add(exchange.Holdings.GetEquityUSD())
	}
	return equity
}

// GetInvestedUSD returns value of all non-cash holdings in USD.
func GetInvestedUSD() decimal.Decimal {
	equity := decimal.Zero
	for _, exchange := range Exchanges.All() {
		equity = equity.Add(exchange.Holdings.GetInvestedUSD())
	}
	return equity
}

// GetRiskFreeRate returns annualized risk-free rate.
func GetRiskFreeRate() decimal.Decimal {
	return *flagRFR
}

// cancelAllOpenOrders cancels all open orders.
func cancelAllOpenOrders() {
	if !Paper {
		for _, exchange := range Exchanges.exchangeArray {
			for _, order := range exchange.Orders.openOrders.Values() {
				if order.OrderID != "" {
					log.Printf("canceling order %s on %s", order.OrderID, exchange)
					if err := AlpacaClient.CancelOrder(order.OrderID); err != nil {
						log.Printf("error canceling order %s on %s: %v", order.OrderID, exchange, err)
					}
				}
			}
		}
	}
}
