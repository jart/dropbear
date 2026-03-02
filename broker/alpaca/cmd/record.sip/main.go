// record.sip records real-time SIP market data from Alpaca
// Subscribes to trades, quotes, and bars for all symbols in ~/equitydata/minutes/
// Writes JSON lines to stdout (redirect to file)
package main

import (
	"dropbear/broker/alpaca"
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
	flagDir = flag.String("dir", os.ExpandEnv("$HOME/equitydata/minutes"), "directory containing symbol subdirectories")
)

func main() {
	flag.Parse()
	loggy.Init()

	// get symbols from directory listing
	symbols, err := getSymbols(*flagDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting symbols: %v", err)
		os.Exit(1)
	}

	// run recording daemon
	daemon := &recordDaemon{symbols: symbols}
	daemon.run()
}

func getSymbols(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var symbols []string
	for _, entry := range entries {
		if !entry.IsDir() {
			symbols = append(symbols, entry.Name())
		}
	}
	return symbols, nil
}

type recordDaemon struct {
	symbols []string
}

func (d *recordDaemon) run() {
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

func (d *recordDaemon) impl() error {
	// open websocket
	conn, _, err := netty.FastWSDial(sipWSURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// authenticate
	key := alpaca.GetKey()
	auth := map[string]any{
		"action": "auth",
		"key":    key.Key,
		"secret": key.Secret,
	}
	if err := conn.WriteJSON(auth); err != nil {
		return err
	}

	subscribe := map[string]any{
		"action":       "subscribe",
		"trades":       d.symbols,
		"quotes":       d.symbols,
		"bars":         d.symbols,
		"updatedBars":  d.symbols,
		"statuses":     d.symbols,
		"lulds":        d.symbols,
		"corrections":  d.symbols,
		"cancelErrors": d.symbols,
	}
	if err := conn.WriteJSON(subscribe); err != nil {
		return err
	}

	// relay messages to stdout
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", message)
		os.Stdout.Sync()
	}
}
