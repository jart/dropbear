package main

import (
	"dropbear/broker/alpaca"
	"dropbear/decimal"
	"dropbear/ds"
	"fmt"
	"sort"
	"strings"
)

func main() {
	ds.SetOffline()
	assets, _ := alpaca.NewClient().GetAssets()

	// now sort keys before iterating over assets
	keys := make([]string, 0, len(assets))
	for _, a := range assets {
		keys = append(keys, a.Symbol)
	}
	sort.Strings(keys)

	for _, symbol := range keys {
		a := assets[symbol]
		if a.Class == alpaca.AssetClassUSEquity &&
			a.Status == alpaca.AssetStatusActive &&
			a.Tradable && !a.PTPNoException && !a.PTPWithException &&
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
