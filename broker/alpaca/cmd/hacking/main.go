package main

import (
	"dropbear/broker/alpaca"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/symbol"
	"fmt"
	"os"
	"slices"
	"strings"
)

func main() {
	client := alpaca.NewClient()
	err := client.SyncAssets()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to sync assets: %v\n", err)
		os.Exit(1)
	}

	// now sort keys before iterating over assets
	keys := make([]symbol.Symbol, 0, len(alpaca.Assets))
	for sym := range alpaca.Assets {
		keys = append(keys, sym)
	}
	slices.Sort(keys)

	for _, sym := range keys {
		a := alpaca.Assets[sym]
		if a.Class == alpaca.AssetClassUSEquity &&
			a.Status == alpaca.AssetStatusActive &&
			a.Tradable == ds.True && a.PTPNoException == ds.False && a.PTPWithException == ds.False &&
			a.MarginRequirementLong.Cmp(decimal.Parse("0.5")) <= 0 {
			name := strings.ToLower(a.Name)
			if strings.Contains(name, "2x") ||
				strings.Contains(name, "3x") ||
				strings.Contains(name, "ultra") ||
				strings.Contains(name, "leveraged") {
				fmt.Printf("%-10s %s %s\n", a.Symbol, a.MarginRequirementLong, a.Name)
			}
		}
	}
}
