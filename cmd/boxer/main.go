package main

import (
	"encoding/json"
	"flag"
	"log"
	"strconv"

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
	minSpread     = decimal.Flag("minspread", "0.10", "min spread")
	maxEdgeFlag   = decimal.Flag("maxedge", "10.00", "max edge")
	maxSpreadFlag = decimal.Flag("maxspread", "1.00", "max spread")
	minBidFlag    = decimal.Flag("minbid", "0.05", "min bid")
	maxOpen       = flag.Int("maxopen", 4, "max open")
	latencyFlag   = clocky.DurationFlag("latency", "70ms", "latency")
	greedFlag     = decimal.Flag("greed", "0.00", "greed factor for spread placement")
	biasFlag      = decimal.Flag("bias", "0.00", "extra greed on bull (+) or bear (-) legs")
	patienceFlag  = clocky.DurationFlag("patience", "500ms", "patience")
	demandFlag    = decimal.Flag("demand", "35", "min profit to pounce")
	verbose       = flag.Bool("v", false, "verbose")
	dry           = flag.Bool("dry", false, "dry run (don't send orders)")
	timeTestFlag  = flag.Bool("timetest", false, "enable time test mode")
)

var (
	es              *Future
	sr1             *Future
	schwabClient    *schwab.Client
	futuresByID     = make(map[uint32]*Future)
	optionsByID     = make(map[uint32]*Option)
	optionsByStrike = treeset.NewWith(compareOptionByStrike)
)

const (
	esSymbol  = symbol.Symbol('E' | 'S'<<8)
	sr1Symbol = symbol.Symbol('S' | 'R'<<8 | '1'<<16)
)

func main() {
	loggy.Init()
	flag.Parse()
	loggy.AlsoLogToFile()

	key := databento.MustLoadDefaultKey()
	schwabClient = schwab.NewClient()
	InitHoldings()

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
		if !*timeTestFlag {
			boxer()
		}
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
	switch f.Symbol {
	case esSymbol:
		es = f
	case sr1Symbol:
		sr1 = f
		go fetchFuturePrice(key, sr1, ticks)
	default:
		log.Fatalf("unknown future symbol: %s", f.Symbol)
	}
}

func onFutureTick(t FutureTick) {
	future := futuresByID[t.ID]
	if future == nil {
		return
	}
	future.TS = t.TS
	future.Bid = t.Bid
	future.Ask = t.Ask
	future.Price = t.Bid.Add(t.Ask).DivInt(2)
	if *timeTestFlag {
		log.Printf("tick %s mid %s bid %s ask %s (%s stale)",
			future.Symbol, future.Price, t.Bid, t.Ask, clocky.Since(future.TS))
	}
}

func onOptionDef(o *Option) {
	optionsByID[o.ID] = o
	optionsByStrike.Add(o)
}

func onOptionTick(t OptionTick) {
	option := optionsByID[t.ID]
	if option == nil {
		return
	}
	if option.Bid == t.Bid && option.Ask == t.Ask {
		// no price change, ignore tick
		return
	}
	option.Bid = t.Bid
	option.Ask = t.Ask
	option.TS = t.TS
	if es != nil {
		option.ES = es.Price
	} else {
		option.ES = decimal.Zero
	}
	if *timeTestFlag {
		log.Printf("tick %s bid %s ask %s (%s stale)", option, t.Bid, t.Ask, clocky.Since(option.TS))
	}

	// log ticks for options associated with unfilled box legs
	for _, box := range boxes {
		for _, leg := range box.Legs {
			if leg.Option == option {
				leg.Lock.RLock()
				if !leg.Filled {
					log.Printf("tick %s %s %s %s bid=%s ask=%s spread=%s",
						leg.Name, leg.Instruction, option.Class, option.Strike,
						option.Bid, option.Ask, option.Ask.Sub(option.Bid))
				}
				leg.Lock.RUnlock()
			}
		}
	}
}

func onOrderUpdate(event *schwab.OrderEvent) {
	pretty, _ := json.MarshalIndent(json.RawMessage(event.RawData), "  ", "  ")
	log.Printf("order %s: %s\n  %s", event.SchwabOrderID, event.BaseEvent.EventType, pretty)

	// when an order is edited in thinkorswim, schwab cancels the old order
	// and creates a new one. update the leg's order ID so we can track the fill.
	if cancel := event.BaseEvent.CancelAcceptedEvent; cancel != nil {
		for _, info := range cancel.LegCancelRequestInfoList {
			if info.ChangedNewSchwabOrderId == "" {
				continue
			}
			oldID, err := strconv.ParseInt(info.LegID, 10, 64)
			if err != nil {
				continue
			}
			newID, err := strconv.ParseInt(info.ChangedNewSchwabOrderId, 10, 64)
			if err != nil {
				continue
			}
			for _, box := range boxes {
				for _, leg := range box.Legs {
					leg.Lock.Lock()
					if leg.OrderID == oldID {
						log.Printf("leg order ID updated %d -> %d", oldID, newID)
						leg.OrderID = newID
					}
					leg.Lock.Unlock()
				}
			}
		}
	}

	// log market maker routing
	if route := event.BaseEvent.ExecutionRequestedEventRoutedInfo; route != nil {
		if route.RouteInfo.RouteRequestedType == "New" {
			routeOrderID, err := strconv.ParseInt(event.SchwabOrderID, 10, 64)
			if err == nil {
				for _, box := range boxes {
					for _, leg := range box.Legs {
						leg.Lock.RLock()
						match := leg.OrderID == routeOrderID
						leg.Lock.RUnlock()
						if match {
							log.Printf("route %s %s %s %s -> %s @ %s",
								leg.Name, leg.Instruction, leg.Option.Class,
								leg.Option.Strike,
								route.RouteInfo.RouteName,
								route.RouteInfo.RoutedPrice)
						}
					}
				}
			}
		}
	}

	fill := event.BaseEvent.OrderFillCompletedEventOrderLegQuantityInfo
	if fill == nil {
		return
	}
	osi := fill.OrderInfoForTransactionPosting.Symbol
	qty := fill.ExecutionInfo.ExecutionQuantity
	if fill.OrderInfoForTransactionPosting.BuySellCode == "Sell" {
		qty = qty.Neg()
	}
	holdings[osi] = holdings[osi].Add(qty)

	// match fill to box leg and check for box completion
	orderID, err := strconv.ParseInt(event.SchwabOrderID, 10, 64)
	if err != nil {
		return
	}
	legName := ""
	remaining := boxes[:0]
	for _, box := range boxes {
		for _, leg := range box.Legs {
			leg.Lock.Lock()
			if leg.OrderID == orderID {
				leg.Filled = true
				legName = leg.Name
			}
			leg.Lock.Unlock()
		}
		if box.AllFilled() {
			side := "BUY"
			if !box.Buying {
				side = "SELL"
			}
			log.Printf("BOX FILLED: %s %s/%s profit=%s ($%s/box)",
				side, box.Low, box.High, box.Profit, box.Profit.MulInt(100))
		} else {
			remaining = append(remaining, box)
		}
	}
	boxes = remaining
	routeName := fill.ExecutionInfo.RouteName
	fillPrice := fill.ExecutionInfo.ExecutionPrice
	if legName != "" {
		log.Printf("fill %s %s via %s @ %s qty now %s",
			legName, osi, routeName, fillPrice, holdings[osi])
	} else {
		log.Printf("fill %s via %s @ %s qty now %s",
			osi, routeName, fillPrice, holdings[osi])
	}
}
