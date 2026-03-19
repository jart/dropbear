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

	"github.com/emirpasic/gods/v2/sets/hashset"
	"github.com/emirpasic/gods/v2/sets/treeset"
)

var (
	demandFlag       = decimal.Flag("demand", "30", "min profit to pounce")
	widthFlag        = decimal.Flag("width", "50", "box width")
	moneynessFlag    = decimal.Flag("moneyness", "200", "maximum distance from the money")
	safetyFlag       = decimal.Flag("safety", "10", "spx safety points")
	freshFlag        = clocky.DurationFlag("fresh", "200ms", "freshness threshold")
	cooldownFlag     = clocky.DurationFlag("cooldown", "8s", "cooldown between boxes")
	greedFlag        = decimal.Flag("greed", "0.00", "greed factor for spread placement")
	maxImbalanceFlag = flag.Int("maximbalance", 3, "maximum absolute difference between unfilled bulls and bears")
	verbose          = flag.Bool("v", false, "verbose")
	dryFlag          = flag.Bool("dry", false, "dry run (don't send orders)")
	timeTestFlag     = flag.Bool("timetest", false, "enable time test mode")
)

var (
	es                  *Future
	sr1                 *Future
	schwabClient        *schwab.Client
	futuresByID         = make(map[uint32]*Future)
	optionsByID         = make(map[uint32]*Option)
	legsByOrderID       = make(map[int64]*Leg)
	optionsByStrike     = treeset.NewWith(compareOptionByStrike)
	restrictedToBuying  = hashset.New[uint32]()
	restrictedToSelling = hashset.New[uint32]()
	legUpdates          = make(chan LegUpdate, 20)
	unfilledBulls       = hashset.New[*Leg]()
	unfilledBears       = hashset.New[*Leg]()
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
		case legUpdate := <-legUpdates:
			onLegUpdate(legUpdate)
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
		case legUpdate := <-legUpdates:
			onLegUpdate(legUpdate)
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
	switch t.Kind {
	case OptionTickKindBid:
		option.Bid = t.Price
	case OptionTickKindAsk:
		option.Ask = t.Price
	case OptionTickKindTrade:
		return // haven't found use for this yet
	}
	option.TS = t.TS
	if es != nil {
		option.ES = es.Price
	} else {
		option.ES = decimal.Zero
	}
}

func onLegUpdate(update LegUpdate) {
	update.Leg.OrderID = update.OrderID
	legsByOrderID[update.OrderID] = update.Leg
}

func onOrderUpdate(event *schwab.OrderEvent) {
	pretty, _ := json.MarshalIndent(json.RawMessage(event.RawData), "  ", "  ")
	log.Printf("order %s: %s\n  %s", event.SchwabOrderID, event.BaseEvent.EventType, pretty)
	if event.BaseEvent.CancelAcceptedEvent != nil {
		onCancelEvent(event.BaseEvent.CancelAcceptedEvent)
		return
	}
	if event.BaseEvent.ExecutionRequestedEventRoutedInfo != nil {
		onRouteEvent(event, event.BaseEvent.ExecutionRequestedEventRoutedInfo)
		return
	}
	if event.BaseEvent.OrderFillCompletedEventOrderLegQuantityInfo != nil {
		onFillEvent(event, event.BaseEvent.OrderFillCompletedEventOrderLegQuantityInfo)
		return
	}
}

func onCancelEvent(cancel *schwab.CancelEvent) {
	for _, info := range cancel.LegCancelRequestInfoList {
		if info.ChangedNewSchwabOrderId == "" {
			continue
		}
		oldID, err := strconv.ParseInt(info.LegID, 10, 64)
		if err != nil {
			continue
		}
		if leg := legsByOrderID[oldID]; leg != nil {
			if info.ChangedNewSchwabOrderId != "" {
				// order was updated in thinkorswim
				newID, err := strconv.ParseInt(info.ChangedNewSchwabOrderId, 10, 64)
				if err != nil {
					continue
				}
				leg.UpdateOrderID(newID)
			} else {
				// order was canceled in thinkorswim
				boxes.Remove(leg.Box)
				unfilledBulls.Remove(leg)
				unfilledBears.Remove(leg)
				log.Printf("leg %s order canceled in thinkorswim (id=%d)", leg.Name, oldID)
			}
		} else {
			log.Printf("order id %d not found for cancel event", oldID)
		}
	}
}

func onRouteEvent(event *schwab.OrderEvent, route *schwab.RouteEvent) {
	if route.RouteInfo.RouteRequestedType != "New" {
		return
	}
	routeOrderID, err := strconv.ParseInt(event.SchwabOrderID, 10, 64)
	if err != nil {
		log.Printf("invalid order id %s for route event", event.SchwabOrderID)
		return
	}
	leg := legsByOrderID[routeOrderID]
	if leg == nil {
		log.Printf("order id %d not found for route event: %s @ %s",
			routeOrderID, route.RouteInfo.RouteName, route.RouteInfo.RoutedPrice)
		return
	}
	log.Printf("route %s %s %s %s -> %s @ %s",
		leg.Name, leg.Instruction(), leg.Option.Class,
		leg.Option.Strike,
		route.RouteInfo.RouteName,
		route.RouteInfo.RoutedPrice)
}

func onFillEvent(event *schwab.OrderEvent, fill *schwab.FillEvent) {
	osi := fill.OrderInfoForTransactionPosting.Symbol
	qty := fill.ExecutionInfo.ExecutionQuantity
	routeName := fill.ExecutionInfo.RouteName
	fillPrice := fill.ExecutionInfo.ExecutionPrice
	priceImprovement := fill.PriceImprovement
	if fill.OrderInfoForTransactionPosting.BuySellCode == "Sell" {
		qty = qty.Neg()
	}
	holdings[osi] = holdings[osi].Add(qty)
	orderID, err := strconv.ParseInt(event.SchwabOrderID, 10, 64)
	if err != nil {
		log.Printf("invalid order id %s for fill event", event.SchwabOrderID)
		return
	}
	leg := legsByOrderID[orderID]
	if leg == nil {
		log.Printf("order id %d not found for fill event", orderID)
		return
	}
	leg.FillPrice = fillPrice
	unfilledBulls.Remove(leg)
	unfilledBears.Remove(leg)
	log.Printf("leg fill %s for %d at %s with %s price improvement from %s route (limit=%s market=%s es=%s)",
		leg.Name, orderID, fillPrice, priceImprovement, routeName, leg.LimitPrice, leg.MarketPrice(), es.Price)
	if leg.Box.Filled() {
		log.Printf("box filled with profit %s (originally %s) %s (limit=%s market=%s es=%s)",
			leg.Box.FillProfit(), leg.Box.LimitProfit(), leg.Box, leg.Box.LimitPrice(), leg.Box.MarketPrice(), es.Price)
		boxes.Remove(leg.Box)
	}
}
