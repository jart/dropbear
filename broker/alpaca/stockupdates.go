package alpaca

import (
	"dropbear/broker/alpaca/sip"
	"dropbear/clocky"
	"dropbear/netty"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	SIPWSURL        = "wss://stream.data.alpaca.markets/v2/sip"
	IEXWSURL        = "wss://stream.data.alpaca.markets/v2/iex"
	BOATSWSURL      = "wss://stream.data.alpaca.markets/v1beta1/boats"
	OvernightSWSURL = "wss://stream.data.alpaca.markets/v1beta1/overnight" // 15 minute delay boats
	DelayedSIPWSURL = "wss://stream.data.alpaca.markets/v2/delayed_sip"    // 15 minute delay sip
	CryptoWSURL     = "wss://stream.data.alpaca.markets/v1beta3/crypto/us"
)

type StockUpdate struct {
	ReceivedAt clocky.Time
	Message    *sip.Message
}

type StockUpdatesRequest struct {
	Action      string   `json:"action"` // set to "subscribe"
	Trades      []string `json:"trades,omitempty"`
	Quotes      []string `json:"quotes,omitempty"`   // e.g. ["AAPL", "TSLA"]
	Statuses    []string `json:"statuses,omitempty"` // e.g. ["*"]
	LULDs       []string `json:"lulds,omitempty"`
	Imbalances  []string `json:"imbalances,omitempty"` // e.g. ["INAQU"]
	Bars        []string `json:"bars,omitempty"`       // minute bars
	DailyBars   []string `json:"dailyBars,omitempty"`
	UpdatedBars []string `json:"updatedBars,omitempty"`
}

// MustStockUpdates connects to Alpaca's real-time stock market data websocket or dies.
func MustStockUpdates(wsurl string, req *StockUpdatesRequest) <-chan StockUpdate {
	c, err := StockUpdates(wsurl, req)
	if err != nil {
		panic(fmt.Sprintf("error subscribing to stock updates: %v", err))
	}
	return c
}

// StockUpdates connects to Alpaca's real-time stock market data websocket.
func StockUpdates(wsurl string, req *StockUpdatesRequest) (<-chan StockUpdate, error) {
	c := make(chan StockUpdate, 64)
	d := &stockUpdatesDaemon{c: c, req: req, wsurl: wsurl}
	err := d.connect()
	if err != nil {
		return nil, err
	}
	go d.run()
	return c, nil
}

type stockUpdatesDaemon struct {
	c     chan<- StockUpdate
	req   *StockUpdatesRequest
	conn  *netty.FastWSConn
	wsurl string
}

type stockUpdatesResponse struct {
	Type string `json:"T"`
	Code int    `json:"code,omitempty"`
	Msg  string `json:"msg,omitempty"`
}

func (d *stockUpdatesDaemon) connect() error {

	// open websocket
	var err error
	d.conn, _, err = netty.FastWSDial(d.wsurl, nil)
	if err != nil {
		return err
	}
	connectResponse, err := d.readControlMessage()
	if err != nil {
		d.conn.Close()
		return err
	}
	if connectResponse.Type != "success" || connectResponse.Msg != "connected" {
		d.conn.Close()
		return fmt.Errorf("connection failed: %v", connectResponse)
	}

	// authenticate
	key := GetKey()
	auth := map[string]any{
		"action": "auth",
		"key":    key.Key,
		"secret": key.Secret,
	}
	if err := d.conn.WriteJSON(auth); err != nil {
		d.conn.Close()
		return err
	}
	authResponse, err := d.readControlMessage()
	if err != nil {
		d.conn.Close()
		return err
	}
	if authResponse.Type != "success" || authResponse.Msg != "authenticated" {
		d.conn.Close()
		return fmt.Errorf("authentication failed: %v", authResponse)
	}

	// subscribe to data
	if err := d.conn.WriteJSON(d.req); err != nil {
		d.conn.Close()
		return err
	}
	subscribeResponse, err := d.readControlMessage()
	if err != nil {
		d.conn.Close()
		return err
	}
	if subscribeResponse.Type != "subscription" {
		d.conn.Close()
		return fmt.Errorf("failed to subscribe: %v", subscribeResponse)
	}

	return nil
}

func (d *stockUpdatesDaemon) run() {
	try := 0
	for {
		ts1 := time.Now()
		err := d.impl()
		ts2 := time.Now()
		if err != nil {
			logf("error reading message: %v\n", err)
		}
		elapsed := ts2.Sub(ts1)
		if elapsed > 30*time.Second {
			try = 0 // connection was healthy so reset backoff
		}
		for {
			wait := time.Duration(15<<min(try, 11)) * time.Millisecond
			time.Sleep(wait) // waits for 30 seconds max
			try++
			err = d.connect()
			if err == nil {
				break
			}
			logf("error reconnecting: %v\n", err)
		}
	}
}

func (d *stockUpdatesDaemon) impl() error {
	defer d.conn.Close()
	for {
		_, bytes, err := d.conn.ReadMessage()
		if err != nil {
			return err
		}
		if len(bytes) < 2 || bytes[0] != '[' {
			logf("unexpected message: %s\n", string(bytes))
			continue
		}
		i := 1
		n := len(bytes)
		t := clocky.Now()
		for i < n {
			skip := false
			msg := new(sip.Message)
			j, err := msg.Parse(bytes[i:])
			if err != nil {
				if err == sip.ErrShortOverflow {
					skip = true
				} else {
					logf("%v: %s\n", err, string(bytes))
					break
				}
			}
			i += j
			if !skip {
				d.c <- StockUpdate{
					ReceivedAt: t,
					Message:    msg,
				}
			}
			if i < n && bytes[i] == ',' {
				i++
			} else if i < n && bytes[i] == ']' {
				break
			} else {
				logf("broken message: %s\n", string(bytes))
				break
			}
		}
	}
}

func (d *stockUpdatesDaemon) readControlMessage() (*stockUpdatesResponse, error) {
	messageType, message, err := d.conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	// log message with timestamp
	flog := getLog()
	if flog != nil {
		var sb strings.Builder
		sb.Grow(128 + len(message))
		sb.WriteString(clocky.Now().String())
		sb.WriteString(" got websocket control message type ")
		sb.WriteString(strconv.Itoa(messageType))
		sb.WriteString(": ")
		sb.Write(message)
		sb.WriteRune('\n')
		flog.Write([]byte(sb.String()))
	}
	var response []*stockUpdatesResponse
	if err := json.Unmarshal(message, &response); err != nil {
		return nil, err
	}
	if len(response) == 0 {
		return nil, fmt.Errorf("empty response")
	}
	if len(response) > 1 {
		return nil, fmt.Errorf("unexpected multiple responses: %v", response)
	}
	return response[0], nil
}
