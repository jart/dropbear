package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds/options"
	"dropbear/ds/osi"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/emirpasic/gods/v2/sets/treeset"
)

var (
	gTotalFees         decimal.Decimal
	gSchwabClient      *schwab.Client
	gSimulationUpdates = make(chan SimulationUpdate, 20)
	gPendingOrders     = treeset.NewWith(compareSimulations)
)

type SimulationUpdate struct {
	Simulation *Simulation
	OrderID    schwab.OrderID
}

func live() {

	// subscribe to databento data
	key := databento.MustLoadDefaultKey()
	optionDefs := make(chan *options.Option, 64)
	optionTicks := make(chan *databento.CMBP1, 256)
	go streamOptions(key, optionDefs, optionTicks)

	// connect to schwab api
	gSchwabClient = schwab.NewClient()
	orderUpdates := gSchwabClient.OrderUpdates()

	// varu is the trading strategy with a heart
	heartbeat := clocky.NewTicker(*heartbeatFlag)
	readySteadyGo := clocky.NewTicker(10 * clocky.Second)
	ready := false

	for {
		// drain all pending events
		select {
		case o := <-optionDefs:
			onOptionDef(o)
			continue
		case t := <-optionTicks:
			onOptionTick(t)
			continue
		case update := <-orderUpdates:
			onOrderUpdate(update)
			continue
		case simulationUpdate := <-gSimulationUpdates:
			onSimulationOrderID(simulationUpdate.Simulation, simulationUpdate.OrderID)
			continue
		case <-heartbeat.C:
			onHeartbeat()
			continue
		default:
			// all channels empty
		}
		// let's go
		if ready {
			cancelUnfilledOrders()
			onThink()
		}
		// block until next event
		select {
		case o := <-optionDefs:
			onOptionDef(o)
		case t := <-optionTicks:
			onOptionTick(t)
		case update := <-orderUpdates:
			onOrderUpdate(update)
		case simulationUpdate := <-gSimulationUpdates:
			onSimulationOrderID(simulationUpdate.Simulation, simulationUpdate.OrderID)
		case <-heartbeat.C:
			onHeartbeat()
		case <-readySteadyGo.C:
			restorePortfolio()
			ready = true
		}
	}
}

// restorePortfolio reloads portfolio across command invocations.
func restorePortfolio() {
	now := clocky.Now()
	orders, err := gSchwabClient.GetOrders(&schwab.GetOrdersRequest{
		FromEnteredTime: clocky.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, clocky.UTC),
		ToEnteredTime:   clocky.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, clocky.UTC),
		Status:          schwab.OrderStatusFilled,
	})
	if err != nil {
		log.Printf("warning: failed to reload previous orders from schwab for today: %v", err)
		os.Exit(1)
	}
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
				for range execLeg.Quantity.Int() {
					cost := price.Mul(kMultiplier)
					gCash = gCash.Add(cost)
					sym := option.OSI()
					pos, _ := gPositions.Get(sym)
					if cost.IsNegative() {
						pos = pos.Add(decimal.One) // bought
					} else {
						pos = pos.Sub(decimal.One) // sold
					}
					gPositions.Put(sym, pos)
				}
			}
		}
	}
}

func streamOptions(key databento.ApiKey, defs chan<- *options.Option, ticks chan<- *databento.CMBP1) {
	client, err := databento.Dial("OPRA.PILLAR", key)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer client.Close()
	dbSymbol := fmt.Sprintf("%s.OPT", gSymbol)
	client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaDefinition,
		SType:   databento.STypeParent,
		Symbols: []string{dbSymbol},
	})
	client.Subscribe(databento.Subscription{
		Schema:  databento.SchemaCMBP1,
		SType:   databento.STypeParent,
		Symbols: []string{dbSymbol},
	})
	meta, err := client.Start()
	if err != nil {
		log.Fatalf("start: %v", err)
	}
	log.Printf("streaming %s (dbn v%d)", dbSymbol, meta.Version)
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
			sym, strike, class, year, month, day, err := osi.Parse(str)
			if err != nil {
				log.Printf("failed to parse OSI: %v", err)
				continue
			}
			now := clocky.Now().In(clocky.UTC)
			todayYear, todayMonth, todayDay := now.Date()
			if year != todayYear || time.Month(month) != todayMonth || day != todayDay {
				continue
			}
			defs <- &options.Option{
				ID:     id,
				Class:  databento.InstrumentClass(class),
				Strike: &options.Strike{Price: strike},
				Sym:    sym,
				Year:   year,
				Month:  month,
				Day:    day,
			}
		case *databento.CMBP1:
			ticks <- m
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
		onCancelAcknowledgement(event.BaseEvent.CancelAcceptedEvent)
	} else if event.BaseEvent.OrderFillCompletedEventOrderLegQuantityInfo != nil {
		onFillEvent(event, event.BaseEvent.OrderFillCompletedEventOrderLegQuantityInfo)
	} else if event.BaseEvent.OrderExpiredEvent != nil {
		onExpiredEvent(event.BaseEvent.OrderExpiredEvent)
	} else if event.BaseEvent.ChangeCreatedEventEquityOrder != nil {
		onChangeEvent(event, event.BaseEvent.ChangeCreatedEventEquityOrder)
	} else if event.BaseEvent.OrderUROutCompletedEvent != nil {
		onUROutCompletedEvent(event.BaseEvent.OrderUROutCompletedEvent)
	}
}

func cancelUnfilledOrders() {
	var toCancel []*Simulation
	for it := gPendingOrders.Iterator(); it.Next(); {
		sim := it.Value()
		if sim.OrderID != 0 && clocky.Now().Sub(sim.Created) >= *patienceFlag {
			toCancel = append(toCancel, sim)
		}
	}
	for _, sim := range toCancel {
		go func() {
			err := gSchwabClient.CancelOrder(sim.OrderID)
			if err != nil {
				log.Printf("failed to cancel order id %d for simulation %d: %v", sim.OrderID, sim.Sequence, err)
				os.Exit(1)
			}
		}()
		delete(gSimulationsByID, sim.OrderID)
		gPendingOrders.Remove(sim)
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
		log.Printf("warning: onChangeEvent only supports limit orders")
		return
	}
	limitPrice := change.Order.Order.AssetOrderEquityOrderLeg.OrderInstruction.ExecutionStrategy.LimitExecutionStrategy.LimitPrice
	legs := change.Order.Order.AssetOrderEquityOrderLeg.OrderLegs
	if len(legs) != 1 {
		log.Printf("warning: onChangeEvent only supports single-leg orders")
		return
	}
	if legs[0].BuySellCode == "Buy" {
		limitPrice = limitPrice.Neg()
	}
	log.Printf("order id changed %d->%s with limit price %s->%s", oldID, newID, sim.Price, limitPrice)
	sim.Price = limitPrice
	sim.OrderID = newID
	gSimulationsByID[newID] = sim
	delete(gSimulationsByID, oldID)
}

func onCancelAcknowledgement(cancel *schwab.CancelAcknowledgement) {
	for _, info := range cancel.LegCancelRequestInfoList {
		sim := gSimulationsByID[info.LegID]
		if sim == nil {
			log.Printf("warning: order id %d not found for cancel event", info.LegID)
			continue
		}
		if info.LegID != sim.OrderID {
			log.Printf("warning: leg for order id %d does not match cancel event leg id %d: %s", sim.OrderID, info.LegID, sim)
			continue
		}
		if info.ChangedNewSchwabOrderID != 0 {
			log.Printf("warning: got replacement cancel event for order id %d leg that still exists: %s", info.LegID, sim)
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

func onUROutCompletedEvent(uro *schwab.OrderUROutCompleted) {
	sim := gSimulationsByID[uro.LegID]
	if sim == nil {
		log.Printf("warning: order id %d not found for urout event", uro.LegID)
		return
	}
	if uro.LegID != sim.OrderID {
		log.Printf("warning: leg for order id %d does not match urout event leg id %d: %s", sim.OrderID, uro.LegID, sim)
		return
	}
	if uro.LegStatus != "LegClosed" {
		log.Printf("warning: leg for order id %d has unexpected leg status for urout event: %s", uro.LegID, uro.LegStatus)
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
	cash := fillPrice.Mul(qty.Abs()).Mul(kMultiplier)
	if qty.IsPositive() {
		cash = cash.Neg() // bought: we pay cash
	}
	gCash = gCash.Add(cash)

	// update position for this filled leg
	existing, _ := gPositions.Get(sym)
	gPositions.Put(sym, existing.Add(qty))
	log.Printf("leg filled for order id %d: %s %s @ %s, route: %s, improvement: %s, fee: %s",
		orderID, fill.OrderInfoForTransactionPosting.BuySellCode, sym, fillPrice, routeName, priceImprovement, fee)

	// remove filled leg from simulation; clean up when all legs are done
	sim.Legs.Remove(sym)
	if sim.Legs.Empty() {
		delete(gSimulationsByID, sim.OrderID)
		gPendingOrders.Remove(sim)
		log.Printf("order complete for order id %d: %s", orderID, sim.Strategy)
	}
}

func onExpiredEvent(ee *schwab.ExpiredEvent) {
	sim := gSimulationsByID[ee.LegID]
	if sim == nil {
		log.Printf("warning: order id %d not found for expired event", ee.LegID)
		return
	}
	if ee.LegID != sim.OrderID {
		log.Printf("warning: leg for order id %d does not match expired event leg id %d: %s", sim.OrderID, ee.LegID, sim)
		return
	}
	if ee.LegStatus != "LegClosed" {
		log.Printf("warning: leg for order id %d has unexpected leg status for expired event: %s", ee.LegID, ee.LegStatus)
	}
	log.Printf("leg expired for order id %d: %s", ee.LegID, sim)
	onOrderCancel(sim)
}

func onSimulationOrderID(sim *Simulation, orderID schwab.OrderID) {
	sim.OrderID = orderID
	gSimulationsByID[orderID] = sim
}

func onOrderCancel(sim *Simulation) {
	delete(gSimulationsByID, sim.OrderID)
	gPendingOrders.Remove(sim)
	log.Printf("leg canceled for order id %d: %s", sim.OrderID, sim)
}
