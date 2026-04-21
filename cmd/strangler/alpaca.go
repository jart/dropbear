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

	"github.com/google/uuid"
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
		case update := <-t.OrderEventsAlpaca:
			t.onOrderEventAlpaca(update)
			t.Web.broadcastState()
			continue
		case req := <-t.Web.WebRequests:
			t.Web.processWebRequest(req)
			continue
		case <-heartbeat.C:
			t.onHeartbeat()
			t.Web.broadcastState()
			continue
		case order := <-t.FailedOrders:
			t.onOrderFail(order)
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
		case update := <-t.OrderEventsAlpaca:
			t.onOrderEventAlpaca(update)
			t.Web.broadcastState()
		case req := <-t.Web.WebRequests:
			t.Web.processWebRequest(req)
		case <-heartbeat.C:
			t.onHeartbeat()
			t.Web.broadcastState()
		case order := <-t.FailedOrders:
			t.onOrderFail(order)
		case <-readySteadyGo.C:
			if !ready && t.Chain.LastPopulate != 0 && clocky.Now().After(t.Chain.LastPopulate.Add(clocky.Second)) {
				t.restorePortfolioAlpaca()
				t.onDefEnd()
				t.Web.broadcastState()
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
	order.ClientOrderID = uuid.New().String()
	t.OrdersByAlpacaID[order.ClientOrderID] = order
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
		_, err := gAlpacaClient.CreateOrder(&alpaca.OrderRequest{
			Symbol:        osi.Uncanonicalize(order.Security.Name()),
			Side:          sid,
			Qty:           qty,
			Type:          orderType,
			LimitPrice:    order.Price,
			TimeInForce:   alpaca.TimeInForceDay,
			ClientOrderID: order.ClientOrderID,
			AdvancedInstructions: &alpaca.AdvancedInstructions{
				Destination: t.Config.DMA,
			},
		})
		if err != nil {
			log.Printf("failed to send order #%d to alpaca: %v", order.ID, err)
			t.FailedOrders <- order
		}
	}()
}

func (t *Trader) onOrderEventAlpaca(orderUpdate *alpaca.OrderUpdate) {
	order := t.OrdersByAlpacaID[orderUpdate.Order.ClientOrderID]
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
		t.Holdings.Add(order.Security, orderUpdate.Qty.Mul(decimal.Decimal(orderUpdate.Order.Side)), orderUpdate.Price)
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
	if orderUpdate.Order.Status.IsFinal() {
		delete(t.OrdersByAlpacaID, order.ClientOrderID)
		t.removePendingOrder(order)
	}
}
