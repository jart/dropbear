package kraken

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"dropbear/ds"
	"encoding/json"
	"log"
	"time"
)

const wsURL = "wss://ws.kraken.com/v2"

type marketData struct {
	Symbol  string
	client  *Client
	channel chan<- *ds.Tick
}

// MarketData creates a channel that streams market data ticks for the given symbol.
// Symbol format is "BTC/USD" (Kraken uses slash separator).
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
			log.Printf("kraken[%s]: market data error: %v", m.Symbol, err)
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
	conn, _, err := ds.FastWSDial(wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// subscribe to order book (level 2)
	if err := conn.WriteJSON(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":  "book",
			"symbol":   []string{m.Symbol},
			"depth":    100,
			"snapshot": true,
		},
	}); err != nil {
		return err
	}

	// subscribe to trades
	if err := conn.WriteJSON(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel":  "trade",
			"symbol":   []string{m.Symbol},
			"snapshot": false,
		},
	}); err != nil {
		return err
	}

	log.Printf("kraken[%s]: market data stream connected", m.Symbol)

	// handle messages
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		m.handleMessage(message)
	}
}

type wsMessage struct {
	Channel string          `json:"channel"`
	Type    string          `json:"type"`
	Data    json.RawMessage `json:"data"`
}

type bookData struct {
	Symbol   string      `json:"symbol"`
	Bids     []bookLevel `json:"bids"`
	Asks     []bookLevel `json:"asks"`
	Checksum uint32      `json:"checksum"`
}

type bookLevel struct {
	Price string `json:"price"`
	Qty   string `json:"qty"`
}

type tradeData struct {
	Symbol    string `json:"symbol"`
	Side      string `json:"side"`
	Price     string `json:"price"`
	Qty       string `json:"qty"`
	OrdType   string `json:"ord_type"`
	TradeID   int64  `json:"trade_id"`
	Timestamp string `json:"timestamp"`
}

func (m *marketData) handleMessage(data []byte) {
	now := clocky.Now()
	var msg wsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("kraken[%s]: market data unmarshal error: %v", m.Symbol, err)
		return
	}

	switch msg.Channel {
	case "book":
		m.handleBook(msg.Data, msg.Type, now)
	case "trade":
		m.handleTrades(msg.Data, now)
	case "heartbeat":
		// ignore
	case "status":
		// connection status messages
	default:
		// subscription confirmations and other messages
		if msg.Channel != "" {
			log.Printf("kraken[%s]: unknown channel: %s", m.Symbol, msg.Channel)
		}
	}
}

func (m *marketData) handleBook(data json.RawMessage, msgType string, now clocky.Time) {
	var books []bookData
	if err := json.Unmarshal(data, &books); err != nil {
		log.Printf("kraken[%s]: book unmarshal error: %v", m.Symbol, err)
		return
	}
	for _, book := range books {
		tick := &ds.Tick{Time: now}
		if msgType == "snapshot" {
			tick.Snap = true
		}
		for _, bid := range book.Bids {
			price := decimal.Parse(bid.Price)
			size := decimal.Parse(bid.Qty)
			tick.Bids = append(tick.Bids, ds.Level{
				Price: price,
				Size:  size,
			})
		}
		for _, ask := range book.Asks {
			price := decimal.Parse(ask.Price)
			size := decimal.Parse(ask.Qty)
			tick.Asks = append(tick.Asks, ds.Level{
				Price: price,
				Size:  size,
			})
		}
		m.channel <- tick
	}
}

func (m *marketData) handleTrades(data json.RawMessage, now clocky.Time) {
	var trades []tradeData
	if err := json.Unmarshal(data, &trades); err != nil {
		log.Printf("kraken[%s]: trade unmarshal error: %v", m.Symbol, err)
		return
	}
	if len(trades) == 0 {
		return
	}
	tick := &ds.Tick{Time: now}
	for _, trade := range trades {
		side := ds.SideBuy
		if trade.Side == "sell" {
			side = ds.SideSell
		}
		tradeTime := now
		if trade.Timestamp != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, trade.Timestamp); err == nil {
				tradeTime = clocky.Time(parsed.UnixMicro())
			}
		}
		tick.Trades = append(tick.Trades, ds.Trade{
			Price:    decimal.Parse(trade.Price),
			Quantity: decimal.Parse(trade.Qty),
			Side:     side,
			Time:     tradeTime,
		})
	}
	m.channel <- tick
}
