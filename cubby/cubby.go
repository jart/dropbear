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

	// Register DTBP lifecycle callbacks
	// These fire at specific market times to manage day trading buying power
	AfterOpen(func() {
		for _, b := range Brokers.All() {
			b.InitDTBP()
		}
	})
	BeforeCloseEarly(func() {
		for _, b := range Brokers.All() {
			b.EndDayTradingTime()
		}
	})
	AfterClose(func() {
		for _, b := range Brokers.All() {
			b.LockDTBP()
			// Charge margin interest on negative cash balance
			if b.MarginInterest != nil {
				charge := b.MarginInterest.ChargeDaily(clocky.Now(), b.Cash.Load())
				if charge.IsPositive() {
					sub(&b.Cash, charge)
				}
			}
		}
	})
}

// StartTime returns the backtest start time.
func StartTime() clocky.Time {
	return *flagStart
}

// EndTime returns the backtest end time.
func EndTime() clocky.Time {
	return *flagEnd
}

// SetBalance sets the simulated balance for the given broker and symbol.
// For USD, this sets the cash balance on the Broker.
// For stocks, this sets the position quantity on the Holding.
func SetBalance(broker ds.Broker, symbol string, quantity decimal.Decimal) {
	if quantity.IsNegative() {
		panic("cannot set negative balance")
	}
	if Live {
		return
	}
	b := Brokers.Get(broker)
	if symbol == "USD" {
		b.Lock.Lock()
		b.Cash.Store(quantity)
		// Set PDT mode from flag
		b.PatternDayTrader = *flagPDT
		// Set buying power based on margin flag
		b.RegTBuyingPower.Store(quantity.MulInt(2))
		if *flagPDT {
			b.DayTradingBuyingPower.Store(quantity.MulInt(4))
			b.BodDTBP = quantity.MulInt(4)
			b.IsDayTradingTime = true
		} else {
			b.DayTradingBuyingPower.Store(quantity.MulInt(*flagMargin))
			b.BodDTBP = quantity.MulInt(2)
		}
		// Initialize LastEquity for DTBP lifecycle
		b.LastEquity = quantity
		b.Lock.Unlock()
	} else {
		if quantity.IsZero() {
			return
		}
		ho := b.Holdings.Get(symbol)
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
		for _, broker := range Brokers.All() {
			broker.run()
			for _, eq := range broker.Equities.All() {
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

// GetEquityUSD returns value of all holdings in USD across all brokers.
func GetEquityUSD() decimal.Decimal {
	equity := decimal.Zero
	for _, broker := range Brokers.All() {
		equity = equity.Add(broker.Holdings.GetEquityUSD())
	}
	return equity
}

// GetInvestedUSD returns value of all non-cash holdings in USD.
func GetInvestedUSD() decimal.Decimal {
	equity := decimal.Zero
	for _, broker := range Brokers.All() {
		equity = equity.Add(broker.Holdings.GetInvestedUSD())
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
		for _, broker := range Brokers.brokerArray {
			for _, order := range broker.Orders.openOrders.Values() {
				if order.OrderID != "" {
					log.Printf("canceling order %s on %s", order.OrderID, broker)
					if err := AlpacaClient.CancelOrder(order.OrderID); err != nil {
						log.Printf("error canceling order %s on %s: %v", order.OrderID, broker, err)
					}
				}
			}
		}
	}
}
