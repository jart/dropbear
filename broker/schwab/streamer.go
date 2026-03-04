package schwab

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"dropbear/netty"
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

// OrderUpdate represents an account activity event from the Schwab streamer WebSocket.
type OrderUpdate struct {
	Event         string          // e.g. "OrderCreated", "OrderFillCompleted", "CancelAccepted"
	SchwabOrderID string          // Schwab's internal order ID
	AccountNumber string          // account number
	Seq           int             // sequence number within the stream
	Timestamp     int64           // stream message timestamp (millis since epoch)
	RawData       json.RawMessage // the full JSON from field "3" (deeply nested order event data)
}

// OrderUpdates returns a channel that receives order update events via the Schwab streamer WebSocket.
// Subscribes to the ACCT_ACTIVITY service and emits an OrderUpdate for each event.
// Reconnects automatically with exponential backoff on disconnection.
func (c *Client) OrderUpdates() <-chan OrderUpdate {
	ch := make(chan OrderUpdate, 64)
	d := &orderUpdatesDaemon{client: c, ch: ch}
	go d.run()
	return ch
}

type orderUpdatesDaemon struct {
	client *Client
	ch     chan<- OrderUpdate
}

func (d *orderUpdatesDaemon) run() {
	try := 0
	for {
		ts1 := time.Now()
		err := d.impl()
		ts2 := time.Now()
		if err != nil {
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

	// main message loop
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}

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
				update := OrderUpdate{
					Event:     content.MessageType,
					Seq:       content.Seq,
					Timestamp: data.Timestamp,
				}
				if content.MessageData != "" {
					update.RawData = json.RawMessage(content.MessageData)
					// extract SchwabOrderID and AccountNumber from the nested JSON
					var base struct {
						SchwabOrderID string `json:"SchwabOrderID"`
						AccountNumber string `json:"AccountNumber"`
					}
					json.Unmarshal([]byte(content.MessageData), &base)
					update.SchwabOrderID = base.SchwabOrderID
					update.AccountNumber = base.AccountNumber
				}
				d.ch <- update
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
