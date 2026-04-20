package alpaca

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/netty"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	alpacaLogFlag = flag.String("alpaca-log", os.ExpandEnv("$HOME/.alpaca.log"), "log file for alpaca websocket messages")
)

type OrderUpdate struct {
	Event       OrderEvent      `json:"event"`
	EventtID    string          `json:"event_id"`     // e.g. 01KPNZVTK5HHCV1RZSC3HTTDTQ
	ExecutionID string          `json:"execution_id"` // e.g. 78e5a6fc-bdd2-4133-a043-1cfaa7121518 (empty for some events like pending_new)
	PositionQty decimal.Decimal `json:"position_qty"` // quantity of the position after the order update (negative if short)
	Price       decimal.Decimal `json:"price"`
	Qty         decimal.Decimal `json:"qty"` // always positive, even for sales
	Timestamp   clocky.Time     `json:"timestamp"`
	At          clocky.Time     `json:"at"` // this usually comes slightly after timestamp
	Order       Order           `json:"order"`
}

func OrderUpdates() <-chan *OrderUpdate {
	flog, err := os.OpenFile(*alpacaLogFlag, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("could not open alpaca log file: %v", err)
	}
	c := make(chan *OrderUpdate, 64)
	d := &orderUpdatesDaemon{c: c, flog: flog}
	go d.run()
	return c
}

type orderUpdatesDaemon struct {
	c    chan<- *OrderUpdate
	flog *os.File
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
	key := GetKey()
	auth := map[string]any{
		"action": "auth",
		"key":    key.Key,
		"secret": key.Secret,
	}
	if err := conn.WriteJSON(auth); err != nil {
		return err
	}

	// need auth response
	_, authResponseBytes, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	var authResponse struct {
		Stream string `json:"stream"`
		Data   struct {
			Action string `json:"action"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(authResponseBytes, &authResponse); err != nil {
		return err
	}
	if authResponse.Stream != "authorization" || authResponse.Data.Action != "authenticate" || authResponse.Data.Status != "authorized" {
		return fmt.Errorf("authentication failed: %s", string(authResponseBytes))
	}

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

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		// log message with timestamp
		var sb strings.Builder
		sb.Grow(128 + len(message))
		sb.WriteString(clocky.Now().String())
		sb.WriteString(" got message type ")
		sb.WriteString(strconv.Itoa(messageType))
		sb.WriteString(": ")
		sb.Write(message)
		sb.WriteRune('\n')
		d.flog.Write([]byte(sb.String()))

		// https://docs.alpaca.markets/docs/websocket-streaming
		var msg struct {
			Stream string       `json:"stream"`
			Data   *OrderUpdate `json:"data"`
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
