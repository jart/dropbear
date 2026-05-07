// Takes a sip feed archive and downloads all corresponding symbols'
// front-expiry option chains from databento in OPRA format
package main

import (
	"dropbear/broker/alpaca/sip"
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/loggy"
	"dropbear/symbol"
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

	err := downloadChains(*flagData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v", *flagData, err)
		os.Exit(1)
	}
}

func downloadChains(sipFile string) error {
	s, err := sip.OpenFile(sipFile)
	if err != nil {
		return err
	}
	defer s.Close()

	startMsg, _ := s.Get(0)
	finalMsg, _ := s.Get(s.Count() - 1)

	start := startMsg.Timestamp
	end := finalMsg.Timestamp

	log.Printf("SIP file session range: %s - %s", start.String(), end.String())
	symbolChains := make(map[symbol.Symbol]clocky.Time, 0)

	nowYear, nowMonth, nowDay := start.Date()

	// Discover all symbols in the SIP feed and get the front
	// expiry option chain for all of the symbols in our feed
	for msg, _ := s.Read(); msg != nil; msg, _ = s.Read() {
		sym := msg.Symbol

		if _, ok := symbolChains[sym]; !ok {
			log.Printf("load front option chain: %s", sym.String())
			y, m, d := cboe.GetNextOptionChain(sym, nowYear, nowMonth, nowDay)
			symbolChains[sym] = cboe.GetOpenTime(y, m, d)
		}
	}

	for sym, nextChain := range symbolChains {
		log.Printf("s=%s, exp=%d", sym.String(), nextChain.DateInt())
	}

	return nil
}
