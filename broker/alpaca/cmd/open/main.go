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
	flagShort         = flag.Bool("short", false, "open short positions instead of long")
	flagOPG           = flag.Bool("opg", false, "participate in opening auction")
	flagCLS           = flag.Bool("cls", false, "participate in closing auction")
	flagFOK           = flag.Bool("fok", false, "use fill or kill time in force")
	flagGTC           = flag.Bool("gtc", false, "use good till canceled time in force")
	flagIOC           = flag.Bool("ioc", false, "use immediate or cancel time in force")
	flagExt           = flag.Bool("ext", false, "participate in extended hours trading")
	flagTWAP          = flag.Bool("twap", false, "use TWAP algorithm")
	flagVWAP          = flag.Bool("vwap", false, "use VWAP algorithm")
	flagPercent       = decimal.FlagPercent("percent", "99", "percent of buying power to use for order quantity")
	flagGreed         = decimal.FlagBPS("greed", "0", "number of basis points improvement over market price to demand")
	flagDuration      = clocky.DurationFlag("duration", "15m", "duration over which to execute TWAP orders (default until end of day)")
	flagParticipation = decimal.Flag("participation", "0.15", "maximum volume participation rate if TWAP/VWAP order")
)

func main() {
	flag.Parse()

	// figure out algorithm
	var endTime clocky.Time
	var orderType alpaca.OrderType
	var maxPercentage decimal.Decimal
	if *flagLimit {
		orderType = alpaca.OrderTypeLimit
	} else if *flagMarket {
		orderType = alpaca.OrderTypeMarket
		if *flagGreed != decimal.Zero {
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

	// get list of stocks to buy
	if len(flag.Args()) == 0 {
		fmt.Fprintf(os.Stderr, "no symbols specified to open\n")
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

	// get account information
	client := alpaca.NewClient()
	account, err := client.GetAccount()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting account: %v\n", err)
		os.Exit(1)
	}

	// figure out buying power to use
	buyingPower := account.BuyingPower.Mul(*flagPercent)
	buyingPowerPerStock := buyingPower.DivInt(len(symbols))

	// open each position using a twap order
	exitCode := 0
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

		// figure out side
		side := ds.SideBuy
		if *flagShort {
			side = ds.SideSell
		}

		// figure out limit price
		var limitPrice decimal.Decimal
		if orderType == alpaca.OrderTypeLimit {
			limitPrice = quote.BidPrice.Add(quote.AskPrice).DivInt(2)
			if *flagShort {
				side = ds.SideSell
				limitPrice = limitPrice.Mul(decimal.One.Add(*flagGreed))
				limitPrice = limitPrice.QuantizeAway(decimal.Cent)
			} else {
				side = ds.SideBuy
				limitPrice = limitPrice.Mul(decimal.One.Sub(*flagGreed))
				limitPrice = limitPrice.QuantizeTruncate(decimal.Cent)
			}
		}

		// figure out quantity
		var quantity decimal.Decimal
		switch side {
		case ds.SideBuy:
			quantity = buyingPowerPerStock.Div(quote.AskPrice).Truncate()
		case ds.SideSell:
			quantity = buyingPowerPerStock.Div(quote.BidPrice).Truncate()
		}

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
		fmt.Printf("order placed to %s %s shares of %s to open at %s using %s over %s\n", side, quantity, sym, limitPrice, algorithm, *flagDuration)
	}

	// report status to parent process
	os.Exit(exitCode)
}
