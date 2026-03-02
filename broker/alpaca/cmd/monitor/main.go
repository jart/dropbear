// monitor monitors Alpaca SIP data streams.
// Usage: go run ./broker/alpaca/cmd/monitor -luld -status GOOG AAPL
package main

import (
	"bytes"
	"dropbear/broker/alpaca"
	"dropbear/broker/alpaca/sip"
	"dropbear/loggy"
	"dropbear/netty"
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

const sipWSURL = "wss://stream.data.alpaca.markets/v2/sip"

var (
	flagTrade  = flag.Bool("trade", false, "subscribe to trade executions")
	flagQuote  = flag.Bool("quote", false, "subscribe to quote updates")
	flagStatus = flag.Bool("status", false, "subscribe to trading status messages")
	flagLULD   = flag.Bool("luld", false, "subscribe to LULD (Limit Up-Limit Down) messages")
)

func main() {
	flag.Parse()
	loggy.Init()

	symbols := flag.Args()
	if len(symbols) == 0 {
		fmt.Fprintf(os.Stderr, "usage: monitor [-trade] [-quote] [-status] [-luld] SYMBOL...\n")
		os.Exit(1)
	}

	if !*flagTrade && !*flagQuote && !*flagStatus && !*flagLULD {
		fmt.Fprintf(os.Stderr, "error: must specify at least one of -trade, -quote, -status, or -luld\n")
		os.Exit(1)
	}

	daemon := &monitorDaemon{symbols: symbols}
	daemon.run()
}

type monitorDaemon struct {
	symbols []string
}

func (d *monitorDaemon) run() {
	try := 0
	for {
		ts1 := time.Now()
		err := d.impl()
		ts2 := time.Now()
		if err != nil {
			log.Printf("alpaca sip ws: %v, reconnecting...", err)
		}
		elapsed := ts2.Sub(ts1)
		if elapsed > 30*time.Second {
			try = 0 // connection was healthy so reset backoff
		}
		wait := time.Duration(15<<min(try, 11)) * time.Millisecond
		time.Sleep(wait)
		try++
	}
}

func (d *monitorDaemon) impl() error {
	// open websocket
	conn, _, err := netty.FastWSDial(sipWSURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// wait for connected message
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("reading connected: %w", err)
	}
	if !bytes.Contains(msg, []byte(`"connected"`)) {
		return fmt.Errorf("expected connected, got: %s", msg)
	}
	log.Printf("connected to alpaca sip")

	// authenticate
	key := alpaca.GetKey()
	auth := map[string]any{
		"action": "auth",
		"key":    key.Key,
		"secret": key.Secret,
	}
	if err := conn.WriteJSON(auth); err != nil {
		return fmt.Errorf("sending auth: %w", err)
	}

	// wait for authenticated message
	_, msg, err = conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("reading auth response: %w", err)
	}
	if bytes.Contains(msg, []byte(`"error"`)) {
		return fmt.Errorf("auth failed: %s", msg)
	}
	if !bytes.Contains(msg, []byte(`"authenticated"`)) {
		return fmt.Errorf("expected authenticated, got: %s", msg)
	}
	log.Printf("authenticated")

	// build subscription based on flags
	subscribe := map[string]any{
		"action": "subscribe",
	}
	if *flagTrade {
		subscribe["trades"] = d.symbols
	}
	if *flagQuote {
		subscribe["quotes"] = d.symbols
	}
	if *flagStatus {
		subscribe["statuses"] = d.symbols
	}
	if *flagLULD {
		subscribe["lulds"] = d.symbols
	}
	if err := conn.WriteJSON(subscribe); err != nil {
		return fmt.Errorf("sending subscribe: %w", err)
	}

	// wait for subscription confirmation
	_, msg, err = conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("reading subscribe response: %w", err)
	}
	if bytes.Contains(msg, []byte(`"error"`)) {
		return fmt.Errorf("subscribe failed: %s", msg)
	}
	if !bytes.Contains(msg, []byte(`"subscription"`)) {
		return fmt.Errorf("expected subscription, got: %s", msg)
	}
	log.Printf("subscribed to %v", d.symbols)

	// process messages
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		d.processMessages(message)
	}
}

func (d *monitorDaemon) processMessages(data []byte) {
	// messages come as JSON arrays like [{"T":"s",...},{"T":"l",...}]
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
						d.processMessage(data[start:i])
						goto nextObject
					}
				}
			}
			i++
		}
	nextObject:
	}
}

func (d *monitorDaemon) processMessage(data []byte) {
	msgType := sip.GetMessageType(data)
	switch msgType {
	case 't': // trade
		if *flagTrade {
			var trade sip.Trade
			_, err := trade.Parse(data)
			if err != nil {
				log.Printf("parse trade error: %v", err)
				return
			}
			fmt.Printf("%#v\n", trade)
		}
	case 'q': // quote
		if *flagQuote {
			var quote sip.Quote
			_, err := quote.Parse(data)
			if err != nil {
				log.Printf("parse quote error: %v", err)
				return
			}
			fmt.Printf("%#v\n", quote)
		}
	case 's': // status
		if *flagStatus {
			var status sip.Status
			_, err := status.Parse(data)
			if err != nil {
				log.Printf("parse status error: %v", err)
				return
			}
			fmt.Printf("%#v\n", status)
		}
	case 'l': // LULD
		if *flagLULD {
			var luld sip.LULD
			_, err := luld.Parse(data)
			if err != nil {
				log.Printf("parse LULD error: %v", err)
				return
			}
			fmt.Printf("%#v\n", luld)
		}
	}
}
