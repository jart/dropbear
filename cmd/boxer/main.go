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
	schwabClient    *schwab.Client
	holdings        = make(map[string]int)
)

func main() {
	loggy.Init()
	flag.Parse()
	loggy.AlsoLogToFile()
	cash = *cashFlag

	key := databento.MustLoadDefaultKey()
	schwabClient = schwab.NewClient()

	// fetch account positions to populate option holdings
	acct, err := schwabClient.GetAccount()
	if err != nil {
		log.Printf("warning: failed to fetch positions: %v", err)
	} else {
		for _, pos := range acct.SecuritiesAccount.Positions {
			qty := pos.LongQuantity.Sub(pos.ShortQuantity).Int()
			if qty != 0 {
				holdings[pos.Instrument.Symbol] = qty
				log.Printf("holding %s qty %d", pos.Instrument.Symbol, qty)
			}
		}
	}

	futureDefs := make(chan *Future, 64)
	futureTicks := make(chan FutureTick, 512)
	optionDefs := make(chan *Option, 64)
	optionTicks := make(chan OptionTick, 512)
	orderUpdates := schwabClient.OrderUpdates()

	go streamFutures(key, futureDefs, futureTicks)
	go streamOptions(key, optionDefs, optionTicks)

	for {
		// drain all pending events
		select {
		case f := <-futureDefs:
			onFutureDef(key, futureTicks, f)
			continue
		case t := <-futureTicks:
			onFutureTick(t)
			continue
		case o := <-optionDefs:
			onOptionDef(o)
			continue
		case t := <-optionTicks:
			onOptionTick(t)
			continue
		case update := <-orderUpdates:
			onOrderUpdate(update)
			continue
		default:
			// all channels empty
		}
		makeDecisions()
		// block until next event
		select {
		case f := <-futureDefs:
			onFutureDef(key, futureTicks, f)
		case t := <-futureTicks:
			onFutureTick(t)
		case o := <-optionDefs:
			onOptionDef(o)
		case t := <-optionTicks:
			onOptionTick(t)
		case update := <-orderUpdates:
			onOrderUpdate(update)
		}
	}
}

func onFutureDef(key databento.ApiKey, ticks chan<- FutureTick, f *Future) {
	futuresByID[f.ID] = f
	if f.Sym == symbol.Symbol('E'|'S'<<8) {
		es = f
	} else if f.Sym == symbol.Symbol('S'|'R'<<8|'1'<<16) {
		sr1 = f
		go fetchFuturePrice(key, sr1, ticks)
	} else {
		log.Fatalf("unknown future symbol: %s", f.Sym)
	}
}

func onFutureTick(t FutureTick) {
	future := futuresByID[t.ID]
	if future == nil {
		return
	}
	future.Bid = t.Bid
	future.Ask = t.Ask
	future.Price = t.Bid.Add(t.Ask).DivInt(2)
	// log.Printf("tick %s mid %s bid %s ask %s", future.Sym, future.Price, t.Bid, t.Ask)
}

func onOptionDef(o *Option) {
	optionsByID[o.ID] = o
	optionsByStrike.Add(o)
	if qty, ok := holdings[o.OSI()]; ok {
		log.Printf("position %s qty %d", o, qty)
		_ = qty
	}
}

func onOptionTick(t OptionTick) {
	option := optionsByID[t.ID]
	if option == nil {
		return
	}
	option.Bid = t.Bid
	option.Ask = t.Ask
	// log.Printf("tick %s bid %s ask %s", option, t.Bid, t.Ask)
}

func onOrderUpdate(event *schwab.OrderEvent) {
	log.Printf("order %s: %s", event.SchwabOrderID, event.BaseEvent.EventType)
	fill := event.BaseEvent.OrderFillCompletedEventOrderLegQuantityInfo
	if fill == nil {
		return
	}
	osi := fill.OrderInfoForTransactionPosting.Symbol
	qty := fill.ExecutionInfo.ExecutionQuantity.Int()
	if fill.OrderInfoForTransactionPosting.BuySellCode == "Sell" {
		qty = -qty
	}
	holdings[osi] += qty
	log.Printf("fill %s qty now %d", osi, holdings[osi])
}
