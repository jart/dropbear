package main

import (
	"flag"

	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/options"
	"dropbear/ds/symbol"
	"dropbear/loggy"

	"github.com/emirpasic/gods/v2/maps/treemap"
	"github.com/emirpasic/gods/v2/sets/hashset"
	"github.com/emirpasic/gods/v2/sets/linkedhashset"
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
	gTotalFees           decimal.Decimal
	gSPX                 = options.NewOptions()
	gFuturesByID         = make(map[uint32]*Future)
	gOptionsByID         = make(map[uint32]*options.Option)
	gOptionsByOSI        = make(map[string]*options.Option)
	gLegsByOrderID       = make(map[schwab.OrderID]*Leg)
	gRestrictedToBuying  = hashset.New[uint32]()
	gRestrictedToSelling = hashset.New[uint32]()
	gHoldings            = treemap.New[string, decimal.Decimal]()
	gPendingStrikes      = treemap.New[decimal.Decimal, *options.Strike]()
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
	optionDefs := make(chan *options.Option, 64)
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
