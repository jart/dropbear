// record.sip records real-time SIP market data from Alpaca
// Subscribes to trades, quotes, and bars for all symbols in ~/equitydata/minutes/
// Writes JSON lines to stdout (redirect to file)
package main

import (
	"dropbear/broker/alpaca"
	"dropbear/loggy"
	"dropbear/netty"
	"encoding/json"
	"flag"
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
		log.Fatalf("error getting symbols: %v", err)
	}
	log.Printf("found %d symbols", len(symbols))

	// run the recording daemon
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
	key, secret := alpaca.GetKey()
	auth := map[string]any{
		"action": "auth",
		"key":    key,
		"secret": secret,
	}
	if err := conn.WriteJSON(auth); err != nil {
		return err
	}

	// read auth response
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	log.Printf("alpaca sip ws: auth response: %s", string(msg))

	// subscribe to all symbols in batches (websocket might have limits)
	batchSize := 500
	for i := 0; i < len(d.symbols); i += batchSize {
		end := min(i+batchSize, len(d.symbols))
		batch := d.symbols[i:end]

		subscribe := map[string]any{
			"action":   "subscribe",
			"trades":   batch,
			"quotes":   batch,
			"bars":     batch,
			"statuses": batch,
		}
		if err := conn.WriteJSON(subscribe); err != nil {
			return err
		}

		// read subscription confirmation
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		log.Printf("alpaca sip ws: subscribed batch %d-%d: %s", i, end, string(msg))
	}

	log.Printf("alpaca sip ws: subscribed to %d symbols for trades, quotes, bars", len(d.symbols))

	// read messages forever and write to stdout
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		log.Printf("got message type %d: %s", messageType, string(message))

		// parse the message array
		var msgs []json.RawMessage
		if err := json.Unmarshal(message, &msgs); err != nil {
			// single message, not array
			var single any
			json.Unmarshal(message, &single)
			enc.Encode(single)
			os.Stdout.Sync()
			continue
		}

		// write each message pretty-printed
		for _, m := range msgs {
			var parsed any
			json.Unmarshal(m, &parsed)
			enc.Encode(parsed)
		}
		os.Stdout.Sync()
	}
}
