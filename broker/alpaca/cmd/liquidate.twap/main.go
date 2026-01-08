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
	flagGreed         = decimal.FlagBPS("greed", "0", "number of basis points improvement over market price to demand")
	flagDuration      = clocky.DurationFlag("duration", "0", "duration over which to execute TWAP orders (default until end of day)")
	flagParticipation = decimal.Flag("participation", "0.15", "maximum volume participation rate for TWAP orders")
)

func main() {
	flag.Parse()

	// figure out ending time for twap orders
	var endTime clocky.Time
	if *flagDuration > 0 {
		endTime = clocky.Now().Add(*flagDuration)
	}

	// get positions
	client := alpaca.NewClient()
	positions, err := client.GetPositions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting positions: %v\n", err)
		os.Exit(1)
	}

	// turn positions into a map for easy lookup
	positionsMap := make(map[symbol.Symbol]*alpaca.Position)
	for i := range positions {
		if positions[i].AssetClass == alpaca.AssetClassUSEquity {
			positionsMap[positions[i].Symbol] = &positions[i]
		}
	}

	// get list of stocks to liquidate
	// we default to everything if you pass nothing
	var symbols []symbol.Symbol
	if len(flag.Args()) == 0 {
		for _, pos := range positions {
			symbols = append(symbols, pos.Symbol)
		}
		// now display a message in the terminal asking the user to press y to confirm
		fmt.Printf("No symbols specified, defaulting to liquidate all positions: %v\n", symbols)
		fmt.Printf("Press 'y' then Enter to confirm liquidation, or any other key to cancel:\n")
		var response string
		_, err := fmt.Scanln(&response)
		if err != nil || response != "y" {
			fmt.Printf("Liquidation cancelled.\n")
			return
		}
	} else {
		for _, arg := range flag.Args() {
			sym, err := symbol.Parse(arg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid symbol %q: %v\n", arg, err)
				os.Exit(1)
			}
			symbols = append(symbols, sym)
		}
	}

	// ensure for each symbol we have a position
	for _, sym := range symbols {
		if positionsMap[sym] == nil {
			fmt.Fprintf(os.Stderr, "no open position for symbol %s\n", sym)
			os.Exit(1)
		}
	}

	// liquidate each position using a twap order
	exitCode := 0
	for _, sym := range symbols {
		var side ds.Side
		position := positionsMap[sym]
		limitPrice := position.CurrentPrice
		if position.Side == alpaca.PositionSideLong {
			side = ds.SideSell
			limitPrice = limitPrice.Mul(decimal.One.Add(*flagGreed))
			limitPrice = limitPrice.QuantizeAway(decimal.Cent)
		} else {
			side = ds.SideBuy
			limitPrice = limitPrice.Mul(decimal.One.Sub(*flagGreed))
			limitPrice = limitPrice.QuantizeTruncate(decimal.Cent)
		}
		_, err := client.CreateOrder(&alpaca.OrderRequest{
			Symbol:      sym,
			Side:        side,
			Qty:         position.Qty,
			LimitPrice:  limitPrice,
			Type:        alpaca.OrderTypeLimit,
			TimeInForce: alpaca.TimeInForceDay,
			AdvancedInstructions: &alpaca.AdvancedInstructions{
				Algorithm:     alpaca.OrderAlgorithmTWAP,
				MaxPercentage: *flagParticipation,
				EndTime:       endTime,
			},
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error liquidating %s: %v\n", position.Symbol, err)
			if exitCode < 255 {
				exitCode++
			}
		}
		fmt.Printf("liquidation order placed to %s %s shares of %s to close at %s using twap\n", side, position.Qty, position.Symbol, limitPrice)
	}

	// report status to parent process
	os.Exit(exitCode)
}
