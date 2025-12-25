package coinbase

import (
	"dropbear/ds"
	"encoding/json"
	"log"
	"time"
)

type OrderUpdate struct {
	OrderID               string `json:"order_id"`
	ProductID             string `json:"product_id"` // e.g. BTC-USD
	AvgPrice              string `json:"avg_price"`  // e.g. 30000.00
	CancelReason          string `json:"cancel_reason"`
	ClientOrderID         string `json:"client_order_id"` // e.g. f47ac10b-58cc-4372-a567-0e02b2c3d479
	CompletionPercentage  string `json:"completion_percentage"`
	ContractExpiryType    string `json:"contract_expiry_type"` // e.g. UNKNOWN_CONTRACT_EXPIRY_TYPE
	CumulativeQuantity    string `json:"cumulative_quantity"`  // amount of order filled so far, in base currency
	FilledValue           string `json:"filled_value"`         // total notional value of what's been filled (price × quantity for each fill, summed up)
	LeavesQuantity        string `json:"leaves_quantity"`      // amount remaining to be filled (in the same currency as the order was placed)
	LimitPrice            string `json:"limit_price"`
	NumberOfFills         string `json:"number_of_fills"`
	OrderSide             string `json:"order_side"` // e.g. buy, sell (should do case insensitive compare)
	OrderType             string `json:"order_type"` // e.g. limit (should do case insensitive compare)
	OutstandingHoldAmount string `json:"outstanding_hold_amount"`
	PostOnly              string `json:"post_only"`    // e.g. true, false
	ProductType           string `json:"product_type"` // e.g. SPOT (should do case insensitive compare)
	RetailPortfolioID     string `json:"retail_portfolio_id"`
	RiskManagedBy         string `json:"risk_managed_by"` // e.g. UNKNOWN_RISK_MANAGEMENT_TYPE
	Status                string `json:"status"`          // e.g. NEW, PENDING, OPEN, FILLED, CANCEL_QUEUED, CANCELLED, REJECTED, FAILED (should do case insensitive compare)
	StopPrice             string `json:"stop_price"`
	TimeInForce           string `json:"time_in_force"` // e.g. GOOD_UNTIL_CANCELLED
	TotalFees             string `json:"total_fees"`
	TotalValueAfterFees   string `json:"total_value_after_fees"`
	TriggerStatus         string `json:"trigger_status"` // e.g. INVALID_ORDER_TYPE
	CreationTime          string `json:"creation_time"`  // e.g. 2025-11-23T23:04:34.830098Z
	EndTime               string `json:"end_time"`       // e.g. 2025-11-23T23:04:34.830098Z
	StartTime             string `json:"start_time"`     // e.g. 2025-11-23T23:04:34.830098Z
}

// OrderUpdates connects to Coinbase websocket and returns a channel of order updates.
func OrderUpdates() <-chan *OrderUpdate {
	c := make(chan *OrderUpdate, 64)
	d := &orderUpdatesDaemon{c}
	go d.run()
	return c
}

type orderUpdatesDaemon struct {
	c chan<- *OrderUpdate
}

// Run connects to the Coinbase websocket and dispatches order updates to subscribers.
// This blocks forever, reconnecting on errors.
func (w *orderUpdatesDaemon) run() {
	try := 0
	for {
		ts1 := time.Now()
		err := w.impl()
		ts2 := time.Now()
		if err != nil {
			log.Printf("coinbase trade ws: %v, reconnecting...", err)
		}
		elapsed := ts2.Sub(ts1)
		if elapsed > 30*time.Second {
			try = 0 // connection was healthy so reset backoff
		}
		wait := time.Duration(15<<min(try, 11)) * time.Millisecond
		time.Sleep(wait) // waits for 30 seconds max
		try++
	}
}

func (w *orderUpdatesDaemon) impl() error {
	conn, _, err := ds.FastWSDial(WSUserURL(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.WriteJSON(map[string]any{
		"type":    "subscribe",
		"channel": "heartbeats",
	})
	if err != nil {
		return err
	}
	sendSubscribe := func() error {
		subscribe := map[string]any{
			"type":    "subscribe",
			"channel": "user",
			"jwt":     GenerateWSJWT(),
		}
		return conn.WriteJSON(subscribe)
	}
	if err := sendSubscribe(); err != nil {
		return err
	}
	log.Printf("coinbase trade ws: subscribed to user channel")
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(90 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := sendSubscribe(); err != nil {
					log.Printf("coinbase trade ws: error refreshing subscription: %v", err)
					return
				}
			case <-done:
				return
			}
		}
	}()
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var msg struct {
			Channel     string `json:"channel"`
			Timestamp   string `json:"timestamp"`
			SequenceNum int    `json:"sequence_num"`
			Events      []struct {
				Type   string         `json:"type"`
				Orders []*OrderUpdate `json:"orders"`
			} `json:"events"`
		}
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("coinbase trade ws: error parsing message: %v", err)
			continue
		}
		if msg.Channel != "user" {
			continue
		}
		for _, event := range msg.Events {
			for _, order := range event.Orders {
				w.c <- order
			}
		}
	}
}
