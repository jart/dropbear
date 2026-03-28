package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/ds/options"
	"dropbear/ds/osi"
	"dropbear/loggy"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
)

var (
	gSchwabClient     *schwab.Client
	gOrderUpdates     = make(chan OrderUpdate, 20)
	gOrdersBySchwabID = map[schwab.OrderID]*Order{}
)

type OrderUpdate struct {
	Order   *Order
	OrderID schwab.OrderID
}

func live() {
	loggy.AlsoLogToFile()
	log.Printf("varu is on the prowl")

	// avoid confusion if we woke up too late
	if clocky.Now().ClockInt() >= kStopTrading {
		log.Printf("warning: varu stops day trading after %02d:%02d hours", kStopTrading/1_00_00, kStopTrading/1_00%1_00)
	}

	// subscribe to databento data
	key := databento.MustLoadDefaultKey()
	optionDefs := make(chan *options.Option, 256)
	optionTicks := make(chan *databento.CMBP1, 256)
	go streamOptions(key, optionDefs, optionTicks)

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
		case o := <-optionDefs:
			onOptionDef(o)
			continue
		case t := <-optionTicks:
			o := onOptionTick(t)
			if _, ok := gPendingOrdersByOption[o]; ok {
				cancelUnfilledOrders(clocky.Now())
			}
			continue
		case update := <-orderUpdates:
			onOrderUpdate(update)
			broadcastState()
			continue
		case orderUpdate := <-gOrderUpdates:
			onOrderOrderID(orderUpdate.Order, orderUpdate.OrderID)
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
		if !ready {
			loggy.Hint("not trading: waiting for market data")
		} else {
			onThought(clocky.Now())
		}
		// block until next event
		select {
		case o := <-optionDefs:
			onOptionDef(o)
		case t := <-optionTicks:
			o := onOptionTick(t)
			if _, ok := gPendingOrdersByOption[o]; ok {
				cancelUnfilledOrders(clocky.Now())
			}
		case update := <-orderUpdates:
			onOrderUpdate(update)
			broadcastState()
		case orderUpdate := <-gOrderUpdates:
			onOrderOrderID(orderUpdate.Order, orderUpdate.OrderID)
		case req := <-gWebRequests:
			processWebRequest(req)
		case <-heartbeat.C:
			onHeartbeat()
		case <-dumpTimer.C:
			broadcastState()
		case <-readySteadyGo.C:
			if !ready && gChain.LastPopulate != 0 && clocky.Now().After(gChain.LastPopulate.Add(clocky.Second)) {
				restorePortfolio()
				onOptionDefEnd()
				broadcastState()
				ready = true
			}
		}
	}
}

// streamOptions subscribes to both definitions and quotes on a single
// live gateway connection. Definitions arrive via SymbolMappingMsg which
// maps parent symbols to instrument IDs with OSI names. This avoids the
// historical API which can time out.
func streamOptions(key databento.ApiKey, defs chan<- *options.Option, ticks chan<- *databento.CMBP1) {
	client, err := databento.Dial("OPRA.PILLAR", key)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer client.Close()
	dbSymbol := fmt.Sprintf("%s.OPT", gSymbol)
	client.MustSubscribe(databento.Subscription{
		Schema:  databento.SchemaDefinition,
		SType:   databento.STypeParent,
		Symbols: []string{dbSymbol},
	})
	client.MustSubscribe(databento.Subscription{
		Schema:  databento.SchemaCMBP1,
		SType:   databento.STypeParent,
		Symbols: []string{dbSymbol},
	})
	meta := client.MustStart()
	log.Printf("streaming %s (dbn v%d)", dbSymbol, meta.Version)
	wantYear, wantMonth, wantDay := clocky.Now().Date()
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
		case *databento.SymbolMappingMsg:
			id := m.Header.InstrumentID
			str := m.GetSTypeOutSymbol()
			sym, strike, class, year, monthy, day, err := osi.Parse(str)
			if err != nil {
				continue
			}
			if sym != gSymbol || year != wantYear || clocky.Month(monthy) != wantMonth || day != wantDay {
				continue
			}
			log.Printf("got option definition: %s (id %d)", str, id)
			defs <- &options.Option{
				ID:     id,
				Class:  databento.InstrumentClass(class),
				Strike: &options.Strike{Price: strike},
				Symbol: sym,
				Year:   year,
				Month:  clocky.Month(monthy),
				Day:    day,
			}
		case *databento.CMBP1:
			ticks <- m
		case *databento.Instrument:
			// ignore raw definitions; we use SymbolMappingMsg instead
		default:
			log.Printf("unknown record type: %T", m)
		}
	}
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
				fillQuantity := execLeg.Quantity
				switch leg.Instruction {
				case schwab.InstructionSell, schwab.InstructionSellToOpen, schwab.InstructionSellToClose:
					fillQuantity = fillQuantity.Neg()
				}
				gHoldings.Add(option, fillQuantity, execLeg.Price)
			}
		}
	}
}

func sendLiveOrder(order *Order) {
	orderType := schwab.OrderTypeNetCredit
	if order.Price.IsNegative() {
		orderType = schwab.OrderTypeNetDebit
	}
	schwabOrder := &schwab.Order{
		OrderType:         orderType,
		Price:             order.Price.Abs(),
		OrderStrategyType: schwab.OrderStrategyTypeSingle,
	}
	for _, leg := range order.Legs {
		var instruction schwab.Instruction
		holding := gHoldings.Positions[leg.Option]
		if leg.Quantity.IsPositive() {
			if holding != nil && holding.Quantity.IsNegative() {
				instruction = schwab.InstructionBuyToClose
			} else {
				instruction = schwab.InstructionBuyToOpen
			}
		} else {
			if holding != nil && holding.Quantity.IsPositive() {
				instruction = schwab.InstructionSellToClose
			} else {
				instruction = schwab.InstructionSellToOpen
			}
		}
		schwabOrder.OrderLegCollection = append(schwabOrder.OrderLegCollection, schwab.OrderLeg{
			Quantity:    leg.Quantity.Abs(),
			Instruction: instruction,
			Instrument: schwab.Instrument{
				AssetType: schwab.AssetTypeOption,
				Symbol:    leg.Option.OSI(),
			},
		})
	}
	addPendingOrder(order)
	go func() {
		orderID, err := gSchwabClient.CreateOrder(schwabOrder)
		if err != nil {
			log.Printf("#%d failed to place order: %v", order.ID, err)
		}
		gOrderUpdates <- OrderUpdate{
			Order:   order,
			OrderID: orderID,
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

func schwabCancelOrder(sim *Order) {
	err := gSchwabClient.CancelOrder(sim.OrderID)
	if err != nil {
		log.Printf("warning: #%d failed to cancel order id %d: %v", sim.ID, sim.OrderID, err)
	}
}

func onChangeEvent(event *schwab.OrderEvent, change *schwab.OrderChangeEvent) {
	newID := event.SchwabOrderID
	oldID := change.ParentSchwabOrderID
	if gOrdersBySchwabID[newID] != nil {
		log.Printf("warning: order id %d already exists for change event", newID)
		return
	}
	sim := gOrdersBySchwabID[oldID]
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
	gOrdersBySchwabID[newID] = sim
	delete(gOrdersBySchwabID, oldID)
}

func onCancelAcknowledgement(event *schwab.OrderEvent, cancel *schwab.CancelAcknowledgement) {
	for _, info := range cancel.LegCancelRequestInfoList {
		sim := gOrdersBySchwabID[info.LegID]
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
	sim := gOrdersBySchwabID[event.SchwabOrderID]
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
	gHoldings.TotalFees = gHoldings.TotalFees.Add(fee)
	routeName := fill.ExecutionInfo.RouteName
	fillPrice := fill.ExecutionInfo.ExecutionPrice
	priceImprovement := fill.PriceImprovement.DivInt(100)
	orderID := event.SchwabOrderID

	// find relevant order
	order := gOrdersBySchwabID[orderID]
	if order == nil {
		log.Printf("warning: order id %d not found for fill event", orderID)
		return
	}

	// get relevant option
	osiSymbol := fill.OrderInfoForTransactionPosting.Symbol
	option := gOptionsByOSI[osiSymbol]
	if option == nil {
		log.Printf("warning: option with osi symbol %s not found for fill event for order id %d", osiSymbol, orderID)
		return
	}

	// update holdings
	fillQuantity := fill.Quantity()
	gHoldings.Add(option, fillQuantity, fillPrice)
	log.Printf("#%d leg filled for order id %d: %s %s @ %s, route: %s, improvement: %s, fee: %s",
		order.ID, orderID, fill.OrderInfoForTransactionPosting.BuySellCode, option, fillPrice, routeName, priceImprovement, fee)

	// mark leg filled
	for _, leg := range order.Legs {
		if leg.Option == option {
			leg.Filled = true
			break
		}
	}

	// remove fully filled orders
	if order.Filled() {
		deleteSchwabOrder(order)
		gStrategiesUsed[order.Strategy] += 1
		log.Printf("#%d order complete for order id %d: %s", order.ID, order.OrderID, order.Strategy)
		// trade instantly without delay once an order is filled
		gNextTradeTime = clocky.Now()
	}
}

func onExpiredEvent(event *schwab.OrderEvent, ee *schwab.ExpiredEvent) {
	sim := gOrdersBySchwabID[event.SchwabOrderID]
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

func onOrderOrderID(order *Order, orderID schwab.OrderID) {
	if orderID != 0 {
		order.OrderID = orderID
		gOrdersBySchwabID[orderID] = order
		log.Printf("#%d got order id %d for order: %s", order.ID, orderID, order)
	} else {
		deleteSchwabOrder(order)
	}
}

func onOrderCancel(order *Order) {
	deleteSchwabOrder(order)
	log.Printf("leg canceled for order id %d: %s", order.OrderID, order)
}

func deleteSchwabOrder(order *Order) {
	delete(gOrdersBySchwabID, order.OrderID)
	removePendingOrder(order)
}
