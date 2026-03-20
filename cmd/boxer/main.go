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

	"github.com/emirpasic/gods/v2/maps/treemap"
	"github.com/emirpasic/gods/v2/sets/hashset"
	"github.com/emirpasic/gods/v2/sets/linkedhashset"
	"github.com/emirpasic/gods/v2/sets/treeset"
)

var (
	demandFlag       = decimal.Flag("demand", "40", "min profit to pounce")
	widthFlag        = decimal.Flag("width", "25", "maximum box width")
	moneynessFlag    = decimal.Flag("moneyness", "25", "maximum distance of any leg from the money")
	safetyFlag       = decimal.Flag("safety", "10", "spx safety points")
	freshFlag        = clocky.DurationFlag("fresh", "200ms", "freshness threshold")
	cooldownFlag     = clocky.DurationFlag("cooldown", "8s", "cooldown between boxes")
	heartbeatFlag    = clocky.DurationFlag("heartbeat", "3s", "reporting interval for unfilled legs")
	patienceFlag     = clocky.DurationFlag("patience", "10s", "how long to wait before closing partially filled box")
	maxImbalanceFlag = flag.Int("max-imbalance", 2, "maximum absolute difference between unfilled bulls and bears")
	maxUnfilledFlag  = flag.Int("max-unfilled", 4, "maximum number of unfilled legs (bulls + bears) before pausing")
	dryFlag          = flag.Bool("dry", false, "dry run (don't send orders)")
)

var (
	gES                  *Future
	gSR1                 *Future
	gSchwabClient        *schwab.Client
	gSPXPrice            decimal.Decimal
	gSPXPriceTime        clocky.Time
	gTotalFees           decimal.Decimal
	gFuturesByID         = make(map[uint32]*Future)
	gOptionsByID         = make(map[uint32]*Option)
	gOptionsByOSI        = make(map[string]*Option)
	gLegsByOrderID       = make(map[schwab.OrderID]*Leg)
	gOptionsByStrike     = treeset.NewWith(compareOptionByStrike)
	gRestrictedToBuying  = hashset.New[uint32]()
	gRestrictedToSelling = hashset.New[uint32]()
	gHoldings            = treemap.New[string, decimal.Decimal]()
	gStrikes             = treemap.New[decimal.Decimal, *Strike]()
	gPendingStrikes      = treemap.New[decimal.Decimal, *Strike]()
	gLegUpdates          = make(chan LegUpdate, 20)
	gUnfilledBulls       = linkedhashset.New[*Leg]()
	gUnfilledBears       = linkedhashset.New[*Leg]()
	gPendingBoxes        = linkedhashset.New[*Box]()
	gPendingLegs         = linkedhashset.New[*Leg]()
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
	gSchwabClient = schwab.NewClient()
	InitHoldings()

	futureDefs := make(chan *Future, 64)
	optionDefs := make(chan *Option, 64)
	futureTicks := make(chan *databento.MBP1, 256)
	optionTicks := make(chan *databento.CMBP1, 256)
	orderUpdates := gSchwabClient.OrderUpdates()
	heartbeat := clocky.NewTicker(*heartbeatFlag)

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
		case legUpdate := <-gLegUpdates:
			onLegOrderID(legUpdate.Leg, legUpdate.OrderID)
			continue
		case <-heartbeat.C:
			onHeartbeat()
			continue
		default:
			// all channels empty
		}
		// let's go
		boxer()
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
		case legUpdate := <-gLegUpdates:
			onLegOrderID(legUpdate.Leg, legUpdate.OrderID)
		case <-heartbeat.C:
			onHeartbeat()
		}
	}
}

func onFutureDef(key databento.ApiKey, ticks chan<- *databento.MBP1, f *Future) {
	gFuturesByID[f.ID] = f
	switch f.Symbol {
	case esSymbol:
		gES = f
	case sr1Symbol:
		gSR1 = f
		go fetchFuturePrice(key, gSR1, ticks)
	default:
		log.Fatalf("unknown future symbol: %s", f.Symbol)
	}
}

func onFutureTick(t *databento.MBP1) {
	f := gFuturesByID[t.Header.InstrumentID]
	if f == nil {
		return
	}
	if t.Header.TSEvent > f.TS {
		f.TS = t.Header.TSEvent
		f.Bid = dbnPrice(t.Levels[0].BidPx)
		f.Ask = dbnPrice(t.Levels[0].AskPx)
		f.Price = f.Bid.Add(f.Ask).DivInt(2)
		f.AskSize = t.Levels[0].AskSz
		f.BidSize = t.Levels[0].BidSz
	}
}

func onOptionDef(o *Option) {
	gOptionsByID[o.ID] = o
	gOptionsByStrike.Add(o)
	gOptionsByOSI[o.OSI()] = o
}

func onOptionTick(t *databento.CMBP1) {
	o := gOptionsByID[t.Header.InstrumentID]
	if o == nil {
		return
	}
	if t.Header.TSEvent > o.TS {
		o.TS = t.Header.TSEvent
		bid := t.Levels[0].BidPx
		if bid != databento.UndefPrice {
			o.Bid = decimal.Decimal(bid / 1000)
			o.BidSize = t.Levels[0].BidSz
			o.Got |= GotBid
		} else {
			o.Bid = decimal.Zero
			o.BidSize = 0
			o.Got &^= GotBid
		}
		ask := t.Levels[0].AskPx
		if ask != databento.UndefPrice {
			o.Ask = decimal.Decimal(ask / 1000)
			o.AskSize = t.Levels[0].AskSz
			o.Got |= GotAsk
		} else {
			o.Ask = decimal.Zero
			o.AskSize = 0
			o.Got &^= GotAsk
		}
		if gES != nil && gES.Price.IsPositive() {
			o.ES = gES.Price
			o.Got |= GotES
		} else {
			o.Got &^= GotES
		}
		if (o.Got & (GotBid | GotAsk | GotES)) == (GotBid | GotAsk | GotES) {
			s, ok := gStrikes.Get(o.Strike)
			if ok {
				updateSPXPrice(s)
			} else {
				s, ok = gPendingStrikes.Get(o.Strike)
				if !ok {
					s = &Strike{}
					gPendingStrikes.Put(o.Strike, s)
				}
				if o.Class == databento.InstrumentClassCall {
					s.Call = o
				} else {
					s.Put = o
				}
				if s.IsReady() {
					gPendingStrikes.Remove(o.Strike)
					gStrikes.Put(o.Strike, s)
					updateSPXPrice(s)
				}
			}
		}
		o.UpdateDelta()
	}
}
