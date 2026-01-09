package main

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/ds/symbol"
	"flag"
	"fmt"
	"os"
)

var (
	flagLimit         = flag.Bool("limit", false, "use limit order")
	flagMarket        = flag.Bool("market", false, "use market order")
	flagOPG           = flag.Bool("opg", false, "participate in opening auction")
	flagCLS           = flag.Bool("cls", false, "participate in closing auction")
	flagFOK           = flag.Bool("fok", false, "use fill or kill time in force")
	flagGTC           = flag.Bool("gtc", false, "use good till canceled time in force")
	flagIOC           = flag.Bool("ioc", false, "use immediate or cancel time in force")
	flagExt           = flag.Bool("ext", false, "participate in extended hours trading")
	flagTWAP          = flag.Bool("twap", false, "use time weighted average price algorithm")
	flagVWAP          = flag.Bool("vwap", false, "use volume weighted average price algorithm")
	flagPrice         = decimal.Flag("price", "0", "sets an explicit limit price")
	flagNotional      = decimal.Flag("notional", "0", "usd amount of order")
	flagQuantity      = decimal.Flag("quantity", "0", "number of shares to transact")
	flagPercent       = decimal.FlagPercent("percent", "0", "percent of buying power (if opening) or holding quantity (if closing) to use for order quantity")
	flagGreed         = decimal.FlagBPS("greed", "0", "number of basis points improvement over market price to demand")
	flagDuration      = clocky.DurationFlag("duration", "0", "duration over which to execute TWAP/VWAP orders (default until end of day)")
	flagParticipation = decimal.Flag("participation", "0.15", "maximum volume participation rate if TWAP/VWAP order")
)

func main() {
	flag.Parse()

	// validate some flags
	if (*flagQuantity).IsZero() && (*flagNotional).IsZero() {
		fmt.Fprintf(os.Stderr, "no -quantity or -notional amount specified\n")
		os.Exit(1)
	}
	if (*flagPrice).IsNegative() {
		fmt.Fprintf(os.Stderr, "-price cannot be negative\n")
		os.Exit(1)
	}
	if !(*flagPrice).IsZero() && !(*flagGreed).IsZero() {
		fmt.Fprintf(os.Stderr, "-price and -greed cannot both be specified\n")
		os.Exit(1)
	}

	// figure out algorithm
	var endTime clocky.Time
	var orderType alpaca.OrderType
	var maxPercentage decimal.Decimal
	if *flagLimit {
		orderType = alpaca.OrderTypeLimit
	} else if *flagMarket {
		orderType = alpaca.OrderTypeMarket
		if !(*flagPrice).IsZero() {
			fmt.Fprintf(os.Stderr, "-price only works with limit orders\n")
			os.Exit(1)
		}
		if !(*flagGreed).IsZero() {
			fmt.Fprintf(os.Stderr, "-greed only works with limit orders\n")
			os.Exit(1)
		}
	} else {
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
	var symbols []symbol.Symbol
	for _, arg := range flag.Args() {
		sym, err := symbol.Parse(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid symbol %q: %v\n", arg, err)
			os.Exit(1)
		}
		symbols = append(symbols, sym)
	}

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

		// figure out quantity
		side := ds.SideBuy
		quantity := *flagQuantity
		if !quantity.IsZero() {
			if quantity.IsNegative() {
				side = ds.SideSell
				quantity = quantity.Neg()
			}
		} else {
			price := decimal.Zero
			notional := *flagNotional
			if notional.IsNegative() {
				side = ds.SideSell
				price := quote.BidPrice
				notional = notional.Neg()
				quantity = notional.Div(price).Truncate()
			} else {
				price := quote.AskPrice
				quantity = notional.Div(price).Truncate()
			}
			if quantity.IsZero() {
				fmt.Fprintf(os.Stderr, "notional amount %s is too small to %s any shares of %s at price %s\n", notional, side, sym, price)
				if exitCode < 255 {
					exitCode++
				}
				continue
			}
		}

		// figure out our price
		limitPrice := *flagPrice
		if orderType == alpaca.OrderTypeLimit && limitPrice.IsZero() {
			limitPrice = quote.BidPrice.Add(quote.AskPrice).DivInt(2)
			if side == ds.SideBuy {
				limitPrice = limitPrice.Mul(decimal.One.Sub(*flagGreed))
				limitPrice = limitPrice.QuantizeTruncate(decimal.Cent)
			} else {
				limitPrice = limitPrice.Mul(decimal.One.Add(*flagGreed))
				limitPrice = limitPrice.QuantizeAway(decimal.Cent)
			}
		}

		// give the order
		_, err = client.CreateOrder(&alpaca.OrderRequest{
			Symbol:        sym,
			Side:          side,
			Qty:           quantity,
			LimitPrice:    limitPrice,
			Type:          orderType,
			TimeInForce:   timeInForce,
			ExtendedHours: extendedHours,
			AdvancedInstructions: &alpaca.AdvancedInstructions{
				Algorithm:     algorithm,
				EndTime:       endTime,
				MaxPercentage: maxPercentage,
			},
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error opening %s: %v\n", sym, err)
			if exitCode < 255 {
				exitCode++
			}
		}

		// log the order
		fmt.Printf("order placed to %s %s shares of %s at %s\n", side, quantity, sym, limitPrice)
	}

	// report status to parent process
	os.Exit(exitCode)
}
