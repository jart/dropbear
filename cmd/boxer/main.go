package main

import (
	"flag"

	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/loggy"
)

var (
	dataset       = flag.String("dataset", "OPRA.PILLAR", "dataset")
	dataPath      = flag.String("data", "", "path to data")
	defsPath      = flag.String("defs", "", "path to defs")
	widthFlag     = decimal.Flag("width", "50", "box width")
	multFlag      = flag.Int("mult", 100, "multiplier")
	edgeFlag      = decimal.Flag("edge", "0.10", "min edge")
	maxEdgeFlag   = decimal.Flag("maxedge", "10.00", "max edge")
	maxSpreadFlag = decimal.Flag("maxspread", "1.00", "max spread")
	minBidFlag    = decimal.Flag("minbid", "0.05", "min bid")
	maxOpen       = flag.Int("maxopen", 4, "max open")
	latencyFlag   = clocky.DurationFlag("latency", "70ms", "latency")
	greedFlag     = decimal.Flag("greed", "0.00", "greed factor for spread placement")
	patienceFlag  = clocky.DurationFlag("patience", "500ms", "patience")
	cashFlag      = decimal.Flag("cash", "100000", "starting cash")
	verbose       = flag.Bool("v", false, "verbose")
)

var (
	cash decimal.Decimal
)

func main() {
	loggy.Init()
	flag.Parse()
	loggy.AlsoLogToFile()
	cash = *cashFlag
	key := databento.MustLoadDefaultKey()
	go monitorFuturesLive(key)
	tradeOptionsLive(key)
}
