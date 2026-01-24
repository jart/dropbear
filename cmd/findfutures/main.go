// findfutures finds actionable futures trades on CME Globex.
//
// This command enumerates all possible trades you can punch into thinkorswim or
// ninjatrader RIGHT NOW, with current prices and margins. It shows:
//
//   - Outright positions (long/short a single contract)
//   - Calendar spreads (long one month, short another)
//
// For each trade, it provides signals to help you decide:
//
//   - For FX: The carry (interest rate differential) you earn by holding
//   - For commodities: The curve state (backwardation/contango) as a directional signal
//   - For spreads: The spread value you can lock in today
//
// Run with -tutorial for detailed examples of how to use this output.
package main

import (
	"bytes"
	"cmp"
	"dropbear/broker/databento"
	"dropbear/db"
	"dropbear/loggy"
	"dropbear/netty"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
	"unsafe"
)

func init() {
	netty.SetOffline()
}

var (
	flagSync     = flag.Bool("sync", false, "fetch latest data from Databento")
	flagAsset    = flag.String("asset", "", "filter to specific asset (e.g., 6J, ES, CL)")
	flagType     = flag.String("type", "", "filter by type: currency, index, energy, metal, ag, rate")
	flagSpreads  = flag.Bool("spreads", false, "show calendar spreads instead of outrights")
	flagUSRate   = flag.Float64("us-rate", 4.5, "current US risk-free rate for FX carry calc")
	flagTutorial = flag.Bool("tutorial", false, "show detailed tutorial with example scenarios")
)

const tutorial = `
FINDFUTURES TRADING TUTORIAL
============================

This tool shows you actionable futures trades with signals to help decide direction.
There are two modes: outrights (default) and spreads (-spreads).

OUTRIGHT POSITIONS
------------------
An outright is simply going long or short a single futures contract.

Example output (from real data):

  CONTRACT   TYPE          PRICE     MARGIN    CURVE   CARRY% SIGNAL
  ----------------------------------------------------------------------
  CLG6       energy      60.5300      $6500  backwrd  -20.24%   LONG
  ZWH6       ag           515.75      $1650 contango   13.05%  SHORT
  6SH6       currency     1.2740      $4500 contango    3.97%  SHORT
  6MH6       currency    0.05696      $2500  backwrd   -3.90%   LONG

How to read this:

  CLG6 (Crude Oil, Feb 2026):
  - Curve is in backwardation (front months cost more than back)
  - The -20.24% carry means front is 20% HIGHER than back annualized
  - Signal says LONG: backwardation = tight supply = bullish
  - In thinkorswim: Buy /CLG6
  - WARNING: You're exposed to crude price moves! If crude drops $5,
    you lose ~$5,000 per contract regardless of curve shape.

  ZWH6 (Wheat, March 2026):
  - Curve is in contango (13.05% annualized)
  - Back months cost MORE than front
  - Signal says SHORT: contango = oversupply = bearish
  - In thinkorswim: Sell /ZWH6
  - WARNING: Same caveat - full price exposure

  6SH6 (Swiss Franc, March 2026):
  - Curve is in contango (3.97%)
  - This means US rates > Swiss rates by about 3.97%
  - Signal says SHORT: you EARN 3.97% annualized by being short
  - In thinkorswim: Sell /6SH6
  - The futures price decays toward spot, you pocket the difference
  - Risk: CHF strengthens against USD (flight to safety)

  6MH6 (Mexican Peso, March 2026):
  - Curve is in backwardation (-3.90%)
  - This means Mexican rates > US rates
  - Signal says LONG: you EARN the rate differential by being long
  - In thinkorswim: Buy /6MH6
  - Risk: Peso weakens against USD

KEY INSIGHT FOR COMMODITIES:
The curve state (backwardation/contango) is a SIGNAL about supply/demand, not a
return you automatically collect. You profit from price movement in the indicated
direction. If crude drops $5 while you're long, you lose ~$5,000 per contract
regardless of what the curve looks like.

KEY INSIGHT FOR FX:
The carry IS a return you collect over time. If you're short 6S at 3.97% carry,
you're earning the US-Swiss interest rate differential. The futures price
gradually converges toward spot, and you pocket the difference. You still have
FX exposure (CHF strengthening hurts you), but the carry is real and accrues
daily through mark-to-market.

CALENDAR SPREADS (-spreads)
---------------------------
A calendar spread is simultaneously long one month and short another. This
ISOLATES the curve shape from outright price movement.

Example output (from real data):

  SPREAD             TYPE          PRICE     MARGIN   CARRY%     EDGE
  ----------------------------------------------------------------------
  CLG6-CLH6          energy      0.94000       $975  -20.24%  backwrd
  ZWH6-ZWK6          ag         -11.2500       $412   13.05% contango
  ZCH6-ZCK6          ag          -8.2500       $244   11.64% contango
  GCG6-GCJ6          metal      -36.1000      $2488    4.50% contango

How to read this:

  CLG6-CLH6 (Crude Feb-Mar spread):
  - Current spread is $0.94 (Feb is $0.94 HIGHER than Mar)
  - Margin is only $975 (vs $6,500 for outright!)
  - "backwrd" edge means backwardation - front premium
  - In thinkorswim: BUY the /CLG6-/CLH6 calendar spread
  - You profit if backwardation STEEPENS (spread widens to say $1.50)
    Profit = ($1.50 - $0.94) * 1000 = $560
  - You lose if it FLATTENS (spread narrows to say $0.50)
    Loss = ($0.50 - $0.94) * 1000 = -$440

  ZWH6-ZWK6 (Wheat Mar-May spread):
  - Current spread is -$11.25 (Mar is $11.25 BELOW May)
  - "contango" edge
  - In thinkorswim: SELL the /ZWH6-/ZWK6 spread
  - You profit if contango NARROWS (spread goes from -11.25 toward 0)
  - You lose if contango STEEPENS (spread goes to -15)

WHY USE SPREADS?
- Much lower margin (often 1/5th to 1/10th of outright)
- Isolated from outright price moves (crude can drop $10 and your spread barely moves)
- You're trading the CURVE SHAPE, not the commodity price
- Professionals use spreads to express views on supply/demand dynamics

TRADING DECISIONS FRAMEWORK
---------------------------

1. CURRENCY FUTURES (carry is king):
   - Positive carry (contango) = go SHORT (you earn the rate differential)
   - Negative carry (backwardation) = go LONG (you earn the rate differential)
   - The carry IS your expected return (plus/minus spot FX movement)
   - Example: 6S at 3.97% contango, short it, earn ~3.97% annualized

2. COMMODITY OUTRIGHTS (curve as signal):
   - Backwardation (negative carry) = LONG signal (tight supply, bullish)
   - Contango (positive carry) = SHORT signal (oversupply, bearish)
   - But YOU'RE EXPOSED TO PRICE MOVES - the curve is a hint, not a guarantee
   - Use this alongside your own chart analysis

3. COMMODITY SPREADS (pure curve trading):
   - Backwardation: BUY the spread to profit if it steepens
   - Contango: SELL the spread to profit if it narrows
   - Lower risk than outrights, but lower absolute profit potential
   - Great for expressing a view on supply/demand without price direction risk

PRACTICAL EXAMPLES
------------------

Scenario 1: You see CL in steep backwardation (-20% carry)
  - This suggests tight supply, inventory draws
  - Outright play: Long /CLG6 at $60.53, $6,500 margin
    Risk: Crude drops to $54, you lose $6,000
  - Spread play: Buy /CLG6-/CLH6 at $0.94, $975 margin
    If backwardation steepens to $1.50: profit $560
    If it flattens to $0.50: loss $440
    Crude price movement barely affects you!

Scenario 2: You see 6S with 3.97% carry (contango)
  - US rates (~4.5%) higher than Swiss rates (~0.5%)
  - Short /6SH6: you earn ~3.97% annualized as futures price decays
  - Risk: CHF strengthens against dollar (flight to safety, etc.)
  - This is a "carry trade" - you're the bank earning the rate spread

Scenario 3: You see 6M with -3.90% carry (backwardation)
  - Mexican rates (~8.4%) higher than US rates (~4.5%)
  - Long /6MH6: you earn the ~3.9% differential
  - Risk: Peso weakens (EM currency crisis, etc.)
  - High-yield EM carry trade

Scenario 4: Wheat in contango (+13% carry)
  - Oversupply situation, storage costs priced in
  - Outright play: Short /ZWH6 betting prices fall
  - Spread play: Sell /ZWH6-/ZWK6 at -$11.25
    Profit if contango narrows (spread moves toward 0)
    Lower risk than betting on outright wheat price
`

// Trade represents an actionable trade
type Trade struct {
	Symbol         string  // e.g., "CLG6" or "CLG6-CLH6"
	Asset          string  // e.g., "CL"
	TradeType      string  // "outright" or "spread"
	AssetType      string  // "currency", "energy", "metal", "ag", "index", "rate"
	Direction      string  // "LONG" or "SHORT"
	Price          float64 // current price or spread value
	Margin         float64 // required margin
	Notional       float64 // contract notional value
	Leverage       float64 // notional / margin
	Carry          float64 // annualized carry (positive = earn, negative = pay)
	ReturnOnMargin float64 // annualized return on margin (positive = earn, negative = pay)
	CurveState     string  // "contango", "backwrd", or "flat"
	DaysToExp      int     // days to front month expiration
}

// FuturesContract represents a single futures contract
type FuturesContract struct {
	Symbol          string
	Asset           string
	Expiration      time.Time
	SettlementPrice float64
	OpenInterest    int64
	Volume          uint64
}

// CME maintenance margin requirements (USD)
var marginRequirements = map[string]float64{
	// FX - G10
	"6J": 2800, "6E": 2700, "6S": 4500, "6A": 1900, "6B": 2000,
	"6C": 1000, "6N": 1300, "6M": 2500,
	"E7": 1350, "J7": 1400,
	"M6E": 270, "M6A": 190, "M6B": 200, "MCD": 100, "MSF": 450, "MJY": 280,
	// FX - EM
	"6L": 4000, "6R": 5000, "6Z": 3000, "PLS": 2000, "CZK": 1500,
	"HUF": 2000, "ILS": 1500, "KRW": 2000, "CNH": 3500,
	// Equity Index
	"ES": 22731, "NQ": 33563, "YM": 14245, "RTY": 9491, "EMD": 15000, "NKD": 12000, "NIY": 1200000,
	"MES": 2273, "MNQ": 3356, "MYM": 1424, "M2K": 949,
	"XAF": 2000, "XAK": 3500, "XAE": 4000, "XAU": 1500, "XAV": 2500,
	"XAB": 1500, "XAY": 2500, "XAI": 2000, "XAP": 2000,
	// Interest Rates
	"ZT": 1000, "Z3N": 1200, "ZF": 1500, "ZN": 2000, "TN": 2500, "ZB": 3500, "UB": 5000,
	"SR3": 500, "SR1": 300, "FF": 300, "GE": 500,
	// Energy
	"CL": 6500, "QM": 3250, "MCL": 650,
	"BZ": 7000,
	"NG": 3000, "QG": 1500,
	"HO": 7500, "RB": 7000,
	// Metals
	"GC": 24881, "SI": 45417, "HG": 10000, "PL": 3500, "PA": 15000,
	"MGC": 2488, "SIL": 9000, "QC": 2500, "ALI": 3000,
	// Agriculture
	"ZC": 975, "ZS": 2000, "ZW": 1650, "KE": 1800, "ZO": 800,
	"ZM": 1800, "ZL": 1200,
	"XC": 195, "XW": 330, "XK": 400,
	// Livestock
	"LE": 2200, "GF": 3500, "HE": 1800,
	// Dairy
	"DC": 1500, "DY": 800, "CB": 2000, "CSC": 1500,
	// Other
	"LBS": 3500, "VX": 8000,
	// Crypto
	"BTC": 50000, "MBT": 5000, "ETH": 8000, "MET": 800,
}

// Spread margin is typically much lower than outright
// These are approximate calendar spread margins
var spreadMarginRatio = map[string]float64{
	// FX spreads are about 15-20% of outright
	"6J": 0.18, "6E": 0.18, "6S": 0.18, "6A": 0.18, "6B": 0.18,
	"6C": 0.18, "6N": 0.18, "6M": 0.18,
	// Energy spreads are about 15% of outright
	"CL": 0.15, "NG": 0.20, "HO": 0.15, "RB": 0.15, "BZ": 0.15,
	// Metals about 10-15%
	"GC": 0.10, "SI": 0.10, "HG": 0.15,
	// Grains about 20-25%
	"ZC": 0.25, "ZS": 0.20, "ZW": 0.25, "ZM": 0.20, "ZL": 0.20,
	// Index about 10%
	"ES": 0.10, "NQ": 0.10, "YM": 0.10, "RTY": 0.10,
}

// Contract multipliers
var contractMultipliers = map[string]float64{
	// FX
	"6J": 12500000, "6E": 125000, "6S": 125000, "6A": 100000, "6B": 62500,
	"6C": 100000, "6N": 100000, "6M": 500000,
	"E7": 62500, "J7": 6250000,
	"M6E": 12500, "M6A": 10000, "M6B": 6250, "MCD": 10000, "MSF": 12500, "MJY": 1250000,
	"6L": 100000, "6R": 2500000, "6Z": 500000, "PLS": 500000, "CZK": 4000000,
	"HUF": 30000000, "ILS": 1000000, "KRW": 125000000, "CNH": 100000,
	// Equity Index
	"ES": 50, "NQ": 20, "YM": 5, "RTY": 50, "EMD": 100, "NKD": 5, "NIY": 500,
	"MES": 5, "MNQ": 2, "MYM": 0.5, "M2K": 5,
	"XAF": 100, "XAK": 100, "XAE": 100, "XAU": 100, "XAV": 100,
	"XAB": 100, "XAY": 100, "XAI": 100, "XAP": 100,
	// Interest Rates
	"ZT": 2000, "Z3N": 1000, "ZF": 1000, "ZN": 1000, "TN": 1000, "ZB": 1000, "UB": 1000,
	"SR3": 2500, "SR1": 4167, "FF": 4167, "GE": 2500,
	// Energy
	"CL": 1000, "QM": 500, "MCL": 100,
	"BZ": 1000,
	"NG": 10000, "QG": 2500,
	"HO": 42000, "RB": 42000,
	// Metals
	"GC": 100, "SI": 5000, "HG": 25000, "PL": 50, "PA": 100,
	"MGC": 10, "SIL": 1000, "QC": 12500, "ALI": 25,
	// Agriculture (bushels, quoted in cents so ÷100)
	"ZC": 50, "ZS": 50, "ZW": 50, "KE": 50, "ZO": 50,
	"ZM": 1, "ZL": 0.6,
	"XC": 10, "XW": 10, "XK": 10,
	// Livestock
	"LE": 400, "GF": 500, "HE": 400,
	// Dairy
	"DC": 2000, "DY": 440, "CB": 200, "CSC": 200,
	// Other
	"LBS": 27.5, "VX": 1000,
	// Crypto
	"BTC": 5, "MBT": 0.1, "ETH": 50, "MET": 0.1,
}

func main() {
	flag.Parse()
	loggy.Init()

	if *flagTutorial {
		fmt.Print(tutorial)
		return
	}

	if *flagSync {
		if err := syncData(); err != nil {
			loggy.Fatalf("sync failed: %v", err)
		}
	}

	contracts, err := loadContracts()
	if err != nil {
		loggy.Fatalf("load contracts: %v", err)
	}

	if len(contracts) == 0 {
		fmt.Println("No contracts found. Run with -sync to fetch data from Databento.")
		os.Exit(1)
	}

	var trades []Trade
	if *flagSpreads {
		trades = generateSpreadTrades(contracts)
	} else {
		trades = generateOutrightTrades(contracts)
	}

	// Filter
	var filtered []Trade
	for _, t := range trades {
		if *flagAsset != "" && t.Asset != *flagAsset {
			continue
		}
		if *flagType != "" && t.AssetType != *flagType {
			continue
		}
		filtered = append(filtered, t)
	}

	// Sort by return on margin descending (best opportunities first)
	slices.SortFunc(filtered, func(a, b Trade) int {
		return cmp.Compare(b.ReturnOnMargin, a.ReturnOnMargin)
	})

	// Print
	if *flagSpreads {
		printSpreads(filtered)
	} else {
		printOutrights(filtered)
	}
}

func generateOutrightTrades(contracts []FuturesContract) []Trade {
	// Group by asset, find front month
	byAsset := make(map[string][]FuturesContract)
	for _, c := range contracts {
		if c.SettlementPrice > 0 {
			byAsset[c.Asset] = append(byAsset[c.Asset], c)
		}
	}

	var trades []Trade
	now := time.Now()

	for asset, cs := range byAsset {
		if len(cs) < 2 {
			continue // Need at least 2 months to determine curve
		}

		// Sort by expiration
		slices.SortFunc(cs, func(a, b FuturesContract) int {
			return a.Expiration.Compare(b.Expiration)
		})

		front := cs[0]
		back := cs[1]

		// Calculate carry from front to back
		daysBetween := int(back.Expiration.Sub(front.Expiration).Hours() / 24)
		if daysBetween <= 0 {
			continue
		}

		priceDiff := back.SettlementPrice - front.SettlementPrice
		carry := (priceDiff / front.SettlementPrice) * (365.0 / float64(daysBetween))

		curveState := "flat"
		if carry > 0.01 {
			curveState = "contango"
		} else if carry < -0.01 {
			curveState = "backwrd"
		}

		assetType := classifyAsset(asset)
		margin := marginRequirements[asset]
		multiplier := contractMultipliers[asset]
		notional := front.SettlementPrice * multiplier
		daysToExp := int(front.Expiration.Sub(now).Hours() / 24)

		// Calculate return on margin (leverage factor)
		leverage := 0.0
		if margin > 0 && notional > 0 {
			leverage = notional / margin
		}

		// For SHORT: you earn carry in contango (positive carry), pay in backwardation
		// For LONG: you earn carry in backwardation (negative carry), pay in contango
		shortCarry := carry // positive = earn, negative = pay
		longCarry := -carry // opposite of short
		shortRoM := shortCarry * leverage
		longRoM := longCarry * leverage

		// Add SHORT entry
		trades = append(trades, Trade{
			Symbol:         front.Symbol,
			Asset:          asset,
			TradeType:      "outright",
			AssetType:      assetType,
			Direction:      "SHORT",
			Price:          front.SettlementPrice,
			Margin:         margin,
			Notional:       notional,
			Leverage:       leverage,
			Carry:          shortCarry,
			ReturnOnMargin: shortRoM,
			CurveState:     curveState,
			DaysToExp:      daysToExp,
		})

		// Add LONG entry
		trades = append(trades, Trade{
			Symbol:         front.Symbol,
			Asset:          asset,
			TradeType:      "outright",
			AssetType:      assetType,
			Direction:      "LONG",
			Price:          front.SettlementPrice,
			Margin:         margin,
			Notional:       notional,
			Leverage:       leverage,
			Carry:          longCarry,
			ReturnOnMargin: longRoM,
			CurveState:     curveState,
			DaysToExp:      daysToExp,
		})
	}

	return trades
}

func generateSpreadTrades(contracts []FuturesContract) []Trade {
	// Group by asset
	byAsset := make(map[string][]FuturesContract)
	for _, c := range contracts {
		if c.SettlementPrice > 0 {
			byAsset[c.Asset] = append(byAsset[c.Asset], c)
		}
	}

	var trades []Trade
	now := time.Now()

	for asset, cs := range byAsset {
		if len(cs) < 2 {
			continue
		}

		// Sort by expiration
		slices.SortFunc(cs, func(a, b FuturesContract) int {
			return a.Expiration.Compare(b.Expiration)
		})

		assetType := classifyAsset(asset)
		outrightMargin := marginRequirements[asset]
		spreadRatio := spreadMarginRatio[asset]
		if spreadRatio == 0 {
			spreadRatio = 0.20 // default 20%
		}

		// Generate adjacent month spreads
		for i := 0; i < len(cs)-1; i++ {
			front := cs[i]
			back := cs[i+1]

			spreadValue := front.SettlementPrice - back.SettlementPrice
			daysBetween := int(back.Expiration.Sub(front.Expiration).Hours() / 24)
			if daysBetween <= 0 {
				continue
			}

			// Carry for this spread
			carry := (spreadValue / front.SettlementPrice) * (365.0 / float64(daysBetween)) * -1

			curveState := "flat"
			if spreadValue > 0 {
				curveState = "backwrd" // front > back
			} else if spreadValue < 0 {
				curveState = "contango" // front < back
			}

			spreadMargin := outrightMargin * spreadRatio
			multiplier := contractMultipliers[asset]
			// For spreads, the "notional" exposure is the spread value * multiplier
			spreadNotional := math.Abs(spreadValue) * multiplier
			daysToExp := int(front.Expiration.Sub(now).Hours() / 24)

			// Return on margin for spread
			// This represents how much the spread would need to move (as % of notional)
			// to generate returns on your margin
			returnOnMargin := 0.0
			if spreadMargin > 0 && multiplier > 0 {
				// For spreads, we show potential return if spread moves by carry amount
				absCarry := carry
				if absCarry < 0 {
					absCarry = -absCarry
				}
				// Use front price notional for consistency
				frontNotional := front.SettlementPrice * multiplier
				returnOnMargin = absCarry * (frontNotional / spreadMargin)
			}

			trades = append(trades, Trade{
				Symbol:         fmt.Sprintf("%s-%s", front.Symbol, back.Symbol),
				Asset:          asset,
				TradeType:      "spread",
				AssetType:      assetType,
				Direction:      "BUY",
				Price:          spreadValue,
				Margin:         spreadMargin,
				Notional:       spreadNotional,
				Carry:          carry,
				ReturnOnMargin: returnOnMargin,
				CurveState:     curveState,
				DaysToExp:      daysToExp,
			})
		}
	}

	return trades
}

func printOutrights(trades []Trade) {
	// Header
	fmt.Printf("%-10s %-5s %-8s %10s %7s %9s %9s %9s\n",
		"CONTRACT", "DIR", "TYPE", "MARGIN", "CARRY", "RIGHT10%", "RIGHT20%", "RIGHT40%")
	fmt.Println(strings.Repeat("-", 80))

	for _, t := range trades {
		marginStr := "-"
		if t.Margin > 0 {
			marginStr = fmt.Sprintf("$%.0f", t.Margin)
		}

		// Calculate net return on margin for different directional moves
		// "RIGHT X%" means your directional bet was correct by X%
		// LONG right = price UP, SHORT right = price DOWN
		right10 := t.Carry + 0.10*t.Leverage
		right20 := t.Carry + 0.20*t.Leverage
		right40 := t.Carry + 0.40*t.Leverage

		fmt.Printf("%-10s %-5s %-8s %10s %+6.0f%% %+8.0f%% %+8.0f%% %+8.0f%%\n",
			t.Symbol,
			t.Direction,
			t.AssetType,
			marginStr,
			t.ReturnOnMargin*100,
			right10*100,
			right20*100,
			right40*100)
	}

	if len(trades) == 0 {
		fmt.Println("No trades found matching criteria.")
	} else {
		fmt.Printf("\n%d positions shown. Use -spreads to see calendar spreads.\n", len(trades))
		fmt.Println("CARRY = annual return on margin if price stays flat (+ = earn, - = pay)")
		fmt.Println("RIGHT X% = total return on margin if your direction is correct by X%")
	}
}

func printSpreads(trades []Trade) {
	fmt.Printf("%-18s %-8s %12s %10s %8s %10s %8s\n",
		"SPREAD", "TYPE", "PRICE", "MARGIN", "CARRY%", "RET/MARGIN", "EDGE")
	fmt.Println(strings.Repeat("-", 82))

	for _, t := range trades {
		marginStr := "-"
		if t.Margin > 0 {
			marginStr = fmt.Sprintf("$%.0f", t.Margin)
		}

		priceStr := formatPrice(t.Price)

		retMarginStr := "-"
		if t.ReturnOnMargin > 0 {
			retMarginStr = fmt.Sprintf("%.0f%%", t.ReturnOnMargin*100)
		}

		fmt.Printf("%-18s %-8s %12s %10s %7.2f%% %10s %8s\n",
			t.Symbol,
			t.AssetType,
			priceStr,
			marginStr,
			t.Carry*100,
			retMarginStr,
			t.CurveState)
	}

	if len(trades) == 0 {
		fmt.Println("No spreads found matching criteria.")
	} else {
		fmt.Printf("\n%d spreads shown.\n", len(trades))
		fmt.Println("Use -tutorial for help interpreting this output.")
	}
}

func formatPrice(price float64) string {
	absPrice := math.Abs(price)
	if absPrice >= 100 {
		return fmt.Sprintf("%.2f", price)
	} else if absPrice >= 1 {
		return fmt.Sprintf("%.4f", price)
	} else if absPrice >= 0.01 {
		return fmt.Sprintf("%.5f", price)
	}
	return fmt.Sprintf("%.6f", price)
}

func classifyAsset(asset string) string {
	if strings.HasPrefix(asset, "6") || asset == "E7" || asset == "J7" ||
		strings.HasPrefix(asset, "M6") || asset == "MCD" || asset == "MSF" || asset == "MJY" ||
		asset == "CNH" || asset == "PLS" || asset == "CZK" || asset == "HUF" || asset == "ILS" || asset == "KRW" {
		return "currency"
	}

	indexAssets := map[string]bool{
		"ES": true, "NQ": true, "YM": true, "RTY": true, "EMD": true, "NKD": true, "NIY": true,
		"MES": true, "MNQ": true, "MYM": true, "M2K": true,
		"XAF": true, "XAK": true, "XAE": true, "XAU": true, "XAV": true,
		"XAB": true, "XAY": true, "XAI": true, "XAP": true,
	}
	if indexAssets[asset] {
		return "index"
	}

	rateAssets := map[string]bool{
		"ZT": true, "Z3N": true, "ZF": true, "ZN": true, "TN": true, "ZB": true, "UB": true,
		"GE": true, "SR3": true, "SR1": true, "FF": true,
	}
	if rateAssets[asset] {
		return "rate"
	}

	energyAssets := map[string]bool{
		"CL": true, "QM": true, "MCL": true, "BZ": true,
		"NG": true, "QG": true, "HO": true, "RB": true,
	}
	if energyAssets[asset] {
		return "energy"
	}

	metalAssets := map[string]bool{
		"GC": true, "SI": true, "HG": true, "PL": true, "PA": true,
		"MGC": true, "SIL": true, "QC": true, "ALI": true,
	}
	if metalAssets[asset] {
		return "metal"
	}

	agAssets := map[string]bool{
		"ZC": true, "ZS": true, "ZW": true, "ZM": true, "ZL": true, "KE": true, "ZO": true,
		"XC": true, "XW": true, "XK": true,
		"LE": true, "HE": true, "GF": true,
		"DC": true, "DY": true, "CB": true, "CSC": true,
		"LBS": true,
	}
	if agAssets[asset] {
		return "ag"
	}

	if asset == "VX" {
		return "vol"
	}

	if asset == "BTC" || asset == "MBT" || asset == "ETH" || asset == "MET" {
		return "crypto"
	}

	return "other"
}

// ============================================================================
// Data fetching (unchanged from original)
// ============================================================================

func syncData() error {
	apiKey, err := databento.GetKey()
	if err != nil {
		return fmt.Errorf("get API key: %w (create ~/.databento.key)", err)
	}

	conn := db.Get()
	_, err = conn.Exec(`
		CREATE TABLE IF NOT EXISTS futures_contracts (
			symbol TEXT PRIMARY KEY,
			asset TEXT NOT NULL,
			expiration INTEGER,
			settlement_price REAL,
			open_interest INTEGER,
			volume INTEGER,
			updated_at INTEGER
		)
	`)
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}
	_, _ = conn.Exec(`CREATE INDEX IF NOT EXISTS idx_futures_asset ON futures_contracts(asset)`)

	symbols := generateSymbols()
	log.Printf("fetching %d CME futures contracts from Databento...", len(symbols))

	yesterday := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")

	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO futures_contracts
		(symbol, asset, expiration, settlement_price, open_interest, volume, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	stored := 0
	errors := 0

	client := &http.Client{Timeout: 30 * time.Second}
	rateLimiter := netty.NewTokenBucketPerMinute(100)

	for _, sym := range symbols {
		rateLimiter.Get()

		price, volume, err := fetchSymbolPrice(client, apiKey, sym, yesterday, today)
		if err != nil {
			errors++
			continue
		}

		if price <= 0 {
			continue
		}

		asset, expiry := parseSymbol(sym)
		if asset == "" {
			continue
		}

		_, err = stmt.Exec(sym, asset, expiry.UnixNano(), price, 0, volume, now)
		if err != nil {
			log.Printf("error storing %s: %v", sym, err)
			continue
		}
		stored++
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Printf("stored %d contracts (%d errors)", stored, errors)
	return nil
}

func generateSymbols() []string {
	assets := []struct {
		name   string
		months string
	}{
		// Equity Index
		{"ES", "HMUZ"}, {"NQ", "HMUZ"}, {"YM", "HMUZ"}, {"RTY", "HMUZ"},
		{"EMD", "HMUZ"}, {"NKD", "HMUZ"}, {"NIY", "HMUZ"},
		{"MES", "HMUZ"}, {"MNQ", "HMUZ"}, {"MYM", "HMUZ"}, {"M2K", "HMUZ"},
		{"XAF", "HMUZ"}, {"XAK", "HMUZ"}, {"XAE", "HMUZ"}, {"XAU", "HMUZ"},
		{"XAV", "HMUZ"}, {"XAB", "HMUZ"}, {"XAY", "HMUZ"}, {"XAI", "HMUZ"}, {"XAP", "HMUZ"},
		// FX - G10
		{"6E", "FGHJKMNQUVXZ"}, {"6J", "FGHJKMNQUVXZ"}, {"6B", "FGHJKMNQUVXZ"},
		{"6C", "FGHJKMNQUVXZ"}, {"6A", "FGHJKMNQUVXZ"}, {"6S", "FGHJKMNQUVXZ"},
		{"6N", "HMUZ"}, {"6M", "FGHJKMNQUVXZ"},
		{"E7", "HMUZ"}, {"J7", "HMUZ"},
		{"M6E", "HMUZ"}, {"M6A", "HMUZ"}, {"M6B", "HMUZ"}, {"MCD", "HMUZ"},
		{"MSF", "HMUZ"}, {"MJY", "HMUZ"},
		// FX - EM
		{"6L", "FGHJKMNQUVXZ"}, {"6R", "HMUZ"}, {"6Z", "HMUZ"},
		{"PLS", "HMUZ"}, {"CZK", "HMUZ"}, {"HUF", "HMUZ"},
		{"ILS", "HMUZ"}, {"KRW", "HMUZ"}, {"CNH", "FGHJKMNQUVXZ"},
		// Interest Rates
		{"ZT", "HMUZ"}, {"Z3N", "HMUZ"}, {"ZF", "HMUZ"}, {"ZN", "HMUZ"},
		{"TN", "HMUZ"}, {"ZB", "HMUZ"}, {"UB", "HMUZ"},
		{"SR3", "FGHJKMNQUVXZ"}, {"SR1", "FGHJKMNQUVXZ"}, {"FF", "FGHJKMNQUVXZ"}, {"GE", "HMUZ"},
		// Energy
		{"CL", "FGHJKMNQUVXZ"}, {"QM", "FGHJKMNQUVXZ"}, {"MCL", "FGHJKMNQUVXZ"},
		{"BZ", "FGHJKMNQUVXZ"}, {"NG", "FGHJKMNQUVXZ"}, {"QG", "FGHJKMNQUVXZ"},
		{"HO", "FGHJKMNQUVXZ"}, {"RB", "FGHJKMNQUVXZ"},
		// Metals
		{"GC", "FGHJKMNQUVXZ"}, {"SI", "FHKNUZ"}, {"PL", "FJNV"}, {"PA", "HMUZ"},
		{"MGC", "FGHJKMNQUVXZ"}, {"SIL", "FGHJKMNQUVXZ"},
		{"HG", "FHKNUZ"}, {"ALI", "FGHJKMNQUVXZ"}, {"QC", "FHKNUZ"},
		// Agriculture
		{"ZC", "FHKNUZ"}, {"ZS", "FHKNQUX"}, {"ZW", "FHKNUZ"}, {"KE", "FHKNUZ"}, {"ZO", "FHKNUZ"},
		{"ZM", "FHKNQUVZ"}, {"ZL", "FHKNQUVZ"},
		{"XC", "FHKNUZ"}, {"XW", "FHKNUZ"}, {"XK", "FHKNQUX"},
		// Livestock
		{"LE", "GJMQVZ"}, {"GF", "FHJKQUVX"}, {"HE", "GJKMNQVZ"},
		// Dairy
		{"DC", "FGHJKMNQUVXZ"}, {"DY", "FGHJKMNQUVXZ"}, {"CB", "FGHJKMNQUVXZ"}, {"CSC", "FGHJKMNQUVXZ"},
		// Other
		{"LBS", "FHKNUX"}, {"VX", "FGHJKMNQUVXZ"},
		// Crypto
		{"BTC", "FGHJKMNQUVXZ"}, {"MBT", "FGHJKMNQUVXZ"}, {"ETH", "FGHJKMNQUVXZ"}, {"MET", "FGHJKMNQUVXZ"},
	}

	currentYear := time.Now().Year()
	years := []int{currentYear % 10, (currentYear + 1) % 10, (currentYear + 2) % 10}

	var symbols []string
	for _, asset := range assets {
		for _, month := range asset.months {
			for _, year := range years {
				sym := fmt.Sprintf("%s%c%d", asset.name, month, year)
				symbols = append(symbols, sym)
			}
		}
	}

	return symbols
}

func fetchSymbolPrice(client *http.Client, apiKey, symbol, startDate, endDate string) (float64, uint64, error) {
	url := fmt.Sprintf(
		"https://hist.databento.com/v0/timeseries.get_range?dataset=GLBX.MDP3&symbols=%s&schema=ohlcv-1d&start=%sT00:00:00&end=%sT00:00:00&encoding=dbn",
		symbol, startDate, endDate,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, 0, err
	}
	req.SetBasicAuth(apiKey, "")

	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return 0, 0, nil
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, err
	}

	return parseOHLCVSingle(data)
}

func parseOHLCVSingle(data []byte) (float64, uint64, error) {
	if len(data) < 8 {
		return 0, 0, nil
	}
	if string(data[:3]) != "DBN" {
		return 0, 0, fmt.Errorf("invalid DBN magic")
	}

	metaLen := binary.LittleEndian.Uint32(data[4:8])
	headerSize := 8 + int(metaLen)

	if headerSize >= len(data) {
		return 0, 0, nil
	}

	records := data[headerSize:]
	ohlcvSize := int(unsafe.Sizeof(databento.OhlcvMsg{}))

	var lastClose int64
	var lastVolume uint64

	for offset := 0; offset+ohlcvSize <= len(records); offset += ohlcvSize {
		var ohlcv databento.OhlcvMsg
		reader := bytes.NewReader(records[offset : offset+ohlcvSize])
		if err := binary.Read(reader, binary.LittleEndian, &ohlcv); err != nil {
			continue
		}

		if ohlcv.Header.RType == databento.RTypeOhlcv1D {
			lastClose = ohlcv.Close
			lastVolume = ohlcv.Volume
		}
	}

	if lastClose == 0 {
		return 0, 0, nil
	}

	return float64(lastClose) / float64(databento.PriceScale), lastVolume, nil
}

var monthCodes = map[byte]int{
	'F': 1, 'G': 2, 'H': 3, 'J': 4, 'K': 5, 'M': 6,
	'N': 7, 'Q': 8, 'U': 9, 'V': 10, 'X': 11, 'Z': 12,
}

var symbolRegex = regexp.MustCompile(`^([A-Z0-9]+)([FGHJKMNQUVXZ])(\d)$`)

func parseSymbol(symbol string) (asset string, expiry time.Time) {
	symbol = strings.TrimSpace(symbol)

	matches := symbolRegex.FindStringSubmatch(symbol)
	if matches == nil {
		return "", time.Time{}
	}

	asset = matches[1]
	monthCode := matches[2][0]
	yearDigit := int(matches[3][0] - '0')

	month := monthCodes[monthCode]
	if month == 0 {
		return "", time.Time{}
	}

	currentYear := time.Now().Year()
	currentDecade := (currentYear / 10) * 10
	year := currentDecade + yearDigit

	if year < currentYear-5 {
		year += 10
	}

	expiry = time.Date(year, time.Month(month), 15, 0, 0, 0, 0, time.UTC)
	return asset, expiry
}

func loadContracts() ([]FuturesContract, error) {
	conn := db.Get()

	var count int
	err := conn.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='futures_contracts'").Scan(&count)
	if err != nil || count == 0 {
		return nil, nil
	}

	rows, err := conn.Query(`
		SELECT symbol, asset, expiration, settlement_price, open_interest, volume
		FROM futures_contracts
		WHERE expiration > ?
		ORDER BY asset, expiration
	`, time.Now().UnixNano())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contracts []FuturesContract
	for rows.Next() {
		var c FuturesContract
		var expNanos int64
		err := rows.Scan(&c.Symbol, &c.Asset, &expNanos, &c.SettlementPrice, &c.OpenInterest, &c.Volume)
		if err != nil {
			continue
		}
		c.Expiration = time.Unix(0, expNanos)
		contracts = append(contracts, c)
	}

	return contracts, nil
}
