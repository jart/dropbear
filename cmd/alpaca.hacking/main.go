package main

import (
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/exchange/alpaca"
	"fmt"
	"strings"
)

func main() {
	ds.SetOffline()
	assets, _ := alpaca.NewClient().GetAssets()
	for _, a := range assets {
		if a.Class == alpaca.AssetClassUSEquity &&
			a.Status == alpaca.AssetStatusActive &&
			a.Tradable && !a.PTPNoException && !a.PTPWithException &&
			a.MarginRequirementLong.Cmp(decimal.Parse("0.3")) == 0 {
			name := strings.ToLower(a.Name)
			if (strings.Contains(name, "2x") ||
				strings.Contains(name, "3x") ||
				strings.Contains(name, "ultra") ||
				strings.Contains(name, "leveraged")) && (!strings.Contains(a.Name, "short") &&
				!strings.Contains(a.Name, "bear")) {
				fmt.Printf("%-10s %s\n", a.Symbol, a.Name)
			}
		}
	}
}
