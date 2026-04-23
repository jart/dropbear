package main

import (
	"dropbear/broker/alpaca"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/osi"
	"dropbear/symbol"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"slices"
	"syscall"

	"github.com/google/uuid"
)

var (
	flagOPG            = flag.Bool("opg", false, "schedule order to occur in opening auction")
	flagCLS            = flag.Bool("cls", false, "schedule order to occur in closing auction")
	flagFOK            = flag.Bool("fok", false, "use fill or kill time in force")
	flagGTC            = flag.Bool("gtc", false, "use good till canceled time in force")
	flagIOC            = flag.Bool("ioc", false, "use immediate or cancel time in force")
	flagTWAP           = flag.Bool("twap", false, "use time weighted average price algorithm")
	flagVWAP           = flag.Bool("vwap", false, "use volume weighted average price algorithm")
	flagWait           = flag.Bool("wait", false, "wait for order to fill")
	flagDMA            = alpaca.OrderDestinationFlag("dma", "", "directly route order to lit exchange (NYSE, NASDAQ, ARCA)")
	flagExt            = flag.Bool("ext", false, "participate in extended hours trading (must be limit order with default (day) time in force)")
	flagLimit          = decimal.Flag("limit", "0", "sets an explicit limit price (the default is to use the midpoint plus/minus greed depending on the side) must be positve")
	flagAmt            = decimal.Flag("amt", "0", "usd notional value of order (negative to sell) which is mutually exclusive with -qty")
	flagQty            = decimal.Flag("qty", "0", "number of shares to transact (negative to sell) which is mutually exclusive with -amt")
	flagLiquidate      = flag.Bool("liquidate", false, "set quantity to liquidate existing position")
	flagLiquidateLong  = flag.Bool("liquidate-long", false, "set quantity to liquidate existing long position")
	flagLiquidateShort = flag.Bool("liquidate-short", false, "set quantity to liquidate existing short position")
	flagGreed          = decimal.FlagBPS("greed", "0", "number of basis points improvement over market midpoint to demand (or negative to increase likelihood of execution); only applies to limit orders without an explicit -price")
	flagSpread         = decimal.Flag("spread", "0", "set limit price based on bid/ask spread (e.g. -1 for making, 0 for midpoint, +1 for taking) instead of -greed based percentage")
	flagDuration       = clocky.DurationFlag("duration", "0", "duration over which to execute TWAP/VWAP orders (e.g. 1h30m; default is until end of day)")
	flagParticipation  = decimal.Flag("participation", "0.15", "maximum volume participation rate if TWAP/VWAP order")
	flagDisplay        = decimal.Flag("display", "0", "max amount of shares to display on exchange in dma mode")
	flagStop           = decimal.Flag("stop", "0", "stop loss price")
	flagTrail          = flag.String("trail", "", "trailing stop distance in dollars (e.g. 0.50) or percent (e.g. 1%)")
	flagOCO            = flag.Bool("oco", false, "use one cancels other (OCO) order class")
	flagOTO            = flag.Bool("oto", false, "use one triggers other (OTO) order class")
	flagBracket        = flag.Bool("bracket", false, "use bracket order class")
	flagStopLimit      = decimal.Flag("stop-limit", "0", "stop limit price")
	flagTakeProfit     = decimal.Flag("take", "0", "take profit price")
	flagMarket         = flag.Bool("market", false, "use market order")
	flagStrike         = decimal.Flag("strike", "0", "use strike price")
	flagExp            = clocky.TimeFlag("exp", "", "use expiration date in YYYY-MM-DD format")
	flagCall           = flag.Bool("call", false, "trade call option")
	flagPut            = flag.Bool("put", false, "trade put option")
)

func main() {

	// parse flags
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `usage: order [options] SYMBOL...
description:
  places an order for the given stock symbols via alpaca.
examples:
  order -limit=329.23 -qty=40 GOOGL                  # buy round lot of goog at specific limit price
  order -vwap -amt=10_000.00 AAPL MSFT               # buy $10,000 worth of AAPL and $10,000 worth of MSFT at midpoint price using dash's volume-weighted algorithm
  order -twap -duration=1h -greed=-5 -qty=-1000 ADBE # sell (or short) 1000 shares of Adobo for an hour allowing up to 5 basis points of slippage over mid price right now
  order -limit=7.80 -qty=-1 WDC260130P00175000       # sell 1 put option contract for WDC with strike price $175 expiring Jan 30, 2026 at limit price $7.80
  order -liquidate GOOG\*                            # close all GOOG and GOOGL stock and options positions at midpoint
options:
`)
		flag.PrintDefaults()
	}
	flag.Parse()
	client := alpaca.NewClient()

	// figure out time in force
	timeInForce := alpaca.TimeInForceDay
	if *flagGTC {
		timeInForce = alpaca.TimeInForceGTC
	}
	if *flagOPG {
		timeInForce = alpaca.TimeInForceOPG
	}
	if *flagCLS {
		timeInForce = alpaca.TimeInForceCLS
	}
	if *flagFOK {
		timeInForce = alpaca.TimeInForceFOK
	}
	if *flagIOC {
		timeInForce = alpaca.TimeInForceIOC
	}

	// validate some flags
	if flagLimit.IsNegative() {
		fmt.Fprintf(os.Stderr, "-limit cannot be negative\n")
		os.Exit(1)
	}
	if flagStop.IsNegative() {
		fmt.Fprintf(os.Stderr, "-stop cannot be negative\n")
		os.Exit(1)
	}
	if moreThanOneSpecified(!flagLimit.IsZero(), !flagGreed.IsZero(), !flagSpread.IsZero()) {
		fmt.Fprintf(os.Stderr, "-limit, -greed, and -spread are mutually exclusive\n")
		os.Exit(1)
	}
	if moreThanOneSpecified(!flagQty.IsZero(), !flagAmt.IsZero(), *flagLiquidate, *flagLiquidate, *flagLiquidateLong, *flagLiquidateShort) {
		fmt.Fprintf(os.Stderr, "-qty, -amt, -liquidate, -liquidate-long, and -liquidate-short are mutually exclusive\n")
		os.Exit(1)
	}
	if flagQty.IsZero() && flagAmt.IsZero() && !*flagLiquidate && !*flagLiquidateLong && !*flagLiquidateShort {
		fmt.Fprintf(os.Stderr, "either -qty, -amt, -liquidate, -liquidate-long, or -liquidate-short must be specified\n")
		os.Exit(1)
	}
	if moreThanOneSpecified(*flagGTC, *flagOPG, *flagCLS, *flagFOK, *flagIOC) {
		fmt.Fprintf(os.Stderr, "-gtc, -opg, -cls, -fok, and -ioc are mutually exclusive\n")
		os.Exit(1)
	}
	if moreThanOneSpecified(*flagBracket, *flagOTO, *flagOCO) {
		fmt.Fprintf(os.Stderr, "-bracket, -oto, and -oco are mutually exclusive\n")
		os.Exit(1)
	}
	if moreThanOneSpecified(*flagTWAP, *flagVWAP, *flagDMA != alpaca.OrderDestinationNone) {
		fmt.Fprintf(os.Stderr, "-twap, -vwap, and -dma are mutually exclusive\n")
		os.Exit(1)
	}
	if timeInForce != alpaca.TimeInForceDay && (*flagTWAP || *flagVWAP) {
		fmt.Fprintf(os.Stderr, "TWAP/VWAP orders must use day time in force\n")
		os.Exit(1)
	}

	// figure out order class
	orderClass := alpaca.OrderClassSimple
	if *flagBracket {
		orderClass = alpaca.OrderClassBracket
		if *flagBracket && *flagExt {
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
	}
	if *flagOCO {
		orderClass = alpaca.OrderClassOCO
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

	if !flagGreed.IsZero() && orderType != alpaca.OrderTypeLimit {
		fmt.Fprintf(os.Stderr, "-greed only works with limit orders\n")
		os.Exit(1)
	}
	if !flagSpread.IsZero() && orderType != alpaca.OrderTypeLimit {
		fmt.Fprintf(os.Stderr, "-spread only works with limit orders\n")
		os.Exit(1)
	}

	// figure out algorithm
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
		if orderClass == alpaca.OrderClassSimple {
			fmt.Fprintf(os.Stderr, "-take only works with -bracket or -oco or -oto order classes\n")
			os.Exit(1)
		}
		takeProfit = &alpaca.TakeProfit{
			LimitPrice: *flagTakeProfit,
		}
	}

	// figure out extended hours
	extendedHours := false
	if *flagExt {
		extendedHours = true
		if orderType == alpaca.OrderTypeMarket {
			fmt.Fprintf(os.Stderr, "extended hours doesn't support market orders\n")
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

	// support glob syntax for convenience based on existing positions
	var positions []alpaca.Position
	needPositions := func() {
		var err error
		positions, err = client.GetPositions()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error getting positions: %v\n", err)
			os.Exit(1)
		}
	}
	if hasGlobSyntaxInSymbols(symbols) {
		var symbols2 []string
		for _, sym := range symbols {
			if !hasGlobSyntax(sym) {
				symbols2 = append(symbols2, sym)
			} else {
				needPositions()
				gotMatch := false
				for _, pos := range positions {
					if matched, _ := path.Match(sym, pos.Symbol); matched {
						symbols2 = append(symbols2, pos.Symbol)
						gotMatch = true
					}
				}
				if !gotMatch {
					fmt.Fprintf(os.Stderr, "pattern %s did not match any existing positions\n", sym)
					os.Exit(1)
				}
			}
		}
		symbols = symbols2
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

	// catch ctrl-c
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// loop over stocks
	var err error
	exitCode := 0
	orders := map[string]*alpaca.Order{}
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
		if *flagLiquidate {
			needPositions()
			found := false
			for _, pos := range positions {
				if pos.Symbol == sym {
					if pos.Qty.IsPositive() {
						side = ds.SideSell
						qty = pos.Qty
					} else {
						side = ds.SideBuy
						qty = pos.Qty.Neg()
					}
					found = true
					break
				}
			}
			if !found {
				fmt.Fprintf(os.Stderr, "no existing position found for %s to liquidate\n", sym)
				if exitCode < 255 {
					exitCode++
				}
				continue
			}
		} else if !qty.IsZero() {
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
		explain := ""
		limitPrice := *flagLimit
		if orderType == alpaca.OrderTypeLimit && limitPrice.IsZero() {
			needQuote()
			limitPrice = bidPrice.Add(askPrice).Half()
			explain = fmt.Sprintf("%s|%s->%s", bidPrice, askPrice, limitPrice)
			if !flagGreed.IsZero() {
				if side == ds.SideBuy {
					limitPrice = limitPrice.Mul(decimal.One.Sub(*flagGreed))
				} else {
					limitPrice = limitPrice.Mul(decimal.One.Add(*flagGreed))
				}
				explain = fmt.Sprintf("%s greed->%s", explain, limitPrice)
			} else if !flagSpread.IsZero() {
				spread := askPrice.Sub(bidPrice).Half()
				adjustment := spread.Mul(*flagSpread)
				limitPrice = limitPrice.Add(adjustment)
				explain = fmt.Sprintf("%s spread->%s", explain, limitPrice)
			}
			tick := cboe.Tick01
			if len(sym) > 8 && limitPrice.Cmp(decimal.Three) > 0 {
				tick = cboe.Tick05
			}
			if side == ds.SideBuy {
				limitPrice = limitPrice.QuantizeTruncate(tick)
			} else {
				limitPrice = limitPrice.QuantizeAway(tick)
			}
			explain = fmt.Sprintf("%s round->%s", explain, limitPrice)
		}
		if explain != "" {
			explain = fmt.Sprintf(" (calculated %s)", explain)
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
		orders[order.ClientOrderID] = order

		// log the order
		fmt.Printf("%s order placed to %s %s shares of %s at %s with status %s%s\n", order.CreatedAt, side, qty, sym, limitPrice, order.Status, explain)
	}

	// print order updates until all orders are done
	for *flagWait && len(orders) > 0 {
		select {
		case orderUpdate := <-orderUpdates:
			fmt.Printf("%s order update for %s: %s price=%s status=%s qty=%s pos=%s filled=%s/%s avg_price=%s\n",
				orderUpdate.Order.UpdatedAt, orderUpdate.Order.Symbol,
				orderUpdate.Event, orderUpdate.Price, orderUpdate.Order.Status, orderUpdate.Qty, orderUpdate.PositionQty,
				orderUpdate.Order.FilledQty, orderUpdate.Order.Qty, orderUpdate.Order.FilledAvgPrice)
			orders[orderUpdate.Order.ClientOrderID] = orderUpdate.Order
			if orderUpdate.Order.Status.IsFinal() {
				delete(orders, orderUpdate.Order.ClientOrderID)
			}
		case <-sigChan:
			for _, order := range orders {
				if order.Status.IsFinal() {
					continue
				}
				err := client.CancelOrder(order.ID)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error canceling order for %s: %v\n", order.Symbol, err)
				}
			}
			exitCode = 128 + 2 // 128 + SIGINT
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

func hasGlobSyntaxInSymbols(s []string) bool {
	return slices.ContainsFunc(s, hasGlobSyntax)
}

func hasGlobSyntax(s string) bool {
	for _, c := range s {
		if c == '*' || c == '?' || c == '[' {
			return true
		}
	}
	return false
}

func moreThanOneSpecified(flags ...bool) bool {
	count := 0
	for _, flag := range flags {
		if flag {
			count++
		}
	}
	return count > 1
}
