package main

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"flag"
	"fmt"
	"os"
)

var (
	flagOPG           = flag.Bool("opg", false, "schedule order to occur in opening auction")
	flagCLS           = flag.Bool("cls", false, "schedule order to occur in closing auction")
	flagFOK           = flag.Bool("fok", false, "use fill or kill time in force")
	flagGTC           = flag.Bool("gtc", false, "use good till canceled time in force")
	flagIOC           = flag.Bool("ioc", false, "use immediate or cancel time in force")
	flagTWAP          = flag.Bool("twap", false, "use time weighted average price algorithm")
	flagVWAP          = flag.Bool("vwap", false, "use volume weighted average price algorithm")
	flagDMA           = flag.String("dma", "", "directly route order to lit exchange (NYSE, NASDAQ, ARCA)")
	flagExt           = flag.Bool("ext", false, "participate in extended hours trading (must be limit order with default (day) time in force)")
	flagLimit         = decimal.Flag("limit", "0", "sets an explicit limit price (the default is to use the midpoint plus/minus greed depending on the side) must be positve")
	flagAmt           = decimal.Flag("amt", "0", "usd notional value of order (negative to sell) which is mutually exclusive with -qty")
	flagQty           = decimal.Flag("qty", "0", "number of shares to transact (negative to sell) which is mutually exclusive with -amt")
	flagGreed         = decimal.FlagBPS("greed", "0", "number of basis points improvement over market midpoint to demand (or negative to increase likelihood of execution); only applies to limit orders without an explicit -price")
	flagDuration      = clocky.DurationFlag("duration", "0", "duration over which to execute TWAP/VWAP orders (e.g. 1h30m; default is until end of day)")
	flagParticipation = decimal.Flag("participation", "0.15", "maximum volume participation rate if TWAP/VWAP order")
	flagStop          = decimal.Flag("stop", "0", "stop loss price")
	flagOCO           = flag.Bool("oco", false, "use one cancels other (OCO) order class")
	flagOTO           = flag.Bool("oto", false, "use one triggers other (OTO) order class")
	flagBracket       = flag.Bool("bracket", false, "use bracket order class")
	flagStopLimit     = decimal.Flag("stop-limit", "0", "stop limit price")
	flagTakeProfit    = decimal.Flag("take-profit", "0", "take profit price")
	flagMarket        = flag.Bool("market", false, "use market order")
)

func main() {

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
	if (*flagQty).IsZero() && (*flagAmt).IsZero() {
		fmt.Fprintf(os.Stderr, "no -qty or -amt specified\n")
		os.Exit(1)
	}
	if (*flagLimit).IsNegative() {
		fmt.Fprintf(os.Stderr, "-limit cannot be negative\n")
		os.Exit(1)
	}
	if !(*flagLimit).IsZero() && !(*flagGreed).IsZero() {
		fmt.Fprintf(os.Stderr, "-limit and -greed cannot both be specified\n")
		os.Exit(1)
	}

	// figure out algorithm
	var endTime clocky.Time
	var maxPercentage decimal.Decimal
	orderType := alpaca.OrderTypeLimit
	if *flagMarket {
		orderType = alpaca.OrderTypeMarket
		if !(*flagLimit).IsZero() {
			fmt.Fprintf(os.Stderr, "-limit can't be used with market orders\n")
			os.Exit(1)
		}
		if !(*flagGreed).IsZero() {
			fmt.Fprintf(os.Stderr, "-greed only works with limit orders\n")
			os.Exit(1)
		}
	} else if *flagLimit <= decimal.Zero {
		fmt.Fprintf(os.Stderr, "must specify either -limit or -market\n")
		os.Exit(1)
	}
	if *flagTWAP && *flagVWAP {
		fmt.Fprintf(os.Stderr, "cannot specify both -twap and -vwap\n")
		os.Exit(1)
	}
	algorithm := alpaca.OrderAlgorithmNone
	if *flagTWAP {
		algorithm = alpaca.OrderAlgorithmTWAP
	} else if *flagVWAP {
		algorithm = alpaca.OrderAlgorithmVWAP
	}
	if algorithm != alpaca.OrderAlgorithmNone {
		if *flagDuration > 0 {
			endTime = clocky.Now().Add(*flagDuration)
		}
		maxPercentage = *flagParticipation
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
		if algorithm != alpaca.OrderAlgorithmNone {
			fmt.Fprintf(os.Stderr, "TWAP/VWAP orders cannot participate in opening auction\n")
			os.Exit(1)
		}
	}
	if *flagCLS {
		timeInForce = alpaca.TimeInForceCLS
		tifs++
		if algorithm != alpaca.OrderAlgorithmNone {
			fmt.Fprintf(os.Stderr, "TWAP/VWAP orders cannot participate in closing auction\n")
			os.Exit(1)
		}
	}
	if *flagFOK {
		timeInForce = alpaca.TimeInForceFOK
		tifs++
		if algorithm != alpaca.OrderAlgorithmNone {
			fmt.Fprintf(os.Stderr, "TWAP/VWAP orders cannot use fill or kill time in force\n")
			os.Exit(1)
		}
	}
	if *flagIOC {
		timeInForce = alpaca.TimeInForceIOC
		tifs++
		if algorithm != alpaca.OrderAlgorithmNone {
			fmt.Fprintf(os.Stderr, "TWAP/VWAP orders cannot use immediate or cancel time in force\n")
			os.Exit(1)
		}
	}
	if tifs > 1 {
		fmt.Fprintf(os.Stderr, "cannot specify more than one time in force\n")
		os.Exit(1)
	}

	// figure out order class
	orderClass := alpaca.OrderClassSimple
	if *flagBracket || *flagOTO || *flagOCO {
		if *flagBracket {
			orderClass = alpaca.OrderClassBracket
		}
		if *flagOTO {
			if orderClass != alpaca.OrderClassSimple {
				fmt.Fprintf(os.Stderr, "cannot combine -oto with other order classes\n")
				os.Exit(1)
			}
			orderClass = alpaca.OrderClassOTO
		}
		if *flagOCO {
			if orderClass != alpaca.OrderClassSimple {
				fmt.Fprintf(os.Stderr, "cannot combine -oco with other order classes\n")
				os.Exit(1)
			}
			orderClass = alpaca.OrderClassOCO
		}
	}

	// figure out stop loss
	var stopLoss *alpaca.StopLoss
	if !(*flagStop).IsZero() {
		stopLoss = &alpaca.StopLoss{
			StopPrice:  *flagStop,
			LimitPrice: *flagStopLimit,
		}
	} else if !(*flagStopLimit).IsZero() {
		fmt.Fprintf(os.Stderr, "-stop-limit requires -stop to be set\n")
		os.Exit(1)
	}

	// figure out take profit
	var takeProfit *alpaca.TakeProfit
	if !(*flagTakeProfit).IsZero() {
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
	if len(flag.Args()) == 0 {
		fmt.Fprintf(os.Stderr, "no symbols specified\n")
		os.Exit(1)
	}
	symbols := flag.Args()

	// loop over stocks
	exitCode := 0
	client := alpaca.NewClient()
	for _, sym := range symbols {

		// get quote
		quote, err := client.GetQuote(sym)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error getting quote for %s: %v\n", sym, err)
			if exitCode < 255 {
				exitCode++
			}
			continue
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
			if amt.IsNegative() {
				side = ds.SideSell
				price := quote.BidPrice
				amt = amt.Neg()
				qty = amt.Div(price).Truncate()
			} else {
				price := quote.AskPrice
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
			limitPrice = quote.BidPrice.Add(quote.AskPrice).Half()
			if side == ds.SideBuy {
				limitPrice = limitPrice.Mul(decimal.One.Sub(*flagGreed))
				limitPrice = limitPrice.QuantizeTruncate(decimal.Cent)
			} else {
				limitPrice = limitPrice.Mul(decimal.One.Add(*flagGreed))
				limitPrice = limitPrice.QuantizeAway(decimal.Cent)
			}
		}

		// figure out exchange destination
		var destination alpaca.OrderDestination
		if *flagDMA != "" {
			destination, err = alpaca.ParseOrderDestination(*flagDMA)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid DMA destination %s: %v\n", *flagDMA, err)
				if exitCode < 255 {
					exitCode++
				}
				continue
			}
		}

		// give the order
		_, err = client.CreateOrder(&alpaca.OrderRequest{
			Symbol:        sym,
			Side:          side,
			Qty:           qty,
			LimitPrice:    limitPrice,
			Type:          orderType,
			TimeInForce:   timeInForce,
			ExtendedHours: extendedHours,
			OrderClass:    orderClass,
			StopLoss:      stopLoss,
			TakeProfit:    takeProfit,
			AdvancedInstructions: &alpaca.AdvancedInstructions{
				Algorithm:     algorithm,
				EndTime:       endTime,
				MaxPercentage: maxPercentage,
				Destination:   destination,
			},
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error opening %s: %v\n", sym, err)
			if exitCode < 255 {
				exitCode++
			}
		}

		// log the order
		fmt.Printf("order placed to %s %s shares of %s at %s\n", side, qty, sym, limitPrice)
	}

	// report status to parent process
	os.Exit(exitCode)
}
