package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/netty"
	"encoding/json"
	"log"
	"time"
)

type OrderUpdate struct {
	Event       OrderEvent      `json:"event"`
	ExecutionID string          `json:"execution_id"`
	PositionQty decimal.Decimal `json:"position_qty"`
	Price       decimal.Decimal `json:"price"`
	Qty         decimal.Decimal `json:"qty"`
	Timestamp   clocky.Time     `json:"timestamp"`
	Order       Order           `json:"order"`
}

func OrderUpdates() <-chan OrderUpdate {
	c := make(chan OrderUpdate, 64)
	d := &orderUpdatesDaemon{c}
	go d.run()
	return c
}

type orderUpdatesDaemon struct {
	c chan<- OrderUpdate
}

func (d *orderUpdatesDaemon) run() {
	try := 0
	for {
		ts1 := time.Now()
		err := d.impl()
		ts2 := time.Now()
		if err != nil {
			log.Printf("alpaca trade ws: %v, reconnecting...", err)
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

func (d *orderUpdatesDaemon) impl() error {

	// open websocket
	conn, _, err := netty.FastWSDial(TradingWSURL(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// authenticate
	key, secret := GetKey()
	auth := map[string]any{
		"action": "auth",
		"key":    key,
		"secret": secret,
	}
	if err := conn.WriteJSON(auth); err != nil {
		return err
	}

	// nead auth response
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	log.Printf("alpaca trade ws: auth response: %s", string(msg))

	// Subscribe to trade updates
	subscribe := map[string]any{
		"action": "listen",
		"data": map[string]any{
			"streams": []string{
				"trade_updates",
			},
		},
	}
	if err := conn.WriteJSON(subscribe); err != nil {
		return err
	}

	log.Printf("alpaca trade ws: subscribed to trade_updates")

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		// https://docs.alpaca.markets/docs/websocket-streaming
		var msg struct {
			Stream string      `json:"stream"`
			Data   OrderUpdate `json:"data"`
		}
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("alpaca trade ws: error parsing message: %v", err)
			continue
		}
		if msg.Stream != "trade_updates" {
			continue
		}
		orderUpdate := msg.Data
		d.c <- orderUpdate
	}
}
