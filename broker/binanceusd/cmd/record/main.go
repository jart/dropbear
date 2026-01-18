package main

import (
	"dropbear/broker/binanceusd"
	"dropbear/ds"
	"dropbear/loggy"
	"encoding/binary"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

var (
	flagName = flag.String("name", "default", "name of dataset")
)

func main() {
	flag.Parse()
	loggy.Init()
	var recorders []*recorder
	client := binanceusd.NewClient()
	onSignal := make(chan os.Signal, 1)
	signal.Notify(onSignal, os.Interrupt, syscall.SIGTERM)
	for _, symbol := range flag.Args() {
		recorders = append(recorders, newRecorder(client, symbol))
	}
	<-onSignal
	log.Println("shutting down...")
	for _, r := range recorders {
		r.Close()
	}
}

type recorder struct {
	client *binanceusd.Client
	symbol string
	lock   sync.Mutex
	writer *ds.TickWriter
	buf    []byte
	done   bool
}

func newRecorder(client *binanceusd.Client, symbol string) *recorder {
	home := os.Getenv("HOME")
	outputDir := home + "/coindata/" + *flagName + "/binanceusd/"
	os.MkdirAll(outputDir, 0755)
	outputPath := outputDir + strings.ToUpper(symbol)
	log.Printf("recording binance market data for %s to %s", symbol, outputPath)
	writer, err := ds.NewTickWriter(outputPath)
	if err != nil {
		log.Fatalf("failed to create output file: %v", err)
	}
	recorder := &recorder{
		client: client,
		symbol: symbol,
		writer: writer,
	}
	go recorder.run()
	return recorder
}

func (r *recorder) Close() {
	r.lock.Lock()
	r.writer.Close()
	r.done = true
	r.lock.Unlock()
}

func (r *recorder) run() {
	for tick := range binanceusd.MarketData(r.symbol, r.client) {
		r.lock.Lock()
		if r.done {
			r.lock.Unlock()
			return
		}
		// Write length-prefixed tick data
		payload := tick.Bytes()
		r.buf = binary.LittleEndian.AppendUint32(r.buf[:0], uint32(len(payload)))
		r.buf = append(r.buf, payload...)
		err := r.writer.Write(r.buf)
		r.lock.Unlock()
		if err != nil {
			loggy.Fatalf("binance[%s]: market data write error: %v", r.symbol, err)
		}
	}
}
