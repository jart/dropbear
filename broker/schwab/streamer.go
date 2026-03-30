package schwab

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"dropbear/clocky"
	"dropbear/netty"
)

var (
	schwabLogFlag = flag.String("schwab-log", os.ExpandEnv("$HOME/.schwab.log"), "log file for schwab websocket messages")
)

// StreamerInfo contains the WebSocket connection details from GET /userPreference.
type StreamerInfo struct {
	StreamerSocketURL      string `json:"streamerSocketUrl"`
	SchwabClientCustomerID string `json:"schwabClientCustomerId"`
	SchwabClientCorrelID   string `json:"schwabClientCorrelId"`
	SchwabClientChannel    string `json:"schwabClientChannel"`
	SchwabClientFunctionID string `json:"schwabClientFunctionId"`
}

// GetStreamerInfo returns the WebSocket connection details for the Schwab streamer.
// Calls GET /userPreference and extracts the first streamerInfo entry.
func (c *Client) GetStreamerInfo() (*StreamerInfo, error) {
	var pref struct {
		StreamerInfo []StreamerInfo `json:"streamerInfo"`
	}
	err := c.RequestJSON(netty.BulkHttpClient, "GET", "/userPreference", nil, &pref)
	if err != nil {
		return nil, err
	}
	if len(pref.StreamerInfo) == 0 {
		return nil, fmt.Errorf("schwab: no streamer info in userPreference response")
	}
	return &pref.StreamerInfo[0], nil
}

// OrderUpdates returns a channel that receives order update events via the Schwab streamer WebSocket.
// Subscribes to the ACCT_ACTIVITY service and emits an *OrderEvent for each event.
// Reconnects automatically with exponential backoff on disconnection.
// Schwab only lets you have one WebSocket connection per account.
func (c *Client) OrderUpdates() <-chan *OrderEvent {
	flog, err := os.OpenFile(*schwabLogFlag, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("could not open schwab log file: %v", err)
	}
	ch := make(chan *OrderEvent, 64)
	ready := make(chan struct{})
	d := &orderUpdatesDaemon{client: c, ch: ch, ready: ready, flog: flog}
	go d.run()
	<-ready
	return ch
}

type orderUpdatesDaemon struct {
	client *Client
	ch     chan<- *OrderEvent
	ready  chan struct{}
	flog   *os.File
}

func (d *orderUpdatesDaemon) run() {
	try := 0
	for {
		ts1 := time.Now()
		err := d.impl()
		ts2 := time.Now()
		if err != nil {
			fmt.Fprintf(d.flog, "error reading message: %v\n", err)
			log.Printf("schwab stream: %v, reconnecting...", err)
		}
		if ts2.Sub(ts1) > 30*time.Second {
			try = 0 // connection was healthy so reset backoff
		}
		wait := time.Duration(15<<min(try, 11)) * time.Millisecond
		time.Sleep(wait)
		try++
	}
}

func (d *orderUpdatesDaemon) impl() error {

	// get streamer info (websocket URL + credentials)
	info, err := d.client.GetStreamerInfo()
	if err != nil {
		return fmt.Errorf("getting streamer info: %w", err)
	}

	// get access token for login
	accessToken, err := getAccessToken()
	if err != nil {
		return fmt.Errorf("getting access token: %w", err)
	}

	// connect websocket
	conn, _, err := netty.FastWSDial(info.StreamerSocketURL, nil)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", info.StreamerSocketURL, err)
	}
	defer conn.Close()

	// login (sent as a bare request, not wrapped in {"requests": [...]})
	loginReq := streamerRequest{
		Service:                "ADMIN",
		Command:                "LOGIN",
		RequestID:              1,
		SchwabClientCustomerID: info.SchwabClientCustomerID,
		SchwabClientCorrelID:   info.SchwabClientCorrelID,
		Parameters: map[string]string{
			"Authorization":          accessToken,
			"SchwabClientChannel":    info.SchwabClientChannel,
			"SchwabClientFunctionId": info.SchwabClientFunctionID,
		},
	}
	if err := conn.WriteJSON(loginReq); err != nil {
		return fmt.Errorf("sending login: %w", err)
	}

	// read login response
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("reading login response: %w", err)
	}
	log.Printf("schwab stream: login response: %s", msg)

	// subscribe to account activity (wrapped in {"requests": [...]})
	subReq := struct {
		Requests []streamerRequest `json:"requests"`
	}{
		Requests: []streamerRequest{{
			Service:                "ACCT_ACTIVITY",
			Command:                "SUBS",
			RequestID:              2,
			SchwabClientCustomerID: info.SchwabClientCustomerID,
			SchwabClientCorrelID:   info.SchwabClientCorrelID,
			Parameters: map[string]string{
				"keys":   "Account Activity",
				"fields": "0,1,2,3",
			},
		}},
	}
	if err := conn.WriteJSON(subReq); err != nil {
		return fmt.Errorf("sending ACCT_ACTIVITY subscription: %w", err)
	}

	// read subscription response
	_, msg, err = conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("reading subscription response: %w", err)
	}
	log.Printf("schwab stream: subscription response: %s", msg)

	// signal that we're connected and subscribed
	select {
	case <-d.ready:
		// already closed from a previous connection
	default:
		close(d.ready)
	}

	// main message loop
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

		// parse the envelope — data messages have {"data": [...]}
		var envelope struct {
			Data []streamData `json:"data"`
		}
		if err := json.Unmarshal(message, &envelope); err != nil {
			log.Printf("schwab stream: error parsing message: %v", err)
			continue
		}
		if len(envelope.Data) == 0 {
			continue // response or notify message, skip
		}

		for _, data := range envelope.Data {
			if data.Service != "ACCT_ACTIVITY" {
				continue
			}
			for _, content := range data.Content {
				if content.MessageData == "" {
					continue
				}
				event := ParseOrderEvent(json.RawMessage(content.MessageData))
				if event != nil {
					d.ch <- event
				}
			}
		}
	}
}

// streamerRequest is a request message for the Schwab streamer WebSocket.
type streamerRequest struct {
	Service                string            `json:"service"`
	Command                string            `json:"command"`
	RequestID              int               `json:"requestid"`
	SchwabClientCustomerID string            `json:"SchwabClientCustomerId"`
	SchwabClientCorrelID   string            `json:"SchwabClientCorrelId"`
	Parameters             map[string]string `json:"parameters,omitempty"`
}

// streamData is a data message from the Schwab streamer.
type streamData struct {
	Service   string          `json:"service"`
	Timestamp int64           `json:"timestamp"`
	Command   string          `json:"command"`
	Content   []streamContent `json:"content"`
}

// streamContent is a single event within a stream data message.
// Fields are numbered: "1" = Account, "2" = Message Type, "3" = Message Data.
type streamContent struct {
	Account     string `json:"1"`
	MessageType string `json:"2"`
	MessageData string `json:"3"`
	Seq         int    `json:"seq"`
	Key         string `json:"key"`
}
