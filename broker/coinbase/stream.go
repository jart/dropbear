package coinbase

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"dropbear/netty"
	"encoding/json"
	"log"
	"time"
)

type marketData struct {
	ProductID string
	client    *Client
	channel   chan<- ds.Tick
	builder   ds.TickBuilder
	buf       []byte
}

func MarketData(productID string, client *Client) <-chan ds.Tick {
	channel := make(chan ds.Tick, 64)
	marketData := &marketData{
		ProductID: productID,
		client:    client,
		channel:   channel,
	}
	go marketData.run()
	return channel
}

func (m *marketData) run() {
	try := 0
	for {
		ts1 := time.Now()
		err := m.runOnce()
		ts2 := time.Now()
		if err != nil {
			log.Printf("coinbase[%s]: market data error: %v", m.ProductID, err)
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

func (m *marketData) runOnce() error {

	// always use public endpoint for L2/trades: it's same data
	// and the authenticated endpoint may have different format
	url := WSURL()

	// connect to websocket
	conn, _, err := netty.FastWSDial(url, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// subscribe to heartbeats
	if err := conn.WriteJSON(map[string]any{
		"type":    "subscribe",
		"channel": "heartbeats",
	}); err != nil {
		return err
	}

	// subscribe to L2 and trades (jwt optional but may give priority)
	if err := m.sendSubscribe(conn); err != nil {
		return err
	}
	log.Printf("coinbase[%s]: market data stream connected", m.ProductID)

	// refresh jwt periodically in live mode
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(90 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := m.sendSubscribe(conn); err != nil {
					log.Printf("coinbase[%s]: JWT refresh error: %v", m.ProductID, err)
					return
				}
			case <-done:
				return
			}
		}
	}()

	// handle messages
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		m.handleMessage(message)
	}
}

func (m *marketData) sendSubscribe(conn *netty.FastWSConn) error {
	sub := map[string]any{
		"type":        "subscribe",
		"channel":     "level2",
		"product_ids": []string{m.ProductID},
		"jwt":         GenerateWSJWT(),
	}
	if err := conn.WriteJSON(sub); err != nil {
		return err
	}
	sub2 := map[string]any{
		"type":        "subscribe",
		"channel":     "market_trades",
		"product_ids": []string{m.ProductID},
		"jwt":         GenerateWSJWT(),
	}
	return conn.WriteJSON(sub2)
}

func (m *marketData) handleMessage(data []byte) {
	now := clocky.Now()
	var msg Events
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("coinbase[%s]: market data unmarshal error: %v", m.ProductID, err)
		return
	}
	switch msg.Channel {
	case "l2_data":
		m.handleL2(msg.Events, now)
	case "market_trades":
		m.handleTrades(msg.Events, now)
	case "subscriptions":
		// subscription confirmation
	case "heartbeats":
		// ignore
	default:
		log.Printf("coinbase[%s]: unknown channel: type=%s channel=%s", m.ProductID, msg.Type, msg.Channel)
	}
}

func (m *marketData) handleL2(events []Event, now clocky.Time) {
	for _, event := range events {
		m.builder.Reset()
		m.builder.Time = now
		if event.Type == "snapshot" {
			m.builder.Snap = true
		}
		for _, u := range event.Updates {
			price := decimal.Parse(u.PriceLevel)
			size := decimal.Parse(u.NewQuantity)
			if u.Side == "bid" {
				m.builder.Bids = append(m.builder.Bids, ds.Level{
					Price: price,
					Size:  size,
				})
			} else {
				m.builder.Asks = append(m.builder.Asks, ds.Level{
					Price: price,
					Size:  size,
				})
			}
		}
		m.sendTick()
	}
}

func (m *marketData) handleTrades(events []Event, now clocky.Time) {
	for _, event := range events {
		m.builder.Reset()
		m.builder.Time = now
		for _, eventTrade := range event.Trades {
			m.builder.Trades = append(m.builder.Trades, ds.Trade{
				Price:    decimal.Parse(eventTrade.Price),
				Quantity: decimal.Parse(eventTrade.Size),
				Side:     ds.MustParseSide(eventTrade.Side).Flip(),
				Time:     clocky.MustParseTime(eventTrade.Time),
			})
		}
		m.sendTick()
	}
}

func (m *marketData) sendTick() {
	// Serialize builder to bytes and create Tick view
	m.buf = m.builder.Encode(m.buf[:0])
	// Make a copy since the buffer will be reused
	data := make([]byte, len(m.buf))
	copy(data, m.buf)
	m.channel <- ds.NewTick(data)
}
