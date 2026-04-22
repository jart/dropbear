package alpaca

import (
	"dropbear/broker/alpaca/sip"
	"dropbear/netty"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

type StockUpdatesRequest struct {
	Action   string   `json:"action"` // set to "subscribe"
	Trades   []string `json:"trades,omitempty"`
	Quotes   []string `json:"quotes,omitempty"`   // e.g. ["AAPL", "TSLA"]
	Statuses []string `json:"statuses,omitempty"` // e.g. ["*"]
	LULDs    []string `json:"lulds,omitempty"`
}

// StockUpdates connects to Alpaca's real-time stock market data websocket.
func StockUpdates(wsurl string, req *StockUpdatesRequest) <-chan *sip.Message {
	c := make(chan *sip.Message, 64)
	ready := make(chan struct{})
	d := &stockUpdatesDaemon{c: c, req: req, wsurl: wsurl, ready: ready}
	go d.run()
	<-ready
	return c
}

type stockUpdatesDaemon struct {
	c     chan<- *sip.Message
	req   *StockUpdatesRequest
	wsurl string
	ready chan struct{}
}

type stockUpdatesResponse struct {
	Type string `json:"T"`
	Code int    `json:"code,omitempty"`
	Msg  string `json:"msg,omitempty"`
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
		wait := time.Duration(15<<min(try, 11)) * time.Millisecond
		time.Sleep(wait) // waits for 30 seconds max
		try++
	}
}

func (d *stockUpdatesDaemon) impl() error {

	// open websocket
	conn, _, err := netty.FastWSDial(d.wsurl, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	connectResponse, err := d.readControlMessage(conn)
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
	if err := conn.WriteJSON(auth); err != nil {
		return err
	}
	authResponse, err := d.readControlMessage(conn)
	if err != nil {
		return err
	}
	if authResponse.Type != "success" || authResponse.Msg != "authenticated" {
		return fmt.Errorf("authentication failed: %v", authResponse)
	}

	// subscribe to data
	if err := conn.WriteJSON(d.req); err != nil {
		return err
	}
	subscribeResponse, err := d.readControlMessage(conn)
	if err != nil {
		return err
	}
	if subscribeResponse.Type != "subscription" {
		return fmt.Errorf("failed to subscribe: %v", subscribeResponse)
	}

	// signal that we're connected and subscribed
	select {
	case <-d.ready:
		// already closed from a previous connection
	default:
		close(d.ready)
	}

	for {
		_, bytes, err := conn.ReadMessage()
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

func (d *stockUpdatesDaemon) readControlMessage(conn *netty.FastWSConn) (*stockUpdatesResponse, error) {
	_, bytes, err := conn.ReadMessage()
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
