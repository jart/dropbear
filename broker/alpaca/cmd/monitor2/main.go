// monitor2 monitors Alpaca SIP data from a JSONL file (written by record.sip).
// Usage: go run ./broker/alpaca/cmd/monitor2 -i ~/sipdata.jsonl -trade -status GOOG
package main

import (
	"bufio"
	"dropbear/broker/alpaca/sip"
	"dropbear/ds/symbol"
	"dropbear/loggy"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

var (
	flagInput  = flag.String("i", "", "input JSONL file (required)")
	flagTrade  = flag.Bool("trade", false, "show trade executions")
	flagQuote  = flag.Bool("quote", false, "show quote updates")
	flagStatus = flag.Bool("status", false, "show trading status messages")
	flagLULD   = flag.Bool("luld", false, "show LULD (Limit Up-Limit Down) messages")
)

func main() {
	flag.Parse()
	loggy.Init()

	if *flagInput == "" {
		fmt.Fprintf(os.Stderr, "usage: monitor2 -i FILE [-trade] [-quote] [-status] [-luld] [SYMBOL...]\n")
		os.Exit(1)
	}

	if !*flagTrade && !*flagQuote && !*flagStatus && !*flagLULD {
		fmt.Fprintf(os.Stderr, "error: must specify at least one of -trade, -quote, -status, or -luld\n")
		os.Exit(1)
	}

	// build symbol filter (empty means all symbols)
	symbols := make(map[symbol.Symbol]bool)
	for _, s := range flag.Args() {
		sym, err := symbol.Parse(s)
		if err != nil {
			log.Fatalf("invalid symbol %q: %v", s, err)
		}
		symbols[sym] = true
	}

	f, err := os.Open(*flagInput)
	if err != nil {
		log.Fatalf("open %s: %v", *flagInput, err)
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			// wait for more data
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if err != nil {
			log.Fatalf("read: %v", err)
		}
		processLine(line, symbols)
	}
}

func processLine(data []byte, symbols map[symbol.Symbol]bool) {
	// each line is a JSON array like [{"T":"t",...},{"T":"q",...}]
	// find each object and process it
	i := 0
	for i < len(data) {
		// skip to next object start
		for i < len(data) && data[i] != '{' {
			i++
		}
		if i >= len(data) {
			break
		}

		// find object end (handle nested braces)
		start := i
		depth := 0
		inString := false
		for i < len(data) {
			c := data[i]
			if inString {
				switch c {
				case '\\':
					i++
				case '"':
					inString = false
				}
			} else {
				switch c {
				case '"':
					inString = true
				case '{':
					depth++
				case '}':
					depth--
					if depth == 0 {
						i++
						processMessage(data[start:i], symbols)
						goto nextObject
					}
				}
			}
			i++
		}
	nextObject:
	}
}

func processMessage(data []byte, symbols map[symbol.Symbol]bool) {
	msgType := sip.GetMessageType(data)
	switch msgType {
	case 't': // trade
		if *flagTrade {
			trade, err := sip.ParseTrade(data)
			if err != nil {
				return
			}
			if len(symbols) > 0 && !symbols[trade.Symbol] {
				return
			}
			fmt.Printf("%#v\n", trade)
		}
	case 'q': // quote
		if *flagQuote {
			quote, err := sip.ParseQuote(data)
			if err != nil {
				return
			}
			if len(symbols) > 0 && !symbols[quote.Symbol] {
				return
			}
			fmt.Printf("%#v\n", quote)
		}
	case 's': // status
		if *flagStatus {
			status, err := sip.ParseStatus(data)
			if err != nil {
				return
			}
			if len(symbols) > 0 && !symbols[status.Symbol] {
				return
			}
			fmt.Printf("%#v\n", status)
		}
	case 'l': // luld
		if *flagLULD {
			luld, err := sip.ParseLULD(data)
			if err != nil {
				return
			}
			if len(symbols) > 0 && !symbols[luld.Symbol] {
				return
			}
			fmt.Printf("%#v\n", luld)
		}
	}
}
