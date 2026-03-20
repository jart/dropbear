package main

import (
	"dropbear/broker/schwab"
	"encoding/json"
	"log"
)

func onOrderUpdate(event *schwab.OrderEvent) {
	pretty, _ := json.MarshalIndent(json.RawMessage(event.RawData), "  ", "  ")
	log.Printf("order %s: %s\n  %s", event.SchwabOrderID, event.BaseEvent.EventType, pretty)
	if event.BaseEvent.CancelAcceptedEvent != nil {
		onCancelEvent(event.BaseEvent.CancelAcceptedEvent)
	} else if event.BaseEvent.ExecutionRequestedEventRoutedInfo != nil {
		onExecutionRequested(event, event.BaseEvent.ExecutionRequestedEventRoutedInfo)
	} else if event.BaseEvent.OrderFillCompletedEventOrderLegQuantityInfo != nil {
		onFillEvent(event, event.BaseEvent.OrderFillCompletedEventOrderLegQuantityInfo)
	} else if event.BaseEvent.OrderExpiredEvent != nil {
		onExpiredEvent(event, event.BaseEvent.OrderExpiredEvent)
	} else if event.BaseEvent.OrderCreatedEventEquityOrder != nil {
		onCreatedEvent(&event.BaseEvent.OrderCreatedEventEquityOrder.Order)
	} else if event.BaseEvent.ChangeCreatedEventEquityOrder != nil {
		onChangeEvent(event, event.BaseEvent.ChangeCreatedEventEquityOrder)
	}
}

func onCreatedEvent(order *schwab.OrderDetailsWrapper) {
	if order.Order.AssetOrderEquityOrderLeg.OrderInstruction.ExecutionStrategy.LimitExecutionStrategy == nil {
		log.Printf("warning: onCreatedEvent only supports limit orders")
		return
	}
	limitPrice := order.Order.AssetOrderEquityOrderLeg.OrderInstruction.ExecutionStrategy.LimitExecutionStrategy.LimitPrice
	legs := order.Order.AssetOrderEquityOrderLeg.OrderLegs
	if len(legs) != 1 {
		log.Printf("warning: onCreatedEvent only supports single-leg orders")
		return
	}
	if legs[0].BuySellCode == "Buy" {
		limitPrice = limitPrice.Neg()
	}
	osi := legs[0].Security.Symbol
	leg := gLegsByOrderID[order.SchwabOrderID]
	if leg != nil {
		// if we know about the order id then make sure everything checks out
		if leg.OrderID != order.SchwabOrderID {
			log.Printf("warning: order id %d already exists but does not match created event order id %d: %s", leg.OrderID, order.SchwabOrderID, leg)
		}
		if leg.Option.OSI() != osi {
			log.Printf("warning: leg for order id %d already exists but does not match created event OSI %s: %s", leg.OrderID, osi, leg)
		}
		if leg.LimitPrice.Cmp(limitPrice) != 0 {
			log.Printf("warning: leg for order id %d already exists but does not match created event limit price %s: %s", leg.OrderID, limitPrice, leg)
		}
		return
	}
	// Schwab's API doesn't have a ClientOrderID parameter like other brokers.
	// So we have to deal with this race condition where we get websocket events
	// for an order before we get the response from the REST API that creates the leg.
	// This is bad, because we don't know what the ID of our order is yet, so we guess.
	found := 0
	for it := gPendingLegs.Iterator(); it.Next(); {
		l := it.Value()
		if l.Option.OSI() == osi && l.LimitPrice.Cmp(limitPrice) == 0 {
			leg = l
			found += 1
		}
	}
	switch found {
	case 0:
		log.Printf("warning: could not find leg for order id %d with OSI %s and limit price %s", order.SchwabOrderID, osi, limitPrice)
		return
	case 1:
		log.Printf("note: guessing that order id %d maps to: %s", order.SchwabOrderID, leg)
		onLegOrderID(leg, order.SchwabOrderID)
	default:
		log.Printf("warning: could not uniquely guess leg for order id %d", order.SchwabOrderID)
		return
	}
}

func onChangeEvent(event *schwab.OrderEvent, change *schwab.OrderChangeEvent) {
	newID := event.SchwabOrderID
	oldID := change.ParentSchwabOrderID
	if gLegsByOrderID[newID] != nil {
		log.Printf("warning: order id %d already exists for change event", newID)
		return
	}
	leg := gLegsByOrderID[oldID]
	if leg == nil {
		log.Printf("warning: old order id %d not found for change event order id %s", oldID, newID)
		return
	}
	if leg.Complete() && !leg.Closing {
		log.Printf("warning: leg for order id %d is already complete for change event order id %s: %s", oldID, newID, leg)
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
	log.Printf("order id changed %d->%s with limit price %s->%s", oldID, newID, leg.LimitPrice, limitPrice)
	leg.LimitPrice = limitPrice
	leg.OrderID = newID
	gLegsByOrderID[newID] = leg
	delete(gLegsByOrderID, oldID)
}

func onCancelEvent(cancel *schwab.CancelEvent) {
	for _, info := range cancel.LegCancelRequestInfoList {
		leg := gLegsByOrderID[info.LegID]
		if leg == nil {
			log.Printf("warning: order id %d not found for cancel event", info.LegID)
			continue
		}
		if info.ChangedNewSchwabOrderId != 0 {
			log.Printf("warning: got replacement cancel event for order id %d leg that still exists: %s", info.LegID, leg)
			continue
		}
		if leg.Complete() && !leg.Closing {
			log.Printf("warning: leg for order id %d is already complete for cancel event: %s", info.LegID, leg)
			continue
		}
		if leg.Canceling {
			log.Printf("leg canceled for order id %d: %s", info.LegID, leg)
		} else {
			log.Printf("note: leg for order id %d was canceled from thinkorswim: %s", info.LegID, leg)
		}
		leg.Canceling = false
		if leg.Closing {
			leg.Closing = false
		} else {
			leg.Canceled = true
			onLegComplete(leg)
		}
	}
}

func onExecutionRequested(event *schwab.OrderEvent, route *schwab.RouteEvent) {
	if route.RouteInfo.RouteRequestedType != "New" {
		return
	}
	leg := gLegsByOrderID[event.SchwabOrderID]
	if leg == nil {
		log.Printf("warning: order id %d not found for route event: %s @ %s",
			event.SchwabOrderID, route.RouteInfo.RouteName, route.RouteInfo.RoutedPrice)
		return
	}
	leg.RouteName = route.RouteInfo.RouteName
	log.Printf("route %s %s %s %s -> %s @ %s",
		leg.Name, leg.Instruction(), leg.Option.Class,
		leg.Option.Strike,
		route.RouteInfo.RouteName,
		route.RouteInfo.RoutedPrice)
}

func onFillEvent(event *schwab.OrderEvent, fill *schwab.FillEvent) {
	sym := fill.OrderInfoForTransactionPosting.Symbol
	qty := fill.Quantity()
	gHoldings[sym] = gHoldings[sym].Add(qty)
	routeName := fill.ExecutionInfo.RouteName
	fillPrice := fill.ExecutionInfo.ExecutionPrice
	priceImprovement := fill.PriceImprovement.DivInt(100)
	orderID := event.SchwabOrderID
	leg := gLegsByOrderID[orderID]
	if leg == nil {
		log.Printf("warning: order id %d not found for fill event", orderID)
		return
	}
	if leg.Complete() && !leg.Closing {
		log.Printf("warning: leg for order id %d is already complete for fill event: %s", orderID, leg)
		return
	}
	if leg.Canceling {
		log.Printf("note: leg for order id %d was filled while canceling: %s", orderID, leg)
		leg.Canceling = false
	}
	leg.RouteName = routeName
	if leg.Closing {
		if !leg.ClosePrice.IsZero() {
			log.Printf("warning: leg for order id %d already has close price for fill event, but got another fill: %s", orderID, leg)
		}
		leg.Closing = false
		leg.ClosePrice = fillPrice
		log.Printf("leg closed for order id %d at %s with %s improvement from %s route yielding %s profit: %s",
			orderID, fillPrice, priceImprovement, routeName, leg.Profit(), leg)
	} else {
		if !leg.FillPrice.IsZero() {
			log.Printf("warning: leg for order id %d already has fill price for fill event, but got another fill: %s", orderID, leg)
		}
		leg.FillPrice = fillPrice
		log.Printf("leg filled for order id %d at %s with %s improvement from %s route: %s",
			orderID, fillPrice, priceImprovement, routeName, leg)
	}
	onLegComplete(leg)
}

func onExpiredEvent(event *schwab.OrderEvent, _ *schwab.ExpiredEvent) {
	leg := gLegsByOrderID[event.SchwabOrderID]
	if leg == nil {
		log.Printf("warning: order id %d not found for expired event", event.SchwabOrderID)
		return
	}
	if leg.Complete() && !leg.Closing {
		log.Printf("warning: leg for order id %d is already complete for expired event: %s", event.SchwabOrderID, leg)
		return
	}
	log.Printf("leg expired for order id %d: %s", event.SchwabOrderID, leg)
	leg.Canceled = true
	leg.Canceling = false
	leg.Closing = false
	onLegComplete(leg)
}

func onLegOrderID(leg *Leg, orderID schwab.OrderID) {
	leg.OrderID = orderID
	gLegsByOrderID[orderID] = leg
	gPendingLegs.Remove(leg)
	if leg.Canceling {
		log.Printf("note: defered canceling leg for order id %d: %s", orderID, leg)
		leg.Canceling = false
		leg.Cancel()
	}
}

func onLegComplete(leg *Leg) {
	gUnfilledBulls.Remove(leg)
	gUnfilledBears.Remove(leg)
	if leg.Box.Complete() {
		onBoxComplete(leg.Box)
	}
}

func onBoxComplete(box *Box) {
	if box.Filled() {
		log.Printf("box filled with %s profit: %s", box.FillProfit(), box)
	} else {
		closedLegs := 0
		canceledLegs := 0
		if box.BuyCall.Closed() {
			closedLegs++
		} else {
			canceledLegs++
		}
		if box.SellCall.Closed() {
			closedLegs++
		} else {
			canceledLegs++
		}
		if box.SellPut.Closed() {
			closedLegs++
		} else {
			canceledLegs++
		}
		if box.BuyPut.Closed() {
			closedLegs++
		} else {
			canceledLegs++
		}
		log.Printf("box aborted with with %s profit (versus %s planned) with %d closed legs and %d canceled legs: %s",
			box.ClosingProfit(), box.FillProfit(), closedLegs, canceledLegs, box)
	}
	gPendingBoxes.Remove(box)
}
