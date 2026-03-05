package main

import (
	"flag"
	"log"

	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/symbol"
	"dropbear/loggy"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

var (
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
	cash            decimal.Decimal
	futuresByID     = make(map[uint32]*Future)
	es              *Future
	sr1             *Future
	optionsByID     = make(map[uint32]*Option)
	optionsByStrike = treeset.NewWith(compareOptionByStrike)
)

func main() {
	loggy.Init()
	flag.Parse()
	loggy.AlsoLogToFile()
	cash = *cashFlag

	key := databento.MustLoadDefaultKey()
	schwabClient := schwab.NewClient()

	futureDefs := make(chan *Future, 64)
	futureTicks := make(chan FutureTick, 64)
	optionDefs := make(chan *Option, 64)
	optionTicks := make(chan OptionTick, 64)
	orderUpdates := schwabClient.OrderUpdates()

	go streamFutures(key, futureDefs, futureTicks)
	go streamOptions(key, optionDefs, optionTicks)

	for {
		select {
		case f := <-futureDefs:
			futuresByID[f.ID] = f
			if f.Sym == symbol.Symbol('E'|'S'<<8) {
				es = f
			} else if f.Sym == symbol.Symbol('S'|'R'<<8|'1'<<16) {
				sr1 = f
				go fetchFuturePrice(key, sr1, futureTicks)
			} else {
				log.Fatalf("unknown future symbol: %s", f.Sym)
			}
		case t := <-futureTicks:
			future := futuresByID[t.ID]
			if future != nil {
				future.Bid = t.Bid
				future.Ask = t.Ask
				future.Price = t.Bid.Add(t.Ask).DivInt(2)
				log.Printf("tick %s mid %s bid %s ask %s", future.Sym, future.Price, t.Bid, t.Ask)
			}
		case o := <-optionDefs:
			optionsByID[o.ID] = o
			optionsByStrike.Add(o)
		case t := <-optionTicks:
			option := optionsByID[t.ID]
			if option != nil {
				option.Bid = t.Bid
				option.Ask = t.Ask
				log.Printf("tick %s bid %s ask %s", option, t.Bid, t.Ask)
			}
		case update := <-orderUpdates:
			log.Printf("order %s: %s", update.SchwabOrderID, update.Event)
		}
	}
}
