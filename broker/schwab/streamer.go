package schwab

import (
	"fmt"

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
