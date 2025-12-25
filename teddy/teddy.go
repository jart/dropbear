package teddy

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/exchange/binance"
	"dropbear/exchange/coinbase"
	"dropbear/loggy"
	"dropbear/teddy/metrics"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"
)

var (
	flagBacktest   = flag.String("backtest", "", "name of backtest dataset")
	flagCPUProfile = flag.String("cpuprofile", "", "write cpu profile to file")
	flagRFR        = decimal.FlagBPS("rfr", "487", "annualized risk-free rate in basis points")
	flagQuantum    = clocky.DurationFlag("quantum", "1h30m", "metric sampling interval while backtesting")
)

var (
	Live              bool
	BinanceClient     *binance.Client
	CoinbaseClient    *coinbase.Client
	gManager          *manager
	gSigChan          chan os.Signal
	gBenchmark        *Pair
	gReport           *report
	gStrategyEquity   *metrics.Equity
	gBenchmarkEquity  *metrics.Equity
	gStrategyInvested *metrics.Invested
	gBenchmarkQty     decimal.Decimal  // quantity of benchmark asset for buy-and-hold comparison
	gInitialHoldings  []initialHolding // snapshot of holdings when Run() is called
)

func Init() {
	Live = *flagBacktest == ""
	BinanceClient = binance.NewClient()
	CoinbaseClient = coinbase.NewClient()
	if Live {
		gSigChan = make(chan os.Signal, 1)
		signal.Notify(gSigChan, syscall.SIGINT, syscall.SIGTERM)
		loggy.AlsoLogToFile()
	} else {
		ds.SetOffline()
		clocky.Now = clocky.FakeNow
		gReport = newReport()
		gManager = newManager()
		gStrategyInvested = metrics.NewInvested()
		gStrategyEquity = metrics.NewEquity(*flagQuantum)
		gBenchmarkEquity = metrics.NewEquity(*flagQuantum)
	}
}

// SetBalance sets the simulated balance for the given exchange and symbol in backtest mode.
func SetBalance(exchange ds.Exchange, symbol string, quantity decimal.Decimal) {
	if !Live {
		ex := Exchanges.Get(exchange)
		h := ex.Holdings.Get(symbol)
		h.Lock.Lock()
		defer h.Lock.Unlock()
		h.Quantity = quantity
		h.Available = quantity
		if !h.IsCash {
			h.Lots.Add(clocky.Now(), quantity, decimal.Zero)
		}
	}
}

// SetBenchmark sets the asset against which performance in judged in backtest mode.
func SetBenchmark(pair *Pair) {
	if !Live {
		gBenchmark = pair
	}
}

// Run starts the main event loop.
// This should be called at the end of your main() function.
func Run() {
	if Live {
		defer func() {
			for _, exchange := range Exchanges.All() {
				for _, order := range exchange.Orders.Open() {
					order.Cancel()
				}
			}
		}()
		<-gSigChan
		log.Printf("goodbye")
	} else {
		if gBenchmark == nil {
			log.Fatalf("you forgot to call teddy.SetBenchmark() in backtest mode")
		}
		captureInitialHoldings()
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
		gManager.Run()
		gReport.Print()
		os.Exit(0)
	}
}

// Spawn runs the given function in a goroutine in live mode, or inline in backtest mode.
func Spawn(f func()) {
	if Live {
		go f()
	} else {
		f()
	}
}

// GetEquityUSD returns value of all holdings in USD across all exchanges.
func GetEquityUSD() decimal.Decimal {
	equity := decimal.Zero
	for _, exchange := range Exchanges.All() {
		equity = equity.Add(exchange.Holdings.GetEquityUSD())
	}
	return equity
}

// GetInvestedUSD returns value of all non-cash holdings in USD across all exchanges.
func GetInvestedUSD() decimal.Decimal {
	equity := decimal.Zero
	for _, exchange := range Exchanges.All() {
		equity = equity.Add(exchange.Holdings.GetInvestedUSD())
	}
	return equity
}

// GetRiskFreeRate returns annualized risk-free rate.
// TODO(jart): this should take a time parameter since it changes.
func GetRiskFreeRate() decimal.Decimal {
	return *flagRFR
}
