package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/loggy"
	"dropbear/netty"
	"dropbear/symbol"
	"flag"
	"log"
	"os"
	"runtime/pprof"
)

var (
	flagVerbose    = flag.Bool("v", false, "verbose logging")
	flagBench      = flag.String("bench", "GOOG", "benchmark against just holding symbol")
	flagPaper      = flag.Bool("paper", false, "simulate order execution on live data")
	flagBacktest   = flag.Bool("backtest", false, "run backtest on historical data")
	flagCash       = decimal.Flag("cash", "100_000", "initial USD balance")
	flagStart      = clocky.TimeFlag("start", "1980-01-01", "backtest start date")
	flagEnd        = clocky.TimeFlag("end", "2099-12-31", "backtest end date")
	flagCPUProfile = flag.String("cpuprofile", "", "write cpu profile to file")
	flagRFR        = decimal.FlagBPS("rfr", "487", "annualized risk-free rate in basis points")
	FlagQuantum    = clocky.DurationFlag("quantum", "1d", "metric sampling interval while backtesting")
	flagRekt       = decimal.Flag("rekt", "25_000", "portfolio value at which to consider the account liquidated")
	FlagBuffer     = decimal.FlagPercent("buffer", "1", "percent of buying power to leave untapped")
	FlagVWAP       = decimal.FlagPercent("vwap", "7", "percent of minute volume we can take")
	FlagPatience   = clocky.DurationFlag("patience", "15m", "time to wait for order fills")
)

var (
	Live        bool // true means we're in live trading or paper trading mode
	Paper       bool // true means orders are simulated
	Verbose     bool // true means verbose logging is enabled
	IsWarmingUp bool // true means we're backfilling historical data
	Liquidated  bool // true if portfolio value dropped to $25k or below
	Client      *alpaca.Client
	Running     bool
	Benchmark   *Equity
	Cash        decimal.Decimal
)

var (
	gLastEquity            decimal.Decimal // portfolio value at previous close
	gLastMaintenanceMargin decimal.Decimal // margin used at previous close
	gMarginHold            decimal.Decimal // margin reserved for pending orders
	gDayTradingBuyingPower decimal.Decimal // buying power for day trades
	gIsPatternDayTrader    bool            // user has been exposed as a pattern day trader
	gFeeCalculator         *alpaca.FeeCalculator
	gInterestCalculator    *alpaca.InterestCalculator
)

func Init() {
	Live = !*flagBacktest
	if Live {
		Verbose = true // life is slow, so don't keep secrets
		if *flagPaper {
			Paper = true
		}
		Client = alpaca.NewClient()
		err := Client.SyncAssets()
		if err != nil {
			panic(err)
		}
		loggy.AlsoLogToFile()
	} else {
		Verbose = *flagVerbose // backtests are fast, so avoid spam
		if *flagPaper {
			panic("-paper is implied by -backtest")
		}
		Paper = true
		netty.SetOffline()
		clocky.Now = clocky.FakeNow
		clocky.Sleep = clocky.FakeSleep
		clocky.NewTicker = clocky.FakeNewTicker
	}
}

// Run runs the main event loop.
func Run() {
	if Benchmark == nil {
		sym, err := symbol.Parse(*flagBench)
		if err != nil {
			loggy.Fatalf("invalid -bench symbol %q", *flagBench)
		}
		equity, err := AddEquity(sym)
		if err != nil {
			loggy.Fatalf("could not add benchmark symbol %q: %v", sym, err)
		}
		Benchmark = equity
	}
	Running = true
	defer func() { Running = false }()
	gFeeCalculator = alpaca.NewFeeCalculator()
	gInterestCalculator = alpaca.NewInterestCalculator(decimal.Parse("0.01"))
	if Live {
		liveTrader := newLiveTrader()
		defer liveTrader.Close()
		liveTrader.Run()
	} else {
		Cash = *flagCash
		backtest := newBacktest()
		defer backtest.Close()
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
		backtest.Run()
	}
}

func GetPortfolioValue() decimal.Decimal {
	return Cash.Add(GetInvestedValue())
}

func GetInvestedValue() decimal.Decimal {
	total := decimal.Zero
	for _, equity := range Equities {
		if equity.Quantity.IsZero() {
			continue
		}
		if !equity.Price.IsPositive() {
			loggy.Fatalf("can't compute GetInvestedValue() because %s has non-positive price %s", equity.Symbol, equity.Price)
		}
		total = total.Add(equity.Price.Mul(equity.Quantity.Abs()))
	}
	return total
}

func GetMarginUsed() decimal.Decimal {
	total := decimal.Zero
	for _, equity := range Equities {
		if equity.Quantity.IsZero() {
			continue
		}
		if !equity.Price.IsPositive() {
			loggy.Fatalf("can't compute GetMarginUsed() because %s has non-positive price %s", equity.Symbol, equity.Price)
		}
		margin := equity.Asset.GetMaintenanceMargin(equity.Quantity, equity.Price)
		total = total.Add(margin)
	}
	return total
}

// GetAvailableBuyingPower returns the remaining day trading buying power.
func GetAvailableBuyingPower() decimal.Decimal {
	return gDayTradingBuyingPower.Sub(GetMarginUsed()).Sub(gMarginHold)
}

func onMarketOpen() {
	// Calculate BOD DTBP from previous close values (already snapshotted in onMarketClose)
	// bod_dtbp = 4 × (last_equity - last_maintenance_margin) for PDT
	// bod_dtbp = 2 × (last_equity - last_maintenance_margin) for non-PDT
	gIsPatternDayTrader = gLastEquity.Cmp(decimal.FromInt(25_000)) >= 0
	excessEquity := gLastEquity.Sub(gLastMaintenanceMargin)
	gDayTradingBuyingPower = excessEquity.Mul(getBuyingPowerMultiplier())
	log.Printf("market open: last_equity=$%s last_maint=$%s excess=$%s multiplier=%s DTBP=$%s",
		gLastEquity.FormatThousand(2),
		gLastMaintenanceMargin.FormatThousand(2),
		excessEquity.FormatThousand(2),
		getBuyingPowerMultiplier(),
		gDayTradingBuyingPower.FormatThousand(2))
}

func onMarketClose(now clocky.Time) {
	Cash = Cash.Sub(gInterestCalculator.GetDailyInterest(now, Cash, *flagRFR))
	gLastEquity = GetPortfolioValue()
	gLastMaintenanceMargin = GetMarginUsed()
}

func getBuyingPowerMultiplier() decimal.Decimal {
	if gIsPatternDayTrader {
		return decimal.FromInt(4)
	}
	return decimal.FromInt(2)
}
