package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/ds/options"
	"dropbear/ds/osi"
	"dropbear/ds/symbol"
	"dropbear/loggy"
	"encoding/json"
	"io"
	"log"
	"os"
	"strconv"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

var (
	gSchwabClient      *schwab.Client
	gSimulationUpdates = make(chan SimulationUpdate, 20)
	gPendingOrders     = treeset.NewWith(compareSimulations)
)

type SimulationUpdate struct {
	Simulation *Simulation
	OrderID    schwab.OrderID
}

func live() {
	loggy.AlsoLogToFile()

	// avoid confusion if we woke up too late
	if clocky.Now().ClockInt() >= kStopTrading {
		log.Printf("warning: varu stops day trading after %02d:%02d hours", kStopTrading/1_00_00, kStopTrading/1_00%1_00)
	}

	// subscribe to databento data
	key := databento.MustLoadDefaultKey()
	databentoHistoricalClient := databento.NewHistoricalClient(key)
	fetchDefinitions(databentoHistoricalClient)
	optionTicks := make(chan *databento.CMBP1, 256)
	go streamOptions(key, optionTicks)

	// connect to schwab api
	gSchwabClient = schwab.NewClient()
	orderUpdates := gSchwabClient.OrderUpdates()

	// varu is the trading strategy with a heart
	heartbeat := clocky.NewTicker(*heartbeatFlag)
	defer heartbeat.Stop()

	// how often to generate json for web dashboard
	dumpTimer := clocky.NewTicker(*slowdownFlag)
	defer dumpTimer.Stop()

	// we must wait for options chain to become available
	readySteadyGo := clocky.NewTicker(clocky.Second)
	defer readySteadyGo.Stop()
	ready := false

	for {
		// drain all pending events
		select {
		case t := <-optionTicks:
			onOptionTick(t)
			continue
		case update := <-orderUpdates:
			onOrderUpdate(update)
			broadcastState()
			continue
		case simulationUpdate := <-gSimulationUpdates:
			onSimulationOrderID(simulationUpdate.Simulation, simulationUpdate.OrderID)
			continue
		case req := <-gWebRequests:
			processWebRequest(req)
			continue
		case <-heartbeat.C:
			onHeartbeat()
			continue
		case <-dumpTimer.C:
			broadcastState()
			continue
		default:
			// all channels empty
		}
		// let's go
		if ready && !gPaused && !*dryFlag {
			now := clocky.Now()
			clock := now.ClockInt()
			if clock >= kStartOfDay && now >= gNextTradeTime && gPendingOrders.Size() < *maxPendingFlag {
				onThink(now)
			}
			cancelUnfilledOrders(now)
		}
		// block until next event
		select {
		case t := <-optionTicks:
			onOptionTick(t)
		case update := <-orderUpdates:
			onOrderUpdate(update)
			broadcastState()
		case simulationUpdate := <-gSimulationUpdates:
			onSimulationOrderID(simulationUpdate.Simulation, simulationUpdate.OrderID)
		case req := <-gWebRequests:
			processWebRequest(req)
		case <-heartbeat.C:
			onHeartbeat()
		case <-dumpTimer.C:
			broadcastState()
		case <-readySteadyGo.C:
			if !ready && gChain.LastPopulate != 0 && clocky.Now().After(gChain.LastPopulate.Add(clocky.Second)) {
				restorePortfolio()
				broadcastState()
				ready = true
			}
		}
	}
}

func fetchDefinitions(client *databento.HistoricalClient) {
	now := clocky.Now()
	yesterday := now.Add(-clocky.Day)
	start := clocky.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, clocky.UTC)
	end := clocky.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 59, 59, 0, clocky.UTC)
	resp, err := client.GetRange(databento.GetRangeParams{
		Dataset: "OPRA.PILLAR",
		Schema:  databento.SchemaDefinition,
		STypeIn: databento.STypeParent,
		Symbols: []string{gSymbol.String() + ".OPT"},
		Start:   start,
		End:     end,
	})
	if err != nil {
		log.Fatal("fetch definitions:", err)
	}
	defer resp.Close()
	wantYear, wantMonth, wantDay := now.Date()
	for {
		rec, err := resp.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal("read definition:", err)
		}
		inst, ok := rec.(*databento.Instrument)
		if !ok {
			continue
		}
		str := inst.GetRawSymbol()
		sym, strike, class, year, monthy, day, err := osi.Parse(str)
		if err != nil {
			log.Printf("failed to parse OSI: %v", err)
			continue
		}
		month := clocky.Month(monthy)
		if sym == gSymbol && year == wantYear && monthy == wantMonth && day == wantDay {
			log.Printf("got option definition: %s (id %d, raw symbol: %s)", str, inst.Header.InstrumentID, inst.GetRawSymbol())
			onOptionDef(&options.Option{
				ID:     inst.Header.InstrumentID,
				Class:  databento.InstrumentClass(class),
				Strike: &options.Strike{Price: strike},
				Sym:    symbol.MustParse(inst.GetUnderlying()),
				Year:   year,
				Month:  month,
				Day:    day,
			})
		}
	}
	if len(gOptionsByID) == 0 {
		log.Fatalf("no instruments found for %s expiring on %04d-%02d-%02d", gSymbol, wantYear, wantMonth, wantDay)
	}
	onOptionDefEnd()
}

func getOptionInstrumentIDs() []string {
	ids := make([]string, 0, len(gOptionsByID))
	for id := range gOptionsByID {
		ids = append(ids, strconv.FormatInt(int64(id), 10))
	}
	if len(ids) == 0 {
		panic("no option instrument ids found")
	}
	return ids
}

// restorePortfolio reloads portfolio across command invocations.
func restorePortfolio() {
	now := clocky.Now()
	startTime := clocky.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, clocky.UTC)
	endTime := clocky.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, clocky.UTC)
	orders, err := gSchwabClient.GetOrders(&schwab.GetOrdersRequest{
		FromEnteredTime: startTime,
		ToEnteredTime:   endTime,
		Status:          schwab.OrderStatusFilled,
	})
	if err != nil {
		log.Printf("warning: failed to reload previous orders from schwab for today: %v", err)
		os.Exit(1)
	}
	log.Printf("restoring portfolio from %d schwab orders placed today twixt %s to %s", len(orders), startTime, endTime)
	for i := range orders {
		loadSchwabOrder(&orders[i])
	}
}

func loadSchwabOrder(order *schwab.Order) {
	if order.Status != schwab.OrderStatusFilled {
		return
	}
	for _, leg := range order.OrderLegCollection {
		if leg.Instrument.AssetType != schwab.AssetTypeOption {
			continue
		}
		option := gOptionsByOSI[leg.Instrument.Symbol]
		if option == nil {
			continue
		}
		for _, activity := range order.OrderActivityCollection {
			if activity.ExecutionType != schwab.ExecutionTypeFill {
				continue
			}
			for _, execLeg := range activity.ExecutionLegs {
				if execLeg.InstrumentID != leg.Instrument.InstrumentID {
					continue
				}
				if execLeg.LegID != leg.LegID {
					continue
				}
				price := execLeg.Price
				switch leg.Instruction {
				case schwab.InstructionBuyToOpen, schwab.InstructionBuyToClose:
					price = price.Neg()
				}
				sym := option.OSI()
				fillQty := execLeg.Quantity
				if price.IsPositive() {
					fillQty = fillQty.Neg() // sold
				}
				qty := execLeg.Quantity.Abs().Int()
				gVolume += qty
				gTotalFees = gTotalFees.Add(kFeePerContract.MulInt(qty))
				log.Printf("restoring filled leg for order id %d: %s %s %s @ %s",
					order.OrderID, fillQty, leg.Instruction, sym, execLeg.Price)
				cashFlow := price.Mul(execLeg.Quantity).MulInt(kMultiplier)
				gCash = gCash.Add(cashFlow)
				if cashFlow.IsPositive() {
					gTotalCashIn = gTotalCashIn.Add(cashFlow)
				} else {
					gTotalCashOut = gTotalCashOut.Add(cashFlow.Neg())
				}
				pos, _ := gPositions.Get(sym)
				gPositions.Put(sym, pos.Add(fillQty))
				recordFill(sym, fillQty, execLeg.Price)
			}
		}
	}
	sanityCheck("loadSchwabOrders")
}

func streamOptions(key databento.ApiKey, ticks chan<- *databento.CMBP1) {
	client, err := databento.Dial("OPRA.PILLAR", key)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer client.Close()
	client.MustSubscribe(databento.Subscription{
		Schema:  databento.SchemaCMBP1,
		SType:   databento.STypeInstrumentID,
		Symbols: getOptionInstrumentIDs(),
	})
	client.MustStart()
	for {
		rec, err := client.Read()
		if err != nil {
			if err == io.EOF {
				return
			}
			log.Fatalf("decode: %v", err)
		}
		switch m := rec.(type) {
		case *databento.ErrorMsg:
			log.Fatalf("option gateway error: %s", m.Err)
		case *databento.SystemMsg:
			log.Printf("option system message: %s", m.Msg)
		case *databento.CMBP1:
			ticks <- m
		case *databento.SymbolMappingMsg:
			// dropout
		default:
			log.Printf("unknown record type: %T", m)
		}
	}
}

func sendLiveOrder(sim *Simulation) {
	orderType := schwab.OrderTypeNetCredit
	if sim.Price.IsNegative() {
		orderType = schwab.OrderTypeNetDebit
	}
	order := &schwab.Order{
		OrderType:         orderType,
		Price:             sim.Price.Abs(),
		OrderStrategyType: schwab.OrderStrategyTypeSingle,
	}
	for legIt := sim.Legs.Iterator(); legIt.Next(); {
		sym, qty := legIt.Key(), legIt.Value()
		if qty.IsZero() {
			continue
		}
		var instruction schwab.Instruction
		holding, _ := gPositions.Get(sym)
		if qty.IsPositive() {
			if holding.IsNegative() {
				instruction = schwab.InstructionBuyToClose
			} else {
				instruction = schwab.InstructionBuyToOpen
			}
		} else {
			if holding.IsPositive() {
				instruction = schwab.InstructionSellToClose
			} else {
				instruction = schwab.InstructionSellToOpen
			}
		}
		order.OrderLegCollection = append(order.OrderLegCollection, schwab.OrderLeg{
			Quantity:    qty.Abs(),
			Instruction: instruction,
			Instrument: schwab.Instrument{
				AssetType: schwab.AssetTypeOption,
				Symbol:    sym,
			},
		})
	}
	gPendingOrders.Add(sim)
	go func() {
		orderID, err := gSchwabClient.CreateOrder(order)
		if err != nil {
			log.Printf("failed to place order: %v", err)
			os.Exit(1)
		}
		gSimulationUpdates <- SimulationUpdate{
			Simulation: sim,
			OrderID:    orderID,
		}
	}()
}

func onOrderUpdate(event *schwab.OrderEvent) {
	pretty, _ := json.MarshalIndent(json.RawMessage(event.RawData), "  ", "  ")
	log.Printf("order %s: %s\n  %s", event.SchwabOrderID, event.BaseEvent.EventType, pretty)
	if event.BaseEvent.CancelAcceptedEvent != nil {
		onCancelAcknowledgement(event, event.BaseEvent.CancelAcceptedEvent)
	} else if event.BaseEvent.OrderFillCompletedEventOrderLegQuantityInfo != nil {
		onFillEvent(event, event.BaseEvent.OrderFillCompletedEventOrderLegQuantityInfo)
	} else if event.BaseEvent.OrderExpiredEvent != nil {
		onExpiredEvent(event, event.BaseEvent.OrderExpiredEvent)
	} else if event.BaseEvent.ChangeCreatedEventEquityOrder != nil {
		onChangeEvent(event, event.BaseEvent.ChangeCreatedEventEquityOrder)
	} else if event.BaseEvent.OrderUROutCompletedEvent != nil {
		onUROutCompletedEvent(event, event.BaseEvent.OrderUROutCompletedEvent)
	}
}

func cancelUnfilledOrders(now clocky.Time) {
	for it := gPendingOrders.Iterator(); it.Next(); {
		sim := it.Value()
		if sim.OrderID != 0 && !sim.Canceling && now.Sub(sim.Created) >= *patienceFlag {
			sim.Canceling = true
			go cancelOrder(sim)
		}
	}
}

func cancelOrder(sim *Simulation) {
	log.Printf("#%d canceling order id %d for being unfilled after %s: %s", sim.ID, sim.OrderID, *patienceFlag, sim)
	err := gSchwabClient.CancelOrder(sim.OrderID)
	if err != nil {
		log.Printf("warning: #%d failed to cancel order id %d: %v", sim.ID, sim.OrderID, err)
		// could legitimately fail, e.g. "Order in state FILLED cannot be canceled"
	}
}

func onChangeEvent(event *schwab.OrderEvent, change *schwab.OrderChangeEvent) {
	newID := event.SchwabOrderID
	oldID := change.ParentSchwabOrderID
	if gSimulationsByID[newID] != nil {
		log.Printf("warning: order id %d already exists for change event", newID)
		return
	}
	sim := gSimulationsByID[oldID]
	if sim == nil {
		log.Printf("warning: old order id %d not found for change event order id %s", oldID, newID)
		return
	}
	if change.Order.Order.AssetOrderEquityOrderLeg.OrderInstruction.ExecutionStrategy.LimitExecutionStrategy == nil {
		log.Printf("#%d warning: onChangeEvent only supports limit orders", sim.ID)
		return
	}
	limitPrice := change.Order.Order.AssetOrderEquityOrderLeg.OrderInstruction.ExecutionStrategy.LimitExecutionStrategy.LimitPrice
	legs := change.Order.Order.AssetOrderEquityOrderLeg.OrderLegs
	if len(legs) != 1 {
		log.Printf("#%d warning: onChangeEvent only supports single-leg orders", sim.ID)
		return
	}
	if legs[0].BuySellCode == "Buy" {
		limitPrice = limitPrice.Neg()
	}
	log.Printf("#%d order id changed %d->%s with limit price %s->%s", sim.ID, oldID, newID, sim.Price, limitPrice)
	sim.Price = limitPrice
	sim.OrderID = newID
	gSimulationsByID[newID] = sim
	delete(gSimulationsByID, oldID)
}

func onCancelAcknowledgement(event *schwab.OrderEvent, cancel *schwab.CancelAcknowledgement) {
	for _, info := range cancel.LegCancelRequestInfoList {
		sim := gSimulationsByID[info.LegID]
		if sim == nil {
			log.Printf("warning: order id %d not found for cancel event", info.LegID)
			continue
		}
		if info.LegID != sim.OrderID {
			log.Printf("warning: #%d leg for order id %d does not match cancel event leg id %d: %s", sim.ID, sim.OrderID, info.LegID, sim)
			continue
		}
		if info.ChangedNewSchwabOrderID != 0 {
			log.Printf("warning: #%d got replacement cancel event for order id %d leg that still exists: %s", sim.ID, info.LegID, sim)
			continue
		}
		if info.LegStatus == "LegClosed" {
			// haven't seen this happen yet. the leg will usually be open when a
			// cancel is acknowledged, after which it gets routed to market makers
			// and a urout event is generated when the leg is closed.
			onOrderCancel(sim)
		}
	}
}

func onUROutCompletedEvent(event *schwab.OrderEvent, uro *schwab.OrderUROutCompleted) {
	sim := gSimulationsByID[event.SchwabOrderID]
	if sim == nil {
		log.Printf("warning: order id %d not found for urout event", event.SchwabOrderID)
		return
	}
	if event.SchwabOrderID != sim.OrderID {
		log.Printf("warning: #%d leg for order id %d does not match urout event leg id %d: %s", sim.ID, sim.OrderID, uro.LegID, sim)
		return
	}
	if uro.LegStatus != "LegClosed" {
		log.Printf("warning: #%d leg for order id %d has unexpected leg status for urout event: %s", sim.ID, sim.OrderID, uro.LegStatus)
		return
	}
	onOrderCancel(sim)
}

func onFillEvent(event *schwab.OrderEvent, fill *schwab.FillEvent) {
	fee := fill.ExecutionInfo.ActualChargedFeesCommissionAndTax.Total()
	gTotalFees = gTotalFees.Add(fee)
	routeName := fill.ExecutionInfo.RouteName
	fillPrice := fill.ExecutionInfo.ExecutionPrice
	priceImprovement := fill.PriceImprovement.DivInt(100)
	orderID := event.SchwabOrderID
	sim := gSimulationsByID[orderID]
	if sim == nil {
		log.Printf("warning: order id %d not found for fill event", orderID)
		return
	}

	// use actual fill price and quantity from execution, not simulated price
	sym := fill.OrderInfoForTransactionPosting.Symbol
	qty := fill.Quantity()
	cash := fillPrice.Mul(qty.Abs()).MulInt(kMultiplier)
	if qty.IsPositive() {
		cash = cash.Neg() // bought: we pay cash
	}
	gCash = gCash.Add(cash)
	if cash.IsPositive() {
		gTotalCashIn = gTotalCashIn.Add(cash)
	} else {
		gTotalCashOut = gTotalCashOut.Add(cash.Neg())
	}

	// update position and cost basis for this filled leg
	existing, _ := gPositions.Get(sym)
	gPositions.Put(sym, existing.Add(qty))
	recordFill(sym, qty, fillPrice)
	gVolume += qty.Abs().Int()
	sanityCheck("onFillEvent")
	log.Printf("#%d leg filled for order id %d: %s %s @ %s, route: %s, improvement: %s, fee: %s",
		sim.ID, orderID, fill.OrderInfoForTransactionPosting.BuySellCode, sym, fillPrice, routeName, priceImprovement, fee)

	// remove filled leg from simulation; clean up when all legs are done
	sim.Legs.Remove(sym)
	if sim.Legs.Empty() {
		deleteOrder(sim)
		log.Printf("#%d order complete for order id %d: %s", sim.ID, sim.OrderID, sim.Strategy)
		// trade instantly without delay once an order is filled
		gNextTradeTime = clocky.Now()
	}
}

func onExpiredEvent(event *schwab.OrderEvent, ee *schwab.ExpiredEvent) {
	sim := gSimulationsByID[event.SchwabOrderID]
	if sim == nil {
		log.Printf("warning: order id %d not found for expired event", event.SchwabOrderID)
		return
	}
	if event.SchwabOrderID != sim.OrderID {
		log.Printf("warning: #%d leg for order id %d does not match expired event leg id %d: %s", sim.ID, sim.OrderID, ee.LegID, sim)
		return
	}
	if ee.LegStatus != "LegClosed" {
		log.Printf("warning: #%d leg for order id %d has unexpected leg status for expired event: %s", sim.ID, sim.OrderID, ee.LegStatus)
	}
	log.Printf("#%d leg expired for order id %d: %s", sim.ID, sim.OrderID, sim)
	onOrderCancel(sim)
}

func onSimulationOrderID(sim *Simulation, orderID schwab.OrderID) {
	sim.OrderID = orderID
	gSimulationsByID[orderID] = sim
	log.Printf("#%d got order id %d for simulation: %s", sim.ID, orderID, sim)
}

func onOrderCancel(sim *Simulation) {
	deleteOrder(sim)
	log.Printf("leg canceled for order id %d: %s", sim.OrderID, sim)
}

func deleteOrder(sim *Simulation) {
	delete(gSimulationsByID, sim.OrderID)
	gPendingOrders.Remove(sim)
}
