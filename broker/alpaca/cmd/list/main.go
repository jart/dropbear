package main

import (
	"dropbear/broker/alpaca"
	"dropbear/decimal"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
)

// tristate represents an optional boolean filter.
// zero value means no filter, positive means require true, negative means require false.
type tristate int

func (t *tristate) require(b bool) bool {
	if *t == 0 {
		return true
	}
	if *t > 0 {
		return b
	}
	return !b
}

// boolFlag creates a pair of flags: -name (sets to require true) and -no-name (sets to require false)
func boolFlag(name, usage string) *tristate {
	var t tristate
	flag.BoolFunc(name, usage, func(s string) error {
		t = 1
		return nil
	})
	flag.BoolFunc("no-"+name, "require "+name+" to be false", func(s string) error {
		t = -1
		return nil
	})
	return &t
}

var (
	healthy   = flag.Bool("healthy", false, "use IsHealthy() preset (tradable, marginable, has-options, easy-to-borrow, no-ptp, standard-margin)")
	showHelp  = flag.Bool("h", false, "show help")
	classFlag = flag.String("class", "", "filter by class: equity, option, crypto, crypto-perp")
	status    = flag.String("status", "", "filter by status: active, inactive")

	tradable            *tristate
	marginable          *tristate
	shortable           *tristate
	easyToBorrow        *tristate
	fractionable        *tristate
	fractionableExt     *tristate
	hasOptions          *tristate
	optionsLateClose    *tristate
	ipo                 *tristate
	ptpNoException      *tristate
	ptpWithException    *tristate
	standardMarginLong  *tristate
	standardMarginShort *tristate
)

func init() {
	tradable = boolFlag("tradable", "can be traded on Alpaca")
	marginable = boolFlag("marginable", "can be purchased with margin")
	shortable = boolFlag("shortable", "can be sold short")
	easyToBorrow = boolFlag("easy-to-borrow", "readily available for shorting")
	fractionable = boolFlag("fractionable", "supports fractional shares")
	fractionableExt = boolFlag("fractionable-ext", "supports fractional shares in extended hours")
	hasOptions = boolFlag("has-options", "options contracts available")
	optionsLateClose = boolFlag("options-late-close", "options trade 15 min past close")
	ipo = boolFlag("ipo", "in IPO phase")
	ptpNoException = boolFlag("ptp-no-exception", "PTP subject to 10% withholding")
	ptpWithException = boolFlag("ptp-with-exception", "PTP currently exempt from withholding")
	standardMarginLong = boolFlag("standard-margin-long", "has standard 30% long margin requirement")
	standardMarginShort = boolFlag("standard-margin-short", "has standard 30% short margin requirement")
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: list [flags]\n\n")
		fmt.Fprintf(os.Stderr, "List Alpaca assets with optional filtering.\n\n")
		fmt.Fprintf(os.Stderr, "Preset:\n")
		fmt.Fprintf(os.Stderr, "  -healthy\n\tuse IsHealthy() preset\n\n")
		fmt.Fprintf(os.Stderr, "Class/Status:\n")
		fmt.Fprintf(os.Stderr, "  -class string\n\tequity, option, crypto, crypto-perp\n")
		fmt.Fprintf(os.Stderr, "  -status string\n\tactive, inactive\n\n")
		fmt.Fprintf(os.Stderr, "Boolean filters (use -name or -no-name):\n")
		fmt.Fprintf(os.Stderr, "  -tradable / -no-tradable\n")
		fmt.Fprintf(os.Stderr, "  -marginable / -no-marginable\n")
		fmt.Fprintf(os.Stderr, "  -shortable / -no-shortable\n")
		fmt.Fprintf(os.Stderr, "  -easy-to-borrow / -no-easy-to-borrow\n")
		fmt.Fprintf(os.Stderr, "  -fractionable / -no-fractionable\n")
		fmt.Fprintf(os.Stderr, "  -fractionable-ext / -no-fractionable-ext\n")
		fmt.Fprintf(os.Stderr, "  -has-options / -no-has-options\n")
		fmt.Fprintf(os.Stderr, "  -options-late-close / -no-options-late-close\n")
		fmt.Fprintf(os.Stderr, "  -ipo / -no-ipo\n")
		fmt.Fprintf(os.Stderr, "  -ptp-no-exception / -no-ptp-no-exception\n")
		fmt.Fprintf(os.Stderr, "  -ptp-with-exception / -no-ptp-with-exception\n")
		fmt.Fprintf(os.Stderr, "  -standard-margin-long / -no-standard-margin-long\n")
		fmt.Fprintf(os.Stderr, "  -standard-margin-short / -no-standard-margin-short\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  list -healthy\n")
		fmt.Fprintf(os.Stderr, "      equivalent to: -class equity -status active -tradable -marginable\n")
		fmt.Fprintf(os.Stderr, "      -has-options -easy-to-borrow -no-ptp-no-exception -no-ptp-with-exception\n")
		fmt.Fprintf(os.Stderr, "      -standard-margin-long -standard-margin-short\n")
		fmt.Fprintf(os.Stderr, "  list -tradable -has-options # tradable assets with options\n")
		fmt.Fprintf(os.Stderr, "  list -class crypto         # all crypto assets\n")
		fmt.Fprintf(os.Stderr, "  list -no-marginable        # non-marginable assets\n")
	}
	flag.Parse()

	if *showHelp {
		flag.Usage()
		os.Exit(0)
	}

	// Apply -healthy preset
	if *healthy {
		*classFlag = "equity"
		*status = "active"
		*tradable = 1
		*marginable = 1
		*hasOptions = 1
		*easyToBorrow = 1
		*ptpNoException = -1
		*ptpWithException = -1
		*standardMarginLong = 1
		*standardMarginShort = 1
	}

	standardMargin := decimal.Parse("0.3")

	var results []string
	for _, a := range alpaca.Assets {
		if !matchClass(a, *classFlag) {
			continue
		}
		if !matchStatus(a, *status) {
			continue
		}
		if !tradable.require(a.Tradable.Load()) {
			continue
		}
		if !marginable.require(a.Marginable.Load()) {
			continue
		}
		if !shortable.require(a.Shortable.Load()) {
			continue
		}
		if !easyToBorrow.require(a.EasyToBorrow.Load()) {
			continue
		}
		if !fractionable.require(a.Fractionable.Load()) {
			continue
		}
		if !fractionableExt.require(a.FractionableExtHours.Load()) {
			continue
		}
		if !hasOptions.require(a.HasOptions.Load()) {
			continue
		}
		if !optionsLateClose.require(a.OptionsLateClose.Load()) {
			continue
		}
		if !ipo.require(a.IPO.Load()) {
			continue
		}
		if !ptpNoException.require(a.PTPNoException.Load()) {
			continue
		}
		if !ptpWithException.require(a.PTPWithException.Load()) {
			continue
		}
		if !standardMarginLong.require(a.MarginRequirementLong.Load().Cmp(standardMargin) == 0) {
			continue
		}
		if !standardMarginShort.require(a.MarginRequirementShort.Load().Cmp(standardMargin) == 0) {
			continue
		}
		results = append(results, a.Symbol.String())
	}
	slices.Sort(results)
	for _, s := range results {
		fmt.Println(s)
	}
}

func matchClass(a *alpaca.Asset, class string) bool {
	if class == "" {
		return true
	}
	switch strings.ToLower(class) {
	case "equity", "us-equity", "us_equity":
		return a.Class == alpaca.AssetClassUSEquity
	case "option", "us-option", "us_option":
		return a.Class == alpaca.AssetClassUSOption
	case "crypto":
		return a.Class == alpaca.AssetClassCrypto
	case "crypto-perp", "crypto_perp":
		return a.Class == alpaca.AssetClassCryptoPerp
	default:
		fmt.Fprintf(os.Stderr, "unknown class: %s\n", class)
		os.Exit(1)
		return false
	}
}

func matchStatus(a *alpaca.Asset, status string) bool {
	if status == "" {
		return true
	}
	switch strings.ToLower(status) {
	case "active":
		return a.Status == alpaca.AssetStatusActive
	case "inactive":
		return a.Status == alpaca.AssetStatusInactive
	default:
		fmt.Fprintf(os.Stderr, "unknown status: %s\n", status)
		os.Exit(1)
		return false
	}
}
