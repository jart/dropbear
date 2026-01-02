package binance

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"encoding/json"
	"log"
	"strings"
	"time"
)

const StreamURL = "wss://nickel.justinestreet.capital/ws"
const PublicStreamURL = "wss://nickel.justinestreet.capital/pub/ws"

type marketData struct {
	Symbol       string
	client       *Client
	channel      chan<- *ds.Tick
	LastUpdateID int64
}

type marketTrade struct {
	Symbol     string `json:"s"`
	Price      string `json:"p"`
	Quantity   string `json:"q"`
	IsSell     bool   `json:"m"` // true = buyer is maker = taker sold
	Time       int64  `json:"T"` // unix milliseconds
	TradeID    int64  `json:"t"`
	IgnoreBigM bool   `json:"M"` // prevents case-insensitive collision with m
}

type depthUpdate struct {
	Symbol        string      `json:"s"`
	Bids          [][2]string `json:"b"`
	Asks          [][2]string `json:"a"`
	FirstUpdateID int64       `json:"U"`
	FinalUpdateID int64       `json:"u"`
}

func MarketData(symbol string, client *Client) <-chan *ds.Tick {
	channel := make(chan *ds.Tick, 64)
	marketData := &marketData{
		Symbol:  symbol,
		client:  client,
		channel: channel,
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
			log.Printf("binance[%s]: market data error: %v", m.Symbol, err)
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

	// connect websocket
	conn, _, err := ds.FastWSDial(StreamURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// subscribe to depth and trade streams
	err = conn.WriteJSON(map[string]any{
		"method": "SUBSCRIBE",
		"params": []string{
			strings.ToLower(m.Symbol) + "@depth",
			strings.ToLower(m.Symbol) + "@trade",
		},
		"id": 1,
	})
	if err != nil {
		return err
	}
	log.Printf("binance[%s]: market data stream connected", m.Symbol)

	// now fetch initial depth snapshot
	err = m.snapshot()
	if err != nil {
		return err
	}

	// handle websocket messages
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		err = m.handleMessage(msg)
		if err != nil {
			log.Printf("binance[%s]: market data message handling error: %v", m.Symbol, err)
		}
	}
}

func (m *marketData) snapshot() error {
	depthSnapshot, err := m.client.GetDepth(m.Symbol, 500)
	if err != nil {
		return err
	}
	tick := &ds.Tick{
		Time: clocky.Now(),
		Snap: true,
	}
	for _, bid := range depthSnapshot.Bids {
		price := decimal.Parse(bid[0])
		size := decimal.Parse(bid[1])
		tick.Bids = append(tick.Bids, ds.Level{
			Price: price,
			Size:  size,
		})
	}
	for _, ask := range depthSnapshot.Asks {
		price := decimal.Parse(ask[0])
		size := decimal.Parse(ask[1])
		tick.Asks = append(tick.Asks, ds.Level{
			Price: price,
			Size:  size,
		})
	}
	m.LastUpdateID = depthSnapshot.LastUpdateID
	m.channel <- tick
	return nil
}

func (m *marketData) handleMessage(data []byte) error {
	now := clocky.Now()
	var wrapper struct {
		Stream    string          `json:"stream"`
		Data      json.RawMessage `json:"data"`
		EventType string          `json:"e"`
		ID        int             `json:"id"`
	}
	// ignore unmarshal errors: fields are optional and "E" (int) conflicts with "e" (string)
	_ = json.Unmarshal(data, &wrapper)
	stream := wrapper.Stream
	if stream == "" {
		stream = wrapper.EventType
	}
	msgData := wrapper.Data
	if msgData == nil {
		msgData = data
	}
	switch {
	case stream == "trade" || strings.HasSuffix(stream, "@trade"):
		return m.handleTrade(msgData, now)
	case stream == "depthUpdate" || strings.Contains(stream, "@depth"):
		return m.handleDepthUpdate(msgData, now)
	case stream == "":
		// subscription confirmations, pings, etc.
		return nil
	default:
		log.Printf("binance[%s]: ignoring unknown stream type %q: %s", m.Symbol, stream, data)
		return nil
	}
}

func (m *marketData) handleTrade(data []byte, now clocky.Time) error {
	var raw marketTrade
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}
	if raw.Price == "" || raw.Quantity == "" {
		log.Printf("binance[%s]: ignoring trade with empty price/quantity: %s", m.Symbol, data)
		return nil
	}
	tick := &ds.Tick{Time: now}
	side := ds.SideBuy
	if raw.IsSell {
		side = ds.SideSell
	}
	tick.Trades = append(tick.Trades, ds.Trade{
		Price:    decimal.Parse(raw.Price),
		Quantity: decimal.Parse(raw.Quantity),
		Time:     clocky.Time(raw.Time * 1000),
		Side:     side,
	})
	m.channel <- tick
	return nil
}

func (m *marketData) handleDepthUpdate(data []byte, now clocky.Time) error {
	var depthUpdate depthUpdate
	err := json.Unmarshal(data, &depthUpdate)
	if err != nil {
		return err
	}
	if depthUpdate.FinalUpdateID > m.LastUpdateID {
		tick := &ds.Tick{Time: now}
		for _, bid := range depthUpdate.Bids {
			price := decimal.Parse(bid[0])
			size := decimal.Parse(bid[1])
			tick.Bids = append(tick.Bids, ds.Level{
				Price: price,
				Size:  size,
			})
		}
		for _, ask := range depthUpdate.Asks {
			price := decimal.Parse(ask[0])
			size := decimal.Parse(ask[1])
			tick.Asks = append(tick.Asks, ds.Level{
				Price: price,
				Size:  size,
			})
		}
		m.channel <- tick
	}
	return nil
}
