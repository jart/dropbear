// Takes a sip feed archive and downloads all corresponding symbols'
// front-expiry option chains from databento in OPRA format
package main

import (
	"dropbear/broker/alpaca/sip"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/loggy"
	"dropbear/symbol"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
)

var (
	flagData = flag.String("data", "", "path of sip data file for retrieval")
)

func main() {
	loggy.Init()
	flag.Parse()

	if *flagData == "" {
		fmt.Fprintf(os.Stderr, "-data must be specified for backtest mode\n")
		os.Exit(1)
	}

	err := downloadChains(flagData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "downloadChain: %v", err)
		os.Exit(1)
	}
}

func downloadChains(sipFile *string) error {
	s, err := sip.OpenFile(*sipFile)
	if err != nil {
		return err
	}
	defer s.Close()

	var (
		startMsg = s.Get(0)
		finalMsg = s.Get(s.Count() - 1)
	)

	if startMsg == nil || finalMsg == nil {
		return errors.New("SIP feed start/end message is nil")
	}

	var (
		start = startMsg.Timestamp
		end   = finalMsg.Timestamp
	)

	log.Printf("SIP file session range: %s - %s", start.String(), end.String())
	symbolChains := make(map[symbol.Symbol]clocky.Time, 0)

	var (
		nowYear  = start.Year()
		nowMonth = start.Month()
		nowDay   = start.Day()
	)

	// Discover all symbols in the SIP feed and get the front
	// expiry option chain for all of the symbols in our feed
	for msg := s.Read(); msg != nil; msg = s.Read() {
		sym := msg.Symbol

		if _, ok := symbolChains[sym]; !ok {
			log.Printf("load front option chain: %s", sym.String())
			y, m, d := cboe.GetNextOptionChain(sym, nowYear, nowMonth, nowDay)
			symbolChains[sym] = clocky.Date(y, m, d, 9, 30, 0, 0, clocky.NYC)
		}
	}

	for sym, nextChain := range symbolChains {
		log.Printf("s=%s, exp=%d", sym.String(), nextChain.DateInt())
	}

	return nil
}
