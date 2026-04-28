package main

import (
	"cmp"
	"dropbear/broker/alpaca"
	"dropbear/broker/alpaca/sip"
	"dropbear/decimal"
	"dropbear/symbol"
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
	healthy    = flag.Bool("healthy", false, "use IsHealthy() preset (tradable, marginable, has-options, easy-to-borrow, no-ptp, standard-margin)")
	showHelp   = flag.Bool("h", false, "show help")
	classFlag  = flag.String("class", "", "filter by class: equity, option, crypto, crypto-perp")
	status     = flag.String("status", "", "filter by status: active, inactive")
	sortVolume = flag.Bool("volume", false, "sort by previous day volume (descending), prints symbol and volume")
	sortPrice  = flag.Bool("price", false, "sort by previous day price (descending), prints symbol and price")
	notional   = flag.Bool("notional", false, "with -volume, use notional volume (shares × price) instead of share count")
	quoteFlag  = flag.Bool("quote", false, "show quote along with each symbol")
	minPrice   = decimal.Flag("min-price", "0", "filter by minimum price")
	maxPrice   = decimal.Flag("max-price", "0", "filter by maximum price")
	minSpread  = decimal.Flag("min-spread", "0", "filter by minimum spread")
	maxSpread  = decimal.Flag("max-spread", "0", "filter by maximum spread")

	otc                 *tristate
	nyse                *tristate
	amex                *tristate
	bats                *tristate
	arca                *tristate
	nasdaq              *tristate
	crypto              *tristate
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
	otc = boolFlag("otc", "is over-the-counter")
	nyse = boolFlag("nyse", "listed on NYSE")
	amex = boolFlag("amex", "listed on AMEX")
	bats = boolFlag("bats", "listed on BATS")
	arca = boolFlag("arca", "listed on ARCA")
	nasdaq = boolFlag("nasdaq", "listed on NASDAQ")
	crypto = boolFlag("crypto", "is a crypto asset")
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
		fmt.Fprintf(os.Stderr, "Output:\n")
		fmt.Fprintf(os.Stderr, "  -price\n\tsort by previous day price (descending), prints symbol and price\n")
		fmt.Fprintf(os.Stderr, "  -volume\n\tsort by previous day volume (descending), prints two columns\n")
		fmt.Fprintf(os.Stderr, "  -notional\n\twith -volume, use notional volume (shares × price)\n")
		fmt.Fprintf(os.Stderr, "  -quote\n\tshow quote along with each symbol\n\n")
		fmt.Fprintf(os.Stderr, "Boolean filters (use -name or -no-name):\n")
		fmt.Fprintf(os.Stderr, "  -otc / -no-otc\n")
		fmt.Fprintf(os.Stderr, "  -nyse / -no-nyse\n")
		fmt.Fprintf(os.Stderr, "  -amex / -no-amex\n")
		fmt.Fprintf(os.Stderr, "  -bats / -no-bats\n")
		fmt.Fprintf(os.Stderr, "  -arca / -no-arca\n")
		fmt.Fprintf(os.Stderr, "  -nasdaq / -no-nasdaq\n")
		fmt.Fprintf(os.Stderr, "  -crypto / -no-crypto\n")
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
		fmt.Fprintf(os.Stderr, "  -standard-margin-short / -no-standard-margin-short\n")
		fmt.Fprintf(os.Stderr, "  -min-price / -no-min-price\n")
		fmt.Fprintf(os.Stderr, "  -max-price / -no-max-price\n\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  list -healthy\n")
		fmt.Fprintf(os.Stderr, "      equivalent to: -no-otc -class=equity -status=active -tradable -marginable\n")
		fmt.Fprintf(os.Stderr, "      -easy-to-borrow -no-ptp-no-exception -no-ptp-with-exception -no-crypto\n")
		fmt.Fprintf(os.Stderr, "  list -tradable -has-options # tradable assets with options\n")
		fmt.Fprintf(os.Stderr, "  list -class crypto         # all crypto assets\n")
		fmt.Fprintf(os.Stderr, "  list -no-marginable        # non-marginable assets\n")
		fmt.Fprintf(os.Stderr, "  list -healthy -volume      # healthy assets sorted by volume\n")
		fmt.Fprintf(os.Stderr, "  list -healthy -volume -notional # sorted by dollar volume\n")
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
		*otc = -1
		*crypto = -1
		*tradable = 1
		*marginable = 1
		*easyToBorrow = 1
		*ptpNoException = -1
		*ptpWithException = -1
	}

	standardMargin := decimal.Parse("0.3")

	if *quoteFlag || *sortVolume || *sortPrice {
		*status = "active" // only active assets have quotes and volume
		*marginable = 1
		*crypto = -1
	}

	var results []string
	for _, a := range alpaca.Assets {
		if !matchClass(a, *classFlag) {
			continue
		}
		if !matchStatus(a, *status) {
			continue
		}
		if !otc.require(a.Exchange == alpaca.ExchangeOTC) {
			continue
		}
		if !nyse.require(a.Exchange == alpaca.ExchangeNYSE) {
			continue
		}
		if !amex.require(a.Exchange == alpaca.ExchangeAMEX) {
			continue
		}
		if !bats.require(a.Exchange == alpaca.ExchangeBATS) {
			continue
		}
		if !arca.require(a.Exchange == alpaca.ExchangeARCA) {
			continue
		}
		if !nasdaq.require(a.Exchange == alpaca.ExchangeNASDAQ) {
			continue
		}
		if !crypto.require(a.Exchange == alpaca.ExchangeCrypto) {
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

	if *quoteFlag || *sortVolume || *sortPrice {
		printWithQuotes(results, *sortPrice, *sortVolume, *notional)
	} else {
		slices.Sort(results)
		for _, s := range results {
			fmt.Println(s)
		}
	}
}

func printWithQuotes(symbols []string, sortByPrice bool, sortByVolume bool, useNotional bool) {
	client := alpaca.NewClient()
	snapshots, err := client.GetSnapshots(symbols)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching snapshots: %v\n", err)
		os.Exit(1)
	}

	type symbolVolume struct {
		symbol string
		volume decimal.Decimal
		trade  *sip.Trade
		quote  *sip.Quote
	}

	var results []symbolVolume
	for sym, snap := range snapshots {
		var vol decimal.Decimal
		if snap != nil && snap.PrevDailyBar != nil {
			vol = snap.PrevDailyBar.Volume
			if useNotional {
				vol = vol.DivInt(1000)
				vol = vol.Mul(snap.PrevDailyBar.Close)
			}
		}
		results = append(results, symbolVolume{
			symbol: sym,
			volume: vol,
			trade:  snap.LatestTrade,
			quote:  snap.LatestQuote,
		})
	}

	slices.SortFunc(results, func(a, b symbolVolume) int {
		if sortByVolume {
			if c := b.volume.Cmp(a.volume); c != 0 {
				return c // descending
			}
		} else if sortByPrice {
			if c := b.trade.Price.Cmp(a.trade.Price); c != 0 {
				return c // descending
			}
		}
		return cmp.Compare(a.symbol, b.symbol) // alphabetical tiebreaker
	})

	for _, r := range results {
		if !minPrice.IsZero() && r.quote.BidPrice.Cmp(*minPrice) < 0 {
			continue
		}
		if !maxPrice.IsZero() && r.quote.AskPrice.Cmp(*maxPrice) > 0 {
			continue
		}
		spread := r.quote.AskPrice.Sub(r.quote.BidPrice)
		if !minSpread.IsZero() && spread.Cmp(*minSpread) < 0 {
			continue
		}
		if !maxSpread.IsZero() && spread.Cmp(*maxSpread) > 0 {
			continue
		}
		var volumeStr string
		if useNotional {
			volumeStr = "$" + r.volume.FormatThousand(6) + "k"
		} else {
			volumeStr = r.volume.FormatThousand(0)
		}
		var extra string
		if *otc == 1 {
			extra += " " + r.quote.Timestamp.String()
		}
		asset := alpaca.GetAsset(symbol.MustParse(r.symbol))
		fmt.Printf("%-8s %-8s %8s bid %8s x %7d ask %8s x %7d spread %8s vol %20s%s\n",
			r.symbol, asset.Exchange, r.trade.Price, r.quote.BidPrice, r.quote.BidSize,
			r.quote.AskPrice, r.quote.AskSize, spread, volumeStr, extra)
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
