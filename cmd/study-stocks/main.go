// study-stocks tells you what stocks are in a .sip file.
package main

import (
	"dropbear/broker/alpaca/sip"
	"dropbear/netty"
	"dropbear/symbol"
	"flag"
	"fmt"
	"log"
	"os"
)

func init() {
	netty.SetOffline()
}

var (
	flagDayOnly = flag.Bool("dayonly", false, "restrict to day session only")
)

var gSymbols = map[symbol.Symbol]struct{}{}

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "usage: go test -run Study -timeout 0 -- <sipfile>...\n")
		os.Exit(1)
	}
	for _, path := range flag.Args() {
		study(path)
	}
	for sym := range gSymbols {
		fmt.Println(sym)
	}
}

func study(path string) {
	f, err := sip.OpenFile(path)
	if err != nil {
		log.Fatalf("%s: %v", path, err)
	}
	defer f.Close()
	for {
		msg := f.Read()
		if msg == nil {
			break
		}
		switch msg.Type {
		case sip.MessageTypeQuote:
			quote := msg.Quote()
			gSymbols[quote.Symbol] = struct{}{}
		case sip.MessageTypeTrade:
			trade := msg.Trade()
			gSymbols[trade.Symbol] = struct{}{}
		}
	}
}
