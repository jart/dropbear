package main

import (
	"dropbear/broker/databento"
	"dropbear/broker/schwab"
	"dropbear/clocky"
	"dropbear/options"
	"log"
	"os"
)

type OrderUpdateSchwab struct {
	Order   *Order
	OrderID schwab.OrderID
}

func (t *Trader) LiveSchwab() {

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
		case update := <-t.OrderEventsSchwab:
			t.onOrderEventSchwab(update)
			if t.Web != nil {
				t.Web.broadcastState()
			}
			continue
		case orderUpdate := <-t.OrderUpdatesSchwab:
			t.onOrderOrderIDSchwab(orderUpdate.Order, orderUpdate.OrderID)
			continue
		case req := <-t.Web.WebRequests:
			t.Web.processWebRequest(req)
			continue
		case <-heartbeat.C:
			t.onHeartbeat()
			if t.Web != nil {
				t.Web.broadcastState()
			}
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
		case update := <-t.OrderEventsSchwab:
			t.onOrderEventSchwab(update)
			if t.Web != nil {
				t.Web.broadcastState()
			}
		case orderUpdate := <-t.OrderUpdatesSchwab:
			t.onOrderOrderIDSchwab(orderUpdate.Order, orderUpdate.OrderID)
		case req := <-t.Web.WebRequests:
			t.Web.processWebRequest(req)
		case <-heartbeat.C:
			t.onHeartbeat()
			if t.Web != nil {
				t.Web.broadcastState()
			}
		case <-readySteadyGo.C:
			if !ready && t.Chain.LastPopulate != 0 && clocky.Now().After(t.Chain.LastPopulate.Add(clocky.Second)) {
				t.restorePortfolioSchwab()
				t.onDefEnd()
				if t.Web != nil {
					t.Web.broadcastState()
				}
				ready = true
			}
		}
	}
}

func (t *Trader) restorePortfolioSchwab() {
	account, err := gSchwabClient.GetAccount()
	if err != nil {
		log.Printf("failed to fetch schwab account: %v", err)
		os.Exit(1)
	}
	for _, position := range account.SecuritiesAccount.Positions {
		if security := t.SecuritiesByName[position.Instrument.Symbol]; security != nil {
			t.Holdings.Restore(security, position.LongQuantity.Add(position.ShortQuantity), position.AveragePrice)
		}
	}
}

func (t *Trader) sendLiveOrderSchwab(order *Order) {
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
	assetType := schwab.AssetTypeOption
	if _, ok := order.Security.(*options.Equity); ok {
		assetType = schwab.AssetTypeEquity
	}
	schwabOrder := &schwab.Order{
		OrderType:         orderType,
		Price:             order.Price,
		OrderStrategyType: schwab.OrderStrategyTypeSingle,
		OrderLegCollection: []schwab.OrderLeg{{
			Quantity:    order.Quantity.Abs(),
			Instruction: instruction,
			Instrument: schwab.Instrument{
				AssetType: assetType,
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
		t.OrderUpdatesSchwab <- OrderUpdateSchwab{
			Order:   order,
			OrderID: orderID,
		}
	}()
}

func (t *Trader) onOrderEventSchwab(event *schwab.OrderEvent) {
	if event.BaseEvent.CancelAcceptedEvent != nil {
		t.onCancelAcknowledgementSchwab(event, event.BaseEvent.CancelAcceptedEvent)
	} else if event.BaseEvent.OrderFillCompletedEventOrderLegQuantityInfo != nil {
		t.onFillEventSchwab(event, event.BaseEvent.OrderFillCompletedEventOrderLegQuantityInfo)
	} else if event.BaseEvent.OrderExpiredEvent != nil {
		t.onExpiredEventSchwab(event, event.BaseEvent.OrderExpiredEvent)
	} else if event.BaseEvent.ChangeCreatedEventEquityOrder != nil {
		t.onChangeEventSchwab(event, event.BaseEvent.ChangeCreatedEventEquityOrder)
	} else if event.BaseEvent.OrderUROutCompletedEvent != nil {
		t.onUROutCompletedEventSchwab(event, event.BaseEvent.OrderUROutCompletedEvent)
	}
}

func (t *Trader) cancelOrderSchwab(sim *Order) {
	err := gSchwabClient.CancelOrder(sim.OrderIDSchwab)
	if err != nil {
		log.Printf("warning: #%d failed to cancel order id %d: %v", sim.ID, sim.OrderIDSchwab, err)
	}
}

func (t *Trader) onChangeEventSchwab(event *schwab.OrderEvent, change *schwab.OrderChangeEvent) {
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
	sim.OrderIDSchwab = newID
	t.OrdersBySchwabID[newID] = sim
	delete(t.OrdersBySchwabID, oldID)
}

func (t *Trader) onCancelAcknowledgementSchwab(_ *schwab.OrderEvent, cancel *schwab.CancelAcknowledgement) {
	for _, info := range cancel.LegCancelRequestInfoList {
		sim := t.OrdersBySchwabID[info.LegID]
		if sim == nil {
			continue
		}
		if info.LegID != sim.OrderIDSchwab {
			log.Printf("warning: #%d leg for order id %d does not match cancel event leg id %d: %s", sim.ID, sim.OrderIDSchwab, info.LegID, sim)
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
			t.onOrderCancelSchwab(sim)
		}
	}
}

func (t *Trader) onUROutCompletedEventSchwab(event *schwab.OrderEvent, uro *schwab.OrderUROutCompleted) {
	order := t.OrdersBySchwabID[event.SchwabOrderID]
	if order == nil {
		return
	}
	if event.SchwabOrderID != order.OrderIDSchwab {
		log.Printf("warning: #%d leg for order id %d does not match urout event leg id %d: %s", order.ID, order.OrderIDSchwab, uro.LegID, order)
		return
	}
	if uro.LegStatus != "LegClosed" {
		log.Printf("warning: #%d leg for order id %d has unexpected leg status for urout event: %s", order.ID, order.OrderIDSchwab, uro.LegStatus)
		return
	}
	t.onOrderCancelSchwab(order)
}

func (t *Trader) onFillEventSchwab(event *schwab.OrderEvent, fill *schwab.FillEvent) {

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
	log.Printf("#%d order complete for order id %d", order.ID, order.OrderIDSchwab)
}

func (t *Trader) onExpiredEventSchwab(event *schwab.OrderEvent, ee *schwab.ExpiredEvent) {
	order := t.OrdersBySchwabID[event.SchwabOrderID]
	if order == nil {
		return
	}
	if event.SchwabOrderID != order.OrderIDSchwab {
		log.Printf("warning: #%d leg for order id %d does not match expired event leg id %d: %s", order.ID, order.OrderIDSchwab, ee.LegID, order)
		return
	}
	if ee.LegStatus != "LegClosed" {
		log.Printf("warning: #%d leg for order id %d has unexpected leg status for expired event: %s", order.ID, order.OrderIDSchwab, ee.LegStatus)
	}
	log.Printf("#%d leg expired for order id %d: %s", order.ID, order.OrderIDSchwab, order)
	t.onOrderCancelSchwab(order)
}

func (t *Trader) onOrderOrderIDSchwab(order *Order, orderID schwab.OrderID) {
	if orderID != 0 {
		order.OrderIDSchwab = orderID
		t.OrdersBySchwabID[orderID] = order
		log.Printf("#%d got order id %d for order: %s", order.ID, orderID, order)
	} else {
		t.deleteSchwabOrder(order)
	}
}

func (t *Trader) onOrderCancelSchwab(order *Order) {
	t.deleteSchwabOrder(order)
	log.Printf("leg canceled for order id %d: %s", order.OrderIDSchwab, order)
}

func (t *Trader) deleteSchwabOrder(order *Order) {
	delete(t.OrdersBySchwabID, order.OrderIDSchwab)
	t.removePendingOrder(order)
}
