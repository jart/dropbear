package alpaca

import (
	"dropbear/broker/alpaca/sip"
	"dropbear/netty"
	"encoding/json"
	"fmt"
	"log"
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

type StockUpdatesRequest struct {
	Action   string   `json:"action"` // set to "subscribe"
	Trades   []string `json:"trades,omitempty"`
	Quotes   []string `json:"quotes,omitempty"`   // e.g. ["AAPL", "TSLA"]
	Statuses []string `json:"statuses,omitempty"` // e.g. ["*"]
	LULDs    []string `json:"lulds,omitempty"`
}

// StockUpdates connects to Alpaca's real-time stock market data websocket.
func StockUpdates(wsurl string, req *StockUpdatesRequest) (<-chan *sip.Message, error) {
	c := make(chan *sip.Message, 64)
	d := &stockUpdatesDaemon{c: c, req: req, wsurl: wsurl}
	err := d.connect()
	if err != nil {
		return nil, err
	}
	go d.run()
	return c, nil
}

type stockUpdatesDaemon struct {
	c     chan<- *sip.Message
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
	defer d.conn.Close()
	connectResponse, err := d.readControlMessage()
	if err != nil {
		return err
	}
	if connectResponse.Type != "success" || connectResponse.Msg != "connected" {
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
		return err
	}
	authResponse, err := d.readControlMessage()
	if err != nil {
		return err
	}
	if authResponse.Type != "success" || authResponse.Msg != "authenticated" {
		return fmt.Errorf("authentication failed: %v", authResponse)
	}

	// subscribe to data
	if err := d.conn.WriteJSON(d.req); err != nil {
		return err
	}
	subscribeResponse, err := d.readControlMessage()
	if err != nil {
		return err
	}
	if subscribeResponse.Type != "subscription" {
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
	for {
		_, bytes, err := d.conn.ReadMessage()
		if err != nil {
			return err
		}
		if len(bytes) < 2 || bytes[0] != '[' {
			log.Printf("unexpected message: %s", string(bytes))
			continue
		}
		i := 1
		n := len(bytes)
		for i < n {
			msg := new(sip.Message)
			j, err := msg.Parse(bytes[i:])
			if err != nil {
				log.Printf("%v: %s", err, string(bytes))
				break
			}
			i += j
			d.c <- msg
			if i < n && bytes[i] == ',' {
				i++
			} else if i < n && bytes[i] == ']' {
				break
			} else {
				log.Printf("broken message: %s", string(bytes))
				break
			}
		}
	}
}

func (d *stockUpdatesDaemon) readControlMessage() (*stockUpdatesResponse, error) {
	_, bytes, err := d.conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	var response []*stockUpdatesResponse
	if err := json.Unmarshal(bytes, &response); err != nil {
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
