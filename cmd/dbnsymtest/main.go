// dbnsymtest checks which symbols have 0DTE options chains on recent trading days.
// Uses the databento historical API to fetch instrument definitions.
package main

import (
	"dropbear/broker/databento"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/osi"
	"fmt"
	"io"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: dbnsymtest SYMBOL [SYMBOL ...]\n")
		fmt.Fprintf(os.Stderr, "checks if symbols have 0DTE options on recent trading days\n")
		os.Exit(1)
	}
	symbols := os.Args[1:]
	client := databento.NewHistoricalClient(databento.MustLoadDefaultKey())

	// find some trading days
	var days []clocky.Time
	d := clocky.Now().Add(-clocky.Day)
	for len(days) < 5 {
		year, month, day := d.Date()
		if cboe.IsTradingDay(year, month, day) {
			days = append(days, cboe.GetOpenTime(year, month, day))
		}
		d = d.Add(-clocky.Day)
	}

	// print header
	fmt.Printf("%-8s", "SYMBOL")
	for _, d := range days {
		y, m, dy := d.Date()
		fmt.Printf("  %04d-%02d-%02d", y, m, dy)
	}
	fmt.Println()

	// check each symbol on each day
	for _, sym := range symbols {
		fmt.Printf("%-8s", sym)
		for _, d := range days {
			result := check0DTE(client, sym, d)
			fmt.Printf("  %10s", result)
		}
		fmt.Println()
	}
}

// check0DTE fetches instrument definitions for sym.OPT on the given day
// and returns "YES", "-", or "!" depending on whether 0DTE options exist.
func check0DTE(client *databento.HistoricalClient, sym string, date clocky.Time) string {
	year, month, day := date.Date()
	start := clocky.Date(year, month, day, 4, 0, 0, 0, clocky.NYC)
	end := clocky.Date(year, month, day, 9, 31, 0, 0, clocky.NYC)
	parentSym := sym + ".OPT"
	resp, err := client.GetRange(databento.GetRangeParams{
		Dataset: "OPRA.PILLAR",
		Schema:  databento.SchemaDefinition,
		STypeIn: databento.STypeParent,
		Symbols: []string{parentSym},
		Start:   start,
		End:     end,
	})
	if err != nil {
		log.Printf("%s %04d-%02d-%02d: %v", sym, year, month, day, err)
		return "!"
	}
	defer resp.Close()
	for {
		rec, err := resp.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			break
		}
		inst, ok := rec.(*databento.Instrument)
		if !ok {
			continue
		}
		_, _, _, expYear, expMonth, expDay, err := osi.Parse(inst.GetRawSymbol())
		if err != nil {
			panic("failed to parse symbol " + inst.GetRawSymbol() + ": " + err.Error())
		}
		if expYear == year && expMonth == month && expDay == day {
			return "YES"
		}
	}
	return "-"
}
