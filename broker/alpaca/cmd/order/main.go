package main

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/loggy"
	"dropbear/osi"
	"dropbear/symbol"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
)

var (
	flagOPG           = flag.Bool("opg", false, "schedule order to occur in opening auction")
	flagCLS           = flag.Bool("cls", false, "schedule order to occur in closing auction")
	flagFOK           = flag.Bool("fok", false, "use fill or kill time in force")
	flagGTC           = flag.Bool("gtc", false, "use good till canceled time in force")
	flagIOC           = flag.Bool("ioc", false, "use immediate or cancel time in force")
	flagTWAP          = flag.Bool("twap", false, "use time weighted average price algorithm")
	flagVWAP          = flag.Bool("vwap", false, "use volume weighted average price algorithm")
	flagWait          = flag.Bool("wait", false, "wait for order to fill")
	flagDMA           = alpaca.OrderDestinationFlag("dma", "", "directly route order to lit exchange (NYSE, NASDAQ, ARCA)")
	flagExt           = flag.Bool("ext", false, "participate in extended hours trading (must be limit order with default (day) time in force)")
	flagLimit         = decimal.Flag("limit", "0", "sets an explicit limit price (the default is to use the midpoint plus/minus greed depending on the side) must be positve")
	flagAmt           = decimal.Flag("amt", "0", "usd notional value of order (negative to sell) which is mutually exclusive with -qty")
	flagQty           = decimal.Flag("qty", "0", "number of shares to transact (negative to sell) which is mutually exclusive with -amt")
	flagGreed         = decimal.FlagBPS("greed", "0", "number of basis points improvement over market midpoint to demand (or negative to increase likelihood of execution); only applies to limit orders without an explicit -price")
	flagDuration      = clocky.DurationFlag("duration", "0", "duration over which to execute TWAP/VWAP orders (e.g. 1h30m; default is until end of day)")
	flagParticipation = decimal.Flag("participation", "0.15", "maximum volume participation rate if TWAP/VWAP order")
	flagDisplay       = decimal.Flag("display", "0", "max amount of shares to display on exchange in dma mode")
	flagStop          = decimal.Flag("stop", "0", "stop loss price")
	flagTrail         = flag.String("trail", "", "trailing stop distance in dollars (e.g. 0.50) or percent (e.g. 1%)")
	flagOCO           = flag.Bool("oco", false, "use one cancels other (OCO) order class")
	flagOTO           = flag.Bool("oto", false, "use one triggers other (OTO) order class")
	flagBracket       = flag.Bool("bracket", false, "use bracket order class")
	flagStopLimit     = decimal.Flag("stop-limit", "0", "stop limit price")
	flagTakeProfit    = decimal.Flag("take", "0", "take profit price")
	flagMarket        = flag.Bool("market", false, "use market order")
	flagStrike        = decimal.Flag("strike", "0", "use strike price")
	flagExp           = clocky.TimeFlag("exp", "", "use expiration date in YYYY-MM-DD format")
	flagCall          = flag.Bool("call", false, "trade call option")
	flagPut           = flag.Bool("put", false, "trade put option")
)

func main() {
	loggy.Init()

	// parse flags
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `usage: order [options] SYMBOL...
description:
  places an order for the given stock symbols via alpaca.
examples:
  order -limit=329.23 -qty=100 GOOGL                 # buy goog at specific limit price
  order -vwap -amt=10_000.00 AAPL MSFT               # buy $10,000 worth of AAPL and $10,000 worth of MSFT at midpoint price using dash's volume-weighted algorithm
  order -twap -duration=1h -greed=-5 -qty=-1000 ADBE # sell (or short) 1000 shares of Adobo for an hour allowing up to 5 basis points of slippage over mid price right now
  order -limit=7.80 -qty=-1 WDC260130P00175000       # sell 1 put option contract for WDC with strike price $175 expiring Jan 30, 2026 at limit price $7.80
options:
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	// validate some flags
	if flagQty.IsZero() && flagAmt.IsZero() {
		fmt.Fprintf(os.Stderr, "no -qty or -amt specified\n")
		os.Exit(1)
	}
	if flagLimit.IsNegative() {
		fmt.Fprintf(os.Stderr, "-limit cannot be negative\n")
		os.Exit(1)
	}
	if flagStop.IsNegative() {
		fmt.Fprintf(os.Stderr, "-stop cannot be negative\n")
		os.Exit(1)
	}
	if !flagLimit.IsZero() && !flagGreed.IsZero() {
		fmt.Fprintf(os.Stderr, "-limit and -greed cannot both be specified\n")
		os.Exit(1)
	}

	// figure out time in force
	tifs := 0
	timeInForce := alpaca.TimeInForceDay
	if *flagGTC {
		timeInForce = alpaca.TimeInForceGTC
		tifs++
	}
	if *flagOPG {
		timeInForce = alpaca.TimeInForceOPG
		tifs++
		if *flagTWAP || *flagVWAP {
			fmt.Fprintf(os.Stderr, "TWAP/VWAP orders cannot participate in opening auction\n")
			os.Exit(1)
		}
	}
	if *flagCLS {
		timeInForce = alpaca.TimeInForceCLS
		tifs++
		if *flagTWAP || *flagVWAP {
			fmt.Fprintf(os.Stderr, "TWAP/VWAP orders cannot participate in closing auction\n")
			os.Exit(1)
		}
	}
	if *flagFOK {
		timeInForce = alpaca.TimeInForceFOK
		tifs++
		if *flagTWAP || *flagVWAP {
			fmt.Fprintf(os.Stderr, "TWAP/VWAP orders cannot use fill or kill time in force\n")
			os.Exit(1)
		}
	}
	if *flagIOC {
		timeInForce = alpaca.TimeInForceIOC
		tifs++
		if *flagTWAP || *flagVWAP {
			fmt.Fprintf(os.Stderr, "TWAP/VWAP orders cannot use immediate or cancel time in force\n")
			os.Exit(1)
		}
	}
	if tifs > 1 {
		fmt.Fprintf(os.Stderr, "only one of -gtc, -opg, -cls, -fok, -ioc can be specified\n")
		os.Exit(1)
	}

	// figure out order class
	orderClass := alpaca.OrderClassSimple
	orderClasses := 0
	if *flagBracket {
		orderClass = alpaca.OrderClassBracket
		orderClasses++
		if *flagExt {
			fmt.Fprintf(os.Stderr, "bracket orders cannot participate in extended hours\n")
			os.Exit(1)
		}
		if timeInForce != alpaca.TimeInForceDay && timeInForce != alpaca.TimeInForceGTC {
			fmt.Fprintf(os.Stderr, "bracket orders can only use day or GTC time in force\n")
			os.Exit(1)
		}
	}
	if *flagOTO {
		orderClass = alpaca.OrderClassOTO
		orderClasses++
	}
	if *flagOCO {
		orderClass = alpaca.OrderClassOCO
		orderClasses++
	}
	if orderClasses > 1 {
		fmt.Fprintf(os.Stderr, "cannot combine -bracket, -oto, and -oco order classes\n")
		os.Exit(1)
	}

	// figure out order type
	orderType := alpaca.OrderTypeLimit
	stopPrice := decimal.Zero
	trailPrice := decimal.Zero
	trailPercent := decimal.Zero
	if *flagMarket {
		orderType = alpaca.OrderTypeMarket
		if !flagLimit.IsZero() {
			fmt.Fprintf(os.Stderr, "-limit can't be used with market orders\n")
			os.Exit(1)
		}
		if !flagGreed.IsZero() {
			fmt.Fprintf(os.Stderr, "-greed only works with limit orders\n")
			os.Exit(1)
		}
		if !flagStop.IsZero() {
			fmt.Fprintf(os.Stderr, "-stop cannot be used with market orders\n")
			os.Exit(1)
		}
		if *flagTrail != "" {
			fmt.Fprintf(os.Stderr, "-trail cannot be used with market orders\n")
			os.Exit(1)
		}
		if orderClass == alpaca.OrderClassOCO {
			fmt.Fprintf(os.Stderr, "-oco orders should not specify -market or -limit (use -take and/or -stop)\n")
			os.Exit(1)
		}
	} else if !flagStop.IsZero() && orderClass == alpaca.OrderClassSimple {
		if flagLimit.IsZero() {
			orderType = alpaca.OrderTypeStop
		} else {
			orderType = alpaca.OrderTypeStopLimit
		}
		stopPrice = *flagStop
		if !flagGreed.IsZero() {
			fmt.Fprintf(os.Stderr, "-greed only works with limit orders\n")
			os.Exit(1)
		}
		if *flagTrail != "" {
			fmt.Fprintf(os.Stderr, "-trail cannot be used with stop orders\n")
			os.Exit(1)
		}
		if !flagStopLimit.IsZero() {
			fmt.Fprintf(os.Stderr, "-stop-limit is meant for -bracket or -oco or -oto orders\n")
			os.Exit(1)
		}
	} else if *flagTrail != "" {
		orderType = alpaca.OrderTypeTrailingStop
		if !flagGreed.IsZero() {
			fmt.Fprintf(os.Stderr, "-greed only works with limit orders\n")
			os.Exit(1)
		}
		if !flagLimit.IsZero() {
			fmt.Fprintf(os.Stderr, "-limit can't be used with -trail orders\n")
			os.Exit(1)
		}
		if !flagStopLimit.IsZero() {
			fmt.Fprintf(os.Stderr, "-stop-limit is meant for -bracket or -oco or -oto orders\n")
			os.Exit(1)
		}
		if orderClass != alpaca.OrderClassSimple {
			fmt.Fprintf(os.Stderr, "-trail orders cannot be used with -bracket or -oco or -oto order classes\n")
			os.Exit(1)
		}
		if timeInForce != alpaca.TimeInForceDay && timeInForce != alpaca.TimeInForceGTC {
			fmt.Fprintf(os.Stderr, "-trail orders can only use day or -gtc time in force\n")
			os.Exit(1)
		}
		ts := *flagTrail
		if ts[len(ts)-1] == '%' {
			percent, err := decimal.ParseString(ts[:len(ts)-1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid trail percentage: %v\n", err)
				os.Exit(1)
			}
			trailPercent = percent
		} else {
			price, err := decimal.ParseString(ts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid trail price: %v\n", err)
				os.Exit(1)
			}
			trailPrice = price
		}
	} else if orderClass == alpaca.OrderClassOCO {
		if flagTakeProfit.IsZero() || flagStop.IsZero() {
			fmt.Fprintf(os.Stderr, "-oco orders require both -take and -stop to be set\n")
			os.Exit(1)
		}
	}

	// figure out algorithm
	if *flagTWAP && *flagVWAP {
		fmt.Fprintf(os.Stderr, "cannot specify both -twap and -vwap\n")
		os.Exit(1)
	}
	var advanced *alpaca.AdvancedInstructions
	if *flagDMA != alpaca.OrderDestinationNone {
		advanced = &alpaca.AdvancedInstructions{
			Algorithm:   alpaca.OrderAlgorithmDMA,
			Destination: *flagDMA,
			DisplayQty:  *flagDisplay,
		}
		if advanced.DisplayQty.QuantizeTruncate(decimal.Lot).Cmp(advanced.MaxPercentage) != 0 {
			fmt.Fprintf(os.Stderr, "-display must be in round lots\n")
			os.Exit(1)
		}
		if *flagBracket || *flagOTO || *flagOCO || *flagTWAP || *flagVWAP {
			fmt.Fprintf(os.Stderr, "-dma only supports -market and -limit orders\n")
			os.Exit(1)
		}
		if timeInForce != alpaca.TimeInForceDay {
			fmt.Fprintf(os.Stderr, "-dma only supports day time in force\n")
			os.Exit(1)
		}
		if *flagExt && advanced.Destination != alpaca.OrderDestinationNASDAQ && advanced.Destination != alpaca.OrderDestinationARCA {
			fmt.Fprintf(os.Stderr, "extended hours only supported with NASDAQ and ARCA destinations\n")
			os.Exit(1)
		}
	} else if *flagTWAP || *flagVWAP {
		advanced = &alpaca.AdvancedInstructions{}
		if *flagTWAP {
			advanced.Algorithm = alpaca.OrderAlgorithmTWAP
		} else {
			advanced.Algorithm = alpaca.OrderAlgorithmVWAP
		}
		if *flagDuration > 0 {
			advanced.EndTime = clocky.Now().Add(*flagDuration)
		}
		advanced.MaxPercentage = *flagParticipation
		if advanced.MaxPercentage.QuantizeTruncate(decimal.Parse(".001")).Cmp(advanced.MaxPercentage) != 0 {
			fmt.Fprintf(os.Stderr, "-participation only allows three decimal places of precision\n")
			os.Exit(1)
		}
	}

	// sanity check fractional trading
	if flagQty.Truncate().Cmp(*flagQty) != 0 {
		if timeInForce != alpaca.TimeInForceDay {
			fmt.Fprintf(os.Stderr, "fractional shares only supported with day time in force\n")
			os.Exit(1)
		}
		if advanced != nil {
			fmt.Fprintf(os.Stderr, "fractional shares not supported with advanced order types\n")
			os.Exit(1)
		}
	}

	// figure out stop loss
	var stopLoss *alpaca.StopLoss
	if orderClass != alpaca.OrderClassSimple && !flagStop.IsZero() {
		stopLoss = &alpaca.StopLoss{
			StopPrice:  *flagStop,
			LimitPrice: *flagStopLimit,
		}
	} else if !flagStopLimit.IsZero() {
		fmt.Fprintf(os.Stderr, "-stop-limit requires -stop to be set along with -bracket or -oco or -oto\n")
		os.Exit(1)
	}

	// figure out take profit
	var takeProfit *alpaca.TakeProfit
	if !flagTakeProfit.IsZero() {
		takeProfit = &alpaca.TakeProfit{
			LimitPrice: *flagTakeProfit,
		}
	}

	// figure out extended hours
	extendedHours := false
	if *flagExt {
		extendedHours = true
		if orderType != alpaca.OrderTypeLimit {
			fmt.Fprintf(os.Stderr, "extended hours only supported with limit orders\n")
			os.Exit(1)
		}
		if timeInForce != alpaca.TimeInForceDay {
			fmt.Fprintf(os.Stderr, "extended hours only supported with day time in force\n")
			os.Exit(1)
		}
	}

	// get list of stocks
	symbols := flag.Args()
	if len(symbols) == 0 {
		fmt.Fprintf(os.Stderr, "no symbols specified\n")
		os.Exit(1)
	}

	// turn into option symbols if necessary
	if !flagStrike.IsZero() || !flagExp.IsZero() || *flagCall || *flagPut {
		if len(symbols) != 1 {
			fmt.Fprintf(os.Stderr, "option orders must specify exactly one symbol\n")
			os.Exit(1)
		}
		sym, err := symbol.Parse(symbols[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid symbol: %v\n", err)
			os.Exit(1)
		}
		if flagStrike.IsZero() {
			fmt.Fprintf(os.Stderr, "must specify -strike for option orders\n")
			os.Exit(1)
		}
		if flagExp.IsZero() {
			fmt.Fprintf(os.Stderr, "must specify -exp for option orders\n")
			os.Exit(1)
		}
		if !*flagCall && !*flagPut {
			fmt.Fprintf(os.Stderr, "must specify either -call or -put for option orders\n")
			os.Exit(1)
		}
		if *flagCall && *flagPut {
			fmt.Fprintf(os.Stderr, "cannot specify both -call and -put for option orders\n")
			os.Exit(1)
		}
		class := byte('C')
		if *flagPut {
			class = byte('P')
		}
		year, month, day := flagExp.Date()
		symbols[0] = osi.EncodeSpaceless(sym, *flagStrike, class, year, month, day)
	}

	// uncanonicalize options symbology
	for i, sym := range symbols {
		symbols[i] = osi.Uncanonicalize(sym)
	}

	// subscribe to websocket messages
	var orderUpdates <-chan *alpaca.OrderUpdate
	if *flagWait {
		orderUpdates = alpaca.OrderUpdates()
	}

	// loop over stocks
	var err error
	exitCode := 0
	client := alpaca.NewClient()
	for _, sym := range symbols {

		// lazy fetch quote once
		gotQuote := false
		bidPrice := decimal.Zero
		askPrice := decimal.Zero
		needQuote := func() {
			if gotQuote {
				return
			}
			bidPrice, askPrice, err = getQuote(client, sym)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error getting quote for %s: %v\n", sym, err)
				os.Exit(1)
			}
			gotQuote = true
		}

		// figure out qty
		side := ds.SideBuy
		qty := *flagQty
		if !qty.IsZero() {
			if qty.IsNegative() {
				side = ds.SideSell
				qty = qty.Neg()
			}
		} else {
			price := decimal.Zero
			amt := *flagAmt
			needQuote()
			if amt.IsNegative() {
				side = ds.SideSell
				price := bidPrice
				amt = amt.Neg()
				qty = amt.Div(price).Truncate()
			} else {
				price := askPrice
				qty = amt.Div(price).Truncate()
			}
			if qty.IsZero() {
				fmt.Fprintf(os.Stderr, "amt amount %s is too small to %s any shares of %s at price %s\n", amt, side, sym, price)
				if exitCode < 255 {
					exitCode++
				}
				continue
			}
		}

		// figure out our price
		limitPrice := *flagLimit
		if orderType == alpaca.OrderTypeLimit && limitPrice.IsZero() {
			needQuote()
			limitPrice = bidPrice.Add(askPrice).Half()
			if side == ds.SideBuy {
				limitPrice = limitPrice.Mul(decimal.One.Sub(*flagGreed))
				limitPrice = limitPrice.QuantizeTruncate(decimal.Cent)
			} else {
				limitPrice = limitPrice.Mul(decimal.One.Add(*flagGreed))
				limitPrice = limitPrice.QuantizeAway(decimal.Cent)
			}
		}

		// give the order
		clientOrderID := uuid.New().String()
		order, err := client.CreateOrder(&alpaca.CreateOrderRequest{
			ClientOrderID:        clientOrderID,
			Symbol:               sym,
			Side:                 side,
			Qty:                  qty,
			LimitPrice:           limitPrice,
			Type:                 orderType,
			TimeInForce:          timeInForce,
			ExtendedHours:        extendedHours,
			OrderClass:           orderClass,
			StopPrice:            stopPrice,
			TrailPrice:           trailPrice,
			TrailPercent:         trailPercent,
			StopLoss:             stopLoss,
			TakeProfit:           takeProfit,
			AdvancedInstructions: advanced,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error opening %s: %v\n", sym, err)
			if exitCode < 255 {
				exitCode++
			}
			continue
		}

		// log the order
		fmt.Printf("order placed to %s %s shares of %s at %s with status %s\n", side, qty, sym, limitPrice, order.Status)

		// print order updates until order is done if -wait is specified
		if *flagWait {
			for orderUpdate := range orderUpdates {
				if orderUpdate.Order.ClientOrderID != clientOrderID {
					continue
				}
				log.Printf("order update for %s: %s price=%s status=%s qty=%s pos=%s filled=%s/%s avg_price=%s", sym,
					orderUpdate.Event, orderUpdate.Price, orderUpdate.Order.Status, orderUpdate.Qty, orderUpdate.PositionQty,
					orderUpdate.Order.FilledQty, orderUpdate.Order.Qty, orderUpdate.Order.FilledAvgPrice)
				if orderUpdate.Order.Status.IsFinal() {
					break
				}
			}
		}
	}

	// report status to parent process
	os.Exit(exitCode)
}

func getQuote(client *alpaca.Client, sym string) (decimal.Decimal, decimal.Decimal, error) {
	_, _, _, _, _, _, err := osi.Parse(sym)
	if err != nil {
		// assume this is an equity symbol
		quote, err := client.GetQuote(sym)
		if err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		return quote.BidPrice, quote.AskPrice, nil
	}
	// this is an options symbol
	snapshot, err := client.GetOptionSnapshot(sym)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	return snapshot.LatestQuote.BidPrice, snapshot.LatestQuote.AskPrice, nil
}
