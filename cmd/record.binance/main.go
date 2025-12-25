package main

import (
	"dropbear/exchange/binance"
	"dropbear/loggy"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/klauspost/compress/zstd"
)

var (
	flagName = flag.String("name", "default", "name of dataset")
)

func main() {
	flag.Parse()
	loggy.Init()
	var recorders []*recorder
	client := binance.NewClient()
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
	client *binance.Client
	symbol string
	lock   sync.Mutex
	writer *zstd.Encoder
	done   bool
}

func newRecorder(client *binance.Client, symbol string) *recorder {
	home := os.Getenv("HOME")
	outputDir := home + "/marketdata/" + *flagName + "/binance/"
	os.MkdirAll(outputDir, 0755)
	outputPath := outputDir + strings.ToUpper(symbol)
	log.Printf("recording binance market data for %s to %s", symbol, outputPath)
	outputFile, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("failed to create output file: %v", err)
	}
	writer, err := zstd.NewWriter(outputFile, zstd.WithWindowSize(128*1024))
	if err != nil {
		log.Fatalf("failed to create zstd writer: %v", err)
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
	for tick := range binance.MarketData(r.symbol, r.client) {
		r.lock.Lock()
		if r.done {
			r.lock.Unlock()
			return
		}
		err := tick.Serialize(r.writer)
		r.lock.Unlock()
		if err != nil {
			loggy.Fatalf("binance[%s]: market data write error: %v", r.symbol, err)
		}
	}
}
