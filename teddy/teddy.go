package teddy

import (
	"dropbear/broker/binance"
	"dropbear/broker/binanceusd"
	"dropbear/broker/coinbase"
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
	flagBacktest   = flag.String("backtest", "", "name of backtest dataset")
	flagCPUProfile = flag.String("cpuprofile", "", "write cpu profile to file")
	flagRFR        = decimal.FlagBPS("rfr", "487", "annualized risk-free rate in basis points")
	flagQuantum    = clocky.DurationFlag("quantum", "1m", "metric sampling interval while backtesting")
	flagVerbose    = flag.Bool("teddy-verbose", false, "log order simulation decisions")
)

var (
	Live             bool // true means we're in live trading or paper trading mode, i.e. not backtesting
	Paper            bool // true means we're connected to live data but all trading orders are simulated
	BinanceClient    *binance.Client
	BinanceusdClient *binanceusd.Client
	CoinbaseClient   *coinbase.Client
	gSigChan         chan os.Signal
	gRunning         bool
	gBenchmark       *Pair
	gRateLimiter     *rateLimiter
)

func Init() {
	Live = *flagBacktest == ""
	BinanceClient = binance.NewClient()
	BinanceusdClient = binanceusd.NewClient()
	CoinbaseClient = coinbase.NewClient()
	if Live {
		if *flagPaper {
			Paper = true
			gRateLimiter = newRateLimiterCoinbase()
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
		gRateLimiter = newRateLimiterCoinbase()
	}
}

// SetBalance sets the simulated balance for the given broker and symbol in backtest mode.
func SetBalance(broker ds.Broker, symbol string, quantity decimal.Decimal) {
	if quantity.IsNegative() {
		panic("cannot set negative balance")
	}
	if quantity.IsZero() {
		return
	}
	if !Live {
		ex := Brokers.Get(broker)
		ho := ex.Holdings.Get(symbol)
		ho.Lock.Lock()
		defer ho.Lock.Unlock()
		ho.Quantity = quantity
		ho.Available = quantity
		if !ho.IsFiat {
			ho.Lots.Add(clocky.Now(), quantity, decimal.Zero)
		}
		ho.Check()
	}
}

// SetBenchmark sets the asset against which performance in judged.
func SetBenchmark(pair *Pair) {
	gBenchmark = pair
}

// Run starts the main event loop.
// This should be called at the end of your main() function.
func Run() {
	gRunning = true
	defer func() {
		gRunning = false
	}()
	if gBenchmark == nil {
		panic("you forgot to call teddy.SetBenchmark()")
	}
	report := newReport(gBenchmark)
	if Live {
		defer cancelAllOpenOrders()
		sampler := newSamplerDaemon(*flagQuantum, report)
		defer sampler.Close()
		sampler.Run()
		for _, broker := range Brokers.All() {
			broker.run()
			for _, pair := range broker.Pairs.All() {
				pair.run()
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
			for _, pair := range broker.Pairs.All() {
				manager.Register(pair)
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

// Spawn runs the given function in a goroutine in live mode, or inline in backtest mode.
func Spawn(f func()) {
	if Live {
		go f()
	} else {
		f()
	}
}

// GetEquityUSD returns value of all holdings in USD across all brokers.
func GetEquityUSD() decimal.Decimal {
	equity := decimal.Zero
	for _, broker := range Brokers.All() {
		equity = equity.Add(broker.Holdings.GetEquityUSD())
	}
	return equity
}

// GetInvestedUSD returns value of all non-cash holdings in USD across all brokers.
func GetInvestedUSD() decimal.Decimal {
	equity := decimal.Zero
	for _, broker := range Brokers.All() {
		equity = equity.Add(broker.Holdings.GetInvestedUSD())
	}
	return equity
}

// GetRiskFreeRate returns annualized risk-free rate.
// TODO(jart): this should take a time parameter since it changes.
func GetRiskFreeRate() decimal.Decimal {
	return *flagRFR
}

// cancelAllOpenOrders cancels all open orders across all brokers.
// This is implemented to bypass the framework so it can happen on panic.
func cancelAllOpenOrders() {
	if Paper {
		return
	}
	for _, broker := range Brokers.brokerArray {
		switch broker.Broker {
		case ds.BrokerCoinbase:
			var orderIDs []string
			for _, order := range broker.Orders.openOrders.Values() {
				if order.OrderID != "" {
					orderIDs = append(orderIDs, order.OrderID)
					log.Printf("canceling order %s on %s", order.OrderID, broker)
				}
			}
			if len(orderIDs) > 0 {
				if err := CoinbaseClient.CancelOrders(orderIDs); err != nil {
					log.Printf("error canceling orders on %s: %v", broker, err)
				}
			}
		default:
			continue
		}
	}
}
