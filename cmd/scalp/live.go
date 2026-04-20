package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/options"
	"dropbear/osi"
	"dropbear/symbol"
	"fmt"
	"io"
	"log"
	"os"
)

type OrderUpdate struct {
	Order   *Order
	OrderID schwab.OrderID
}

func (t *Trader) Live() {

	// subscribe to databento data
	key := databento.MustLoadDefaultKey()
	equityDefs := make(chan *options.Equity, 256)
	optionDefs := make(chan *options.Option, 256)
	equityTicks := make(chan *databento.MBP1, 256)
	optionTicks := make(chan *databento.CMBP1, 256)
	go t.streamEquities(key, equityDefs, equityTicks)
	go t.streamOptions(key, optionDefs, optionTicks)

	// varu is the trading strategy with a heart
	heartbeat := clocky.NewTicker(*heartbeatFlag)
	defer heartbeat.Stop()

	// we must wait for options chain to become available
	readySteadyGo := clocky.NewTicker(clocky.Second)
	defer readySteadyGo.Stop()
	ready := false

	for {
		// drain all pending events
		select {
		case o := <-optionDefs:
			t.onOptionDef(o)
			continue
		case m := <-optionTicks:
			t.onOptionTick(m)
			continue
		case update := <-t.OrderEvents:
			t.onOrderEvent(update)
			continue
		case orderUpdate := <-t.OrderUpdates:
			t.onOrderOrderID(orderUpdate.Order, orderUpdate.OrderID)
			continue
		case <-heartbeat.C:
			t.onHeartbeat()
			continue
		default:
			// all channels empty
		}
		// let's go
		if !ready {
			t.Hinter.Hint("not trading: waiting for market data")
		} else {
			t.onThought(clocky.Now())
		}
		// block until next event
		select {
		case e := <-equityDefs:
			t.onEquityDef(e)
		case o := <-optionDefs:
			t.onOptionDef(o)
		case m := <-equityTicks:
			t.onEquityTick(m)
		case m := <-optionTicks:
			t.onOptionTick(m)
		case update := <-t.OrderEvents:
			t.onOrderEvent(update)
		case orderUpdate := <-t.OrderUpdates:
			t.onOrderOrderID(orderUpdate.Order, orderUpdate.OrderID)
		case <-heartbeat.C:
			t.onHeartbeat()
		case <-readySteadyGo.C:
			if !ready && t.Chain.LastPopulate != 0 && clocky.Now().After(t.Chain.LastPopulate.Add(clocky.Second)) {
				t.restorePortfolio()
				t.onDefEnd()
				ready = true
			}
		}
	}
}

func (t *Trader) streamEquities(key databento.ApiKey, defs chan<- *options.Equity, ticks chan<- *databento.MBP1) {
	client, err := databento.Dial("XNAS.ITCH", key)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer client.Close()
	client.MustSubscribe(databento.Subscription{
		Schema:  databento.SchemaDefinition,
		SType:   databento.STypeParent,
		Symbols: []string{t.Config.Symbol.String()},
	})
	client.MustSubscribe(databento.Subscription{
		Schema:  databento.SchemaMBP1,
		SType:   databento.STypeParent,
		Symbols: []string{t.Config.Symbol.String()},
	})
	meta := client.MustStart()
	log.Printf("streaming %s (dbn v%d)", t.Config.Symbol, meta.Version)
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
			sym, err := symbol.Parse(str)
			if err != nil {
				continue
			}
			if sym != t.Config.Symbol {
				continue
			}
			log.Printf("got equity definition: %s (id %d)", str, id)
			defs <- &options.Equity{
				ID:     id,
				Symbol: sym,
			}
		case *databento.MBP1:
			ticks <- m
		case *databento.Instrument:
			// ignore raw definitions; we use SymbolMappingMsg instead
		default:
			log.Printf("unknown record type: %T", m)
		}
	}
}

// streamOptions subscribes to both definitions and quotes on a single
// live gateway connection. Definitions arrive via SymbolMappingMsg which
// maps parent symbols to instrument IDs with OSI names. This avoids the
// historical API which can time out.
func (t *Trader) streamOptions(key databento.ApiKey, defs chan<- *options.Option, ticks chan<- *databento.CMBP1) {
	client, err := databento.Dial("OPRA.PILLAR", key)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer client.Close()
	dbSymbol := fmt.Sprintf("%s.OPT", t.Config.Symbol)
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
			if sym != t.Config.Symbol || year != wantYear || clocky.Month(monthy) != wantMonth || day != wantDay {
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
func (t *Trader) restorePortfolio() {
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
		t.loadSchwabOrder(&orders[i])
	}
}

func (t *Trader) loadSchwabOrder(order *schwab.Order) {
	if order.Status != schwab.OrderStatusFilled {
		return
	}
	for _, leg := range order.OrderLegCollection {
		if leg.Instrument.AssetType != schwab.AssetTypeOption {
			continue
		}
		security := t.SecuritiesByName[leg.Instrument.Symbol]
		if security == nil {
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
				t.Holdings.Add(security, fillQuantity, execLeg.Price)
			}
		}
	}
}

func (t *Trader) sendLiveOrder(order *Order) {
	orderType := schwab.OrderTypeMarket
	if order.Price.IsPositive() {
		orderType = schwab.OrderTypeLimit
	}
	var instruction schwab.Instruction
	holding := t.Holdings.Positions[order.Security]
	if order.Quantity.IsPositive() {
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
	schwabOrder := &schwab.Order{
		OrderType:         orderType,
		Price:             order.Price,
		OrderStrategyType: schwab.OrderStrategyTypeSingle,
		OrderLegCollection: []schwab.OrderLeg{{
			Quantity:    order.Quantity.Abs(),
			Instruction: instruction,
			Instrument: schwab.Instrument{
				AssetType: schwab.AssetTypeOption,
				Symbol:    order.Security.Name(),
			},
		}},
	}
	t.addPendingOrder(order)
	go func() {
		orderID, err := gSchwabClient.CreateOrder(schwabOrder)
		if err != nil {
			log.Printf("#%d failed to place order: %v", order.ID, err)
		}
		t.OrderUpdates <- OrderUpdate{
			Order:   order,
			OrderID: orderID,
		}
	}()
}

func (t *Trader) onOrderEvent(event *schwab.OrderEvent) {
	if event.BaseEvent.CancelAcceptedEvent != nil {
		t.onCancelAcknowledgement(event, event.BaseEvent.CancelAcceptedEvent)
	} else if event.BaseEvent.OrderFillCompletedEventOrderLegQuantityInfo != nil {
		t.onFillEvent(event, event.BaseEvent.OrderFillCompletedEventOrderLegQuantityInfo)
	} else if event.BaseEvent.OrderExpiredEvent != nil {
		t.onExpiredEvent(event, event.BaseEvent.OrderExpiredEvent)
	} else if event.BaseEvent.ChangeCreatedEventEquityOrder != nil {
		t.onChangeEvent(event, event.BaseEvent.ChangeCreatedEventEquityOrder)
	} else if event.BaseEvent.OrderUROutCompletedEvent != nil {
		t.onUROutCompletedEvent(event, event.BaseEvent.OrderUROutCompletedEvent)
	}
}

func (t *Trader) schwabCancelOrder(sim *Order) {
	err := gSchwabClient.CancelOrder(sim.OrderID)
	if err != nil {
		log.Printf("warning: #%d failed to cancel order id %d: %v", sim.ID, sim.OrderID, err)
	}
}

func (t *Trader) onChangeEvent(event *schwab.OrderEvent, change *schwab.OrderChangeEvent) {
	newID := event.SchwabOrderID
	oldID := change.ParentSchwabOrderID
	if t.OrdersBySchwabID[newID] != nil {
		log.Printf("warning: order id %d already exists for change event", newID)
		return
	}
	sim := t.OrdersBySchwabID[oldID]
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
	t.OrdersBySchwabID[newID] = sim
	delete(t.OrdersBySchwabID, oldID)
}

func (t *Trader) onCancelAcknowledgement(_ *schwab.OrderEvent, cancel *schwab.CancelAcknowledgement) {
	for _, info := range cancel.LegCancelRequestInfoList {
		sim := t.OrdersBySchwabID[info.LegID]
		if sim == nil {
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
			t.onOrderCancel(sim)
		}
	}
}

func (t *Trader) onUROutCompletedEvent(event *schwab.OrderEvent, uro *schwab.OrderUROutCompleted) {
	order := t.OrdersBySchwabID[event.SchwabOrderID]
	if order == nil {
		return
	}
	if event.SchwabOrderID != order.OrderID {
		log.Printf("warning: #%d leg for order id %d does not match urout event leg id %d: %s", order.ID, order.OrderID, uro.LegID, order)
		return
	}
	if uro.LegStatus != "LegClosed" {
		log.Printf("warning: #%d leg for order id %d has unexpected leg status for urout event: %s", order.ID, order.OrderID, uro.LegStatus)
		return
	}
	t.onOrderCancel(order)
}

func (t *Trader) onFillEvent(event *schwab.OrderEvent, fill *schwab.FillEvent) {

	// figure out if fill was on chain we're trading
	name := fill.OrderInfoForTransactionPosting.Symbol
	security := t.SecuritiesByName[name]
	if security == nil {
		return
	}

	// account for fill in holdings and cash
	fillQuantity := fill.Quantity()
	fee := fill.ExecutionInfo.ActualChargedFeesCommissionAndTax.Total()
	t.Holdings.TotalFees = t.Holdings.TotalFees.Add(fee)
	routeName := fill.ExecutionInfo.RouteName
	fillPrice := fill.ExecutionInfo.ExecutionPrice
	t.Holdings.Add(security, fillQuantity, fillPrice)
	priceImprovement := fill.PriceImprovement.DivInt(100)

	// find relevant order
	orderID := event.SchwabOrderID
	order := t.OrdersBySchwabID[orderID]
	if order == nil {
		log.Printf("warning: order id %d not found for fill event (order placed in thinkorswim?)", orderID)
		return
	}

	// update holdings
	log.Printf("#%d filled for order id %d %s %s @ %s, route: %s, improvement: %s, fee: %s",
		order.ID, orderID, fill.OrderInfoForTransactionPosting.BuySellCode, security.Name(), fillPrice, routeName, priceImprovement, fee)

	// mark order filled
	order.Filled = true
	t.deleteSchwabOrder(order)
	log.Printf("#%d order complete for order id %d", order.ID, order.OrderID)
}

func (t *Trader) onExpiredEvent(event *schwab.OrderEvent, ee *schwab.ExpiredEvent) {
	order := t.OrdersBySchwabID[event.SchwabOrderID]
	if order == nil {
		return
	}
	if event.SchwabOrderID != order.OrderID {
		log.Printf("warning: #%d leg for order id %d does not match expired event leg id %d: %s", order.ID, order.OrderID, ee.LegID, order)
		return
	}
	if ee.LegStatus != "LegClosed" {
		log.Printf("warning: #%d leg for order id %d has unexpected leg status for expired event: %s", order.ID, order.OrderID, ee.LegStatus)
	}
	log.Printf("#%d leg expired for order id %d: %s", order.ID, order.OrderID, order)
	t.onOrderCancel(order)
}

func (t *Trader) onOrderOrderID(order *Order, orderID schwab.OrderID) {
	if orderID != 0 {
		order.OrderID = orderID
		t.OrdersBySchwabID[orderID] = order
		log.Printf("#%d got order id %d for order: %s", order.ID, orderID, order)
	} else {
		t.deleteSchwabOrder(order)
	}
}

func (t *Trader) onOrderCancel(order *Order) {
	t.deleteSchwabOrder(order)
	log.Printf("leg canceled for order id %d: %s", order.OrderID, order)
}

func (t *Trader) deleteSchwabOrder(order *Order) {
	delete(t.OrdersBySchwabID, order.OrderID)
	t.removePendingOrder(order)
}
