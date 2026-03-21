package main

import (
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/osi"
	"dropbear/ds/symbol"
	"dropbear/loggy"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
)

var (
	closeFlag = decimal.Flag("close", "6000", "price of spx at close")
	rangeFlag = decimal.Flag("range", "100", "possible movement of spx for risk calculation")
)

const (
	kSPXW = symbol.Symbol('S' | 'P'<<8 | 'X'<<16 | 'W'<<24)
)

var (
	gTrades     []Trade
	gCash       decimal.Decimal
	gStrikeMin  decimal.Decimal
	gStrikeMax  decimal.Decimal
	gPositions  = map[Contract]int{}
	kMultiplier = decimal.FromInt(100)
	kTick       = decimal.FromInt(5)
)

type Trade struct {
	Contract Contract
	Cost     decimal.Decimal
	Time     clocky.Time
	Tag      string
}

type Contract struct {
	Strike decimal.Decimal
	Class  byte
}

func (c Contract) String() string {
	return fmt.Sprintf("%s %c", c.Strike, c.Class)
}

// Intrinsic returns the intrinsic value of the option contract at the given underlying price.
// Returns negative number when short contract is in the money.
// Returns positive number when long contract is in the money.
// Returns zero when contract isn't in the money.
func (c *Contract) Intrinsic(underlyingPrice decimal.Decimal) decimal.Decimal {
	if c.Class == 'C' {
		return underlyingPrice.Sub(c.Strike).Max(decimal.Zero).Mul(kMultiplier)
	}
	return c.Strike.Sub(underlyingPrice).Max(decimal.Zero).Mul(kMultiplier)
}

func main() {
	loggy.Init()
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "usage: boxreport -close 6000 <schwab-orders.json> ...\n")
		os.Exit(1)
	}
	for _, path := range flag.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		var batch []schwab.Order
		if err := json.Unmarshal(data, &batch); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		for _, order := range batch {
			getTrades(path, &order)
		}
	}
	if len(gTrades) == 0 {
		fmt.Fprintf(os.Stderr, "no filled 0dte spxw option trades found\n")
		os.Exit(1)
	}
	sort.Slice(gTrades, func(i, j int) bool {
		return gTrades[i].Time.Before(gTrades[j].Time)
	})

	for _, trade := range gTrades {
		cash := trade.Cost
		gCash = gCash.Add(cash)
		if trade.Cost.IsNegative() {
			gPositions[trade.Contract]++ // bought
		} else {
			gPositions[trade.Contract]-- // sold
		}
		destiny := computeSettlementAt(*closeFlag)
		best, worst := computeRisk()
		fmt.Printf("%s have %+3d of %s costing: %6s balance: %8s destiny: %8s best: %8s worst: %8s using: %s\n",
			trade.Time, gPositions[trade.Contract], trade.Contract,
			trade.Cost, gCash, destiny, best, worst, trade.Tag)
	}

	// settle remaining positions at close
	fmt.Printf("\nend of day settlement:\n")
	for contract, pos := range gPositions {
		if pos == 0 {
			continue
		}
		worth := contract.Intrinsic(*closeFlag).MulInt(pos)
		gCash = gCash.Add(worth)
		fmt.Printf("have %3d of %s worth %8s\n", pos, contract, worth)
	}
	fmt.Printf("\nending balance %s at spx price %s\n", gCash, *closeFlag)
}

func computeRisk() (best, worst decimal.Decimal) {
	lo := gStrikeMin.Sub(*rangeFlag)
	hi := gStrikeMax.Add(*rangeFlag)
	for price := lo; price.Cmp(hi) <= 0; price = price.Add(kTick) {
		settlement := computeSettlementAt(price)
		worst = worst.Min(settlement)
		best = best.Max(settlement)
	}
	return best, worst
}

func computeSettlementAt(underlyingPrice decimal.Decimal) decimal.Decimal {
	cash := gCash
	for contract, pos := range gPositions {
		intrinsic := contract.Intrinsic(underlyingPrice)
		settlement := intrinsic.MulInt(pos)
		cash = cash.Add(settlement)
	}
	return cash
}

func getTrades(path string, order *schwab.Order) {
	if order.Status != schwab.OrderStatusFilled {
		log.Printf("%s: warning: skipping order %d with status %s\n", path, order.OrderID, order.Status)
		return
	}
	for _, leg := range order.OrderLegCollection {
		if leg.Instrument.AssetType != schwab.AssetTypeOption {
			log.Printf("%s: warning: skipping leg with non-option asset type %s\n", path, leg.Instrument.AssetType)
			continue
		}
		sym, strike, class, year, month, day, err := osi.Parse(leg.Instrument.Symbol)
		if err != nil {
			log.Printf("%s: warning: skipping leg with unparseable symbol %s: %v\n", path, leg.Instrument.Symbol, err)
			continue
		}
		if sym != kSPXW {
			log.Printf("%s: warning: skipping leg with non-SPXW symbol %s\n", path, leg.Instrument.Symbol)
			continue
		}
		if order.CloseTime.Year() != year || order.CloseTime.Month() != clocky.Month(month) || order.CloseTime.Day() != day {
			log.Printf("%s: warning: skipping leg with non-0dte expiration %s\n", path, leg.Instrument.Symbol)
			continue
		}
		for _, activity := range order.OrderActivityCollection {
			if activity.ExecutionType != schwab.ExecutionTypeFill {
				log.Printf("%s: warning: skipping activity with non-fill execution type %s\n", path, activity.ExecutionType)
				continue
			}
			for _, execLeg := range activity.ExecutionLegs {
				if execLeg.InstrumentID != leg.Instrument.InstrumentID {
					continue
				}
				if execLeg.LegID != leg.LegID {
					continue
				}
				price := execLeg.Price
				switch leg.Instruction {
				case schwab.InstructionBuyToOpen, schwab.InstructionBuyToClose:
					price = price.Neg()
				}
				for range execLeg.Quantity.Int() {
					gStrikeMin = gStrikeMin.Min(strike)
					gStrikeMax = gStrikeMax.Max(strike)
					gTrades = append(gTrades, Trade{
						Contract: Contract{
							Strike: strike,
							Class:  class,
						},
						Cost: price.Mul(kMultiplier),
						Time: execLeg.Time,
						Tag:  order.Tag,
					})
				}
			}
		}
	}
}
