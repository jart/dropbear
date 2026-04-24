package main

import (
	"dropbear/broker/alpaca"
	"dropbear/broker/databento"
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/options"
	"dropbear/osi"
	"log"
	"os"
)

func (t *Trader) LiveAlpaca() {

	// subscribe to databento data
	key := databento.MustLoadDefaultKey()
	equityDefs := make(chan *options.Equity, 256)
	optionDefs := make(chan *options.Option, 256)
	equityTicks := make(chan *databento.MBP1, 256)
	optionTicks := make(chan *databento.CMBP1, 256)
	go t.streamEquities(key, equityDefs, equityTicks)
	go t.streamOptions(key, optionDefs, optionTicks)

	heartbeat := clocky.NewTicker(*heartbeatFlag)
	defer heartbeat.Stop()
	dumpTimer := clocky.NewTicker(100 * clocky.Millisecond)
	defer dumpTimer.Stop()

	// we must wait for options chain to become available
	readySteadyGo := clocky.NewTicker(clocky.Second)
	defer readySteadyGo.Stop()
	var tsEquity, tsOption clocky.Time
	lastEquityTick := false
	lastOptionTick := false
	ready := false

	for {
		// drain all pending events
		select {
		case o := <-optionDefs:
			t.onOptionDef(o)
			continue
		case m := <-optionTicks:
			t.onOptionTick(m)
			lastOptionTick = m.Flags&databento.FlagSetLast != 0
			tsOption = m.TSRecv
			continue
		case m := <-equityTicks:
			t.onEquityTick(m)
			lastEquityTick = m.Flags&databento.FlagSetLast != 0
			tsEquity = m.TSRecv
			continue
		case update := <-t.OrderEventsAlpaca:
			t.onOrderEventAlpaca(update)
			continue
		case req := <-t.Web.WebRequests:
			t.Web.processWebRequest(req)
			continue
		case <-heartbeat.C:
			t.onHeartbeat()
			continue
		case <-dumpTimer.C:
			t.Web.broadcastState()
			continue
		case order := <-t.FailedOrders:
			t.onOrderFail(order)
			continue
		default:
			// all channels empty
		}
		// let's go
		if !ready {
			t.Hinter.Hint("not trading: waiting for market data")
		} else if lastEquityTick && lastOptionTick {
			now := clocky.Now()
			ageEquity := now.Sub(tsEquity)
			ageOption := now.Sub(tsOption)
			if ageEquity > clocky.Second {
				t.Hinter.Hint("not trading: last equity tick %s old", ageEquity.Round(clocky.Millisecond))
			} else if ageOption > clocky.Second {
				t.Hinter.Hint("not trading: last option tick %s old", ageOption.Round(clocky.Millisecond))
			} else {
				t.onThought(now)
			}
		}
		// block until next event
		select {
		case e := <-equityDefs:
			t.onEquityDef(e)
		case o := <-optionDefs:
			t.onOptionDef(o)
		case m := <-equityTicks:
			t.onEquityTick(m)
			lastEquityTick = m.Flags&databento.FlagSetLast != 0
			tsEquity = m.TSRecv
		case m := <-optionTicks:
			t.onOptionTick(m)
			lastOptionTick = m.Flags&databento.FlagSetLast != 0
			tsOption = m.TSRecv
		case update := <-t.OrderEventsAlpaca:
			t.onOrderEventAlpaca(update)
		case req := <-t.Web.WebRequests:
			t.Web.processWebRequest(req)
		case <-heartbeat.C:
			t.onHeartbeat()
		case <-dumpTimer.C:
			t.Web.broadcastState()
		case order := <-t.FailedOrders:
			t.onOrderFail(order)
		case <-readySteadyGo.C:
			if !ready && t.Chain.LastPopulate != 0 && clocky.Now().After(t.Chain.LastPopulate.Add(clocky.Second)) {
				t.restorePortfolioAlpaca()
				t.onDefEnd()
				ready = true
			}
		}
	}
}

func (t *Trader) cancelOrderAlpaca(order *Order) {
	err := gAlpacaClient.CancelOrder(order.OrderIDAlpaca)
	if err != nil {
		log.Printf("warning: #%d failed to cancel order id %s: %v", order.ID, order.OrderIDAlpaca, err)
	}
}

func (t *Trader) restorePortfolioAlpaca() {
	positions, err := gAlpacaClient.GetPositions()
	if err != nil {
		log.Printf("failed to fetch alpaca account: %v", err)
		os.Exit(1)
	}
	for _, position := range positions {
		symbol := osi.Canonicalize(position.Symbol)
		if security := t.SecuritiesByName[symbol]; security != nil {
			t.Holdings.Restore(security, position.Qty, position.AvgEntryPrice)
		}
	}
}

func (t *Trader) sendLiveOrderAlpaca(order *Order) {
	clientOrderID := order.generateClientOrderID()
	t.OrdersByClientOrderID[clientOrderID] = order
	t.addPendingOrder(order)
	go func() {
		sid := ds.SideBuy
		qty := order.Quantity
		if qty.IsNegative() {
			sid = ds.SideSell
			qty = qty.Neg()
		}
		orderType := alpaca.OrderTypeLimit
		if order.Price.IsZero() {
			orderType = alpaca.OrderTypeMarket
		}
		var advanced *alpaca.AdvancedInstructions
		if t.Config.DMA != alpaca.OrderDestinationNone {
			advanced = &alpaca.AdvancedInstructions{
				Algorithm:   alpaca.OrderAlgorithmDMA,
				Destination: t.Config.DMA,
			}
		}
		_, err := gAlpacaClient.CreateOrder(&alpaca.CreateOrderRequest{
			Symbol:               osi.Uncanonicalize(order.Security.Name()),
			Side:                 sid,
			Qty:                  qty,
			Type:                 orderType,
			LimitPrice:           order.Price,
			TimeInForce:          alpaca.TimeInForceDay,
			ClientOrderID:        clientOrderID,
			AdvancedInstructions: advanced,
		})
		if err != nil {
			log.Printf("failed to send order #%d to alpaca: %v", order.ID, err)
			t.FailedOrders <- order
		}
	}()
}

func (t *Trader) updateOrderAlpaca(order *Order) {
	clientOrderID := order.generateClientOrderID()
	t.OrdersByClientOrderID[clientOrderID] = order
	go func() {
		_, err := gAlpacaClient.ReplaceOrder(order.OrderIDAlpaca, &alpaca.ReplaceOrderRequest{
			LimitPrice:    order.Price,
			ClientOrderID: clientOrderID,
		})
		if err != nil {
			log.Printf("failed to update price #%d on alpaca: %v", order.ID, err)
			return
		}
	}()
}

func (t *Trader) onOrderEventAlpaca(orderUpdate *alpaca.OrderUpdate) {
	order := t.OrdersByClientOrderID[orderUpdate.Order.ClientOrderID]
	if order == nil {
		return
	}
	order.OrderIDAlpaca = orderUpdate.Order.ID
	log.Printf("order #%d update for %s: %s price=%s status=%s qty=%s pos=%s filled=%s/%s avg_price=%s id=%s",
		order.ID, orderUpdate.Order.Symbol,
		orderUpdate.Event, orderUpdate.Price, orderUpdate.Order.Status, orderUpdate.Qty, orderUpdate.PositionQty,
		orderUpdate.Order.FilledQty, orderUpdate.Order.Qty, orderUpdate.Order.FilledAvgPrice, orderUpdate.Order.ID)
	if !orderUpdate.Qty.IsZero() {
		// this order update represents a fill
		absoluteOrderQuantity := order.Quantity.Abs()
		absoluteFilledQuantity := orderUpdate.Order.FilledQty
		order.Unfilled = absoluteOrderQuantity.Sub(absoluteFilledQuantity)
		fillQty := orderUpdate.Qty.Mul(decimal.Decimal(orderUpdate.Order.Side))
		pnl := t.Holdings.Add(order.Security, fillQty, orderUpdate.Price)
		fee := order.EstimateFee(orderUpdate.Qty, order.Unfilled.Cmp(order.Quantity.Abs()) == 0, !order.Making)
		t.Holdings.TotalFees = t.Holdings.TotalFees.Add(fee)
		t.recordFill(clocky.Now(), order.Security, fillQty, order.Price, orderUpdate.Price, pnl, fee)
		holding := t.Holdings.Positions[order.Security]
		if holding != nil {
			if holding.Quantity.Cmp(orderUpdate.PositionQty) != 0 {
				log.Fatalf("position quantity mismatch for %s: we think it's %s but alpaca says it's %s",
					orderUpdate.Order.Symbol, holding.Quantity, orderUpdate.PositionQty)
			}
			ourAvgPrice := holding.AverageCost
			alpacaAvgPrice := orderUpdate.Order.FilledAvgPrice
			if ourAvgPrice.Sub(alpacaAvgPrice).Abs().Cmp(decimal.Cent) > 0 {
				log.Printf("warning: average price mismatch for %s: we think it's %s but alpaca says it's %s",
					orderUpdate.Order.Symbol, ourAvgPrice, alpacaAvgPrice)
			}
		}
	}
	switch orderUpdate.Order.Status {
	case alpaca.OrderStatusReplaced:
		order.OrderIDAlpaca = orderUpdate.Order.ReplacedBy
	case alpaca.OrderStatusFilled, alpaca.OrderStatusCanceled, alpaca.OrderStatusExpired, alpaca.OrderStatusRejected:
		t.removePendingOrder(order)
	}
}
