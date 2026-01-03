package main

import (
	"dropbear/broker/alpaca"
	"dropbear/decimal"
	"dropbear/ds"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	ds.SetOffline()
	client := alpaca.NewClient()
	err := client.SyncAssets()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to sync assets: %v\n", err)
		os.Exit(1)
	}

	// now sort keys before iterating over assets
	keys := make([]string, 0, len(alpaca.Assets))
	for _, a := range alpaca.Assets {
		keys = append(keys, a.Symbol)
	}
	sort.Strings(keys)

	for _, symbol := range keys {
		a := alpaca.Assets[symbol]
		if a.Class == alpaca.AssetClassUSEquity &&
			a.Status == alpaca.AssetStatusActive &&
			a.Tradable == ds.True && a.PTPNoException == ds.False && a.PTPWithException == ds.False &&
			a.MarginRequirementLong.Cmp(decimal.Parse("0.3")) == 0 {
			name := strings.ToLower(a.Name)
			if (strings.Contains(name, "2x") ||
				strings.Contains(name, "3x") ||
				strings.Contains(name, "ultra") ||
				strings.Contains(name, "leveraged")) && (!strings.Contains(name, "short") &&
				!strings.Contains(name, "bear")) {
				fmt.Printf("%-10s %s\n", a.Symbol, a.Name)
			}
		}
	}
}
