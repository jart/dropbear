// Package databento provides a client for Databento's live market data feed.
//
// The client connects to Databento's Live Subscription Gateway (LSG) using
// a binary protocol over TCP. Authentication uses CRAM-SHA256.
//
// Reference: https://github.com/NimbleMarkets/dbn-go
package databento

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const (
	// DefaultPort is the Databento LSG port.
	DefaultPort = 13000

	// DefaultTimeout for connection and reads.
	DefaultTimeout = 30 * time.Second
)

var (
	ErrAuth     = errors.New("databento: authentication failed")
	ErrProtocol = errors.New("databento: protocol error")
)

// Client connects to Databento's Live Subscription Gateway.
type Client struct {
	conn    net.Conn
	reader  *bufio.Reader
	apiKey  string
	dataset string
}

// NewClient creates a new Databento client for the given dataset.
// Dataset examples: "XNAS.ITCH" (NASDAQ), "GLBX.MDP3" (CME futures)
func NewClient(dataset, apiKey string) *Client {
	return &Client{
		apiKey:  apiKey,
		dataset: dataset,
	}
}

// Connect establishes a TCP connection to the Databento gateway.
func (c *Client) Connect() error {
	addr := fmt.Sprintf("%s.lsg.databento.com:%d", strings.ToLower(c.dataset), DefaultPort)
	conn, err := net.DialTimeout("tcp", addr, DefaultTimeout)
	if err != nil {
		return fmt.Errorf("databento: connect to %s: %w", addr, err)
	}
	c.conn = conn
	c.reader = bufio.NewReader(conn)
	return nil
}

// Close closes the connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Authenticate performs CRAM-SHA256 authentication with the gateway.
// Returns the session ID on success.
func (c *Client) Authenticate() (string, error) {
	// Read greeting: lsg_version=...\n
	greeting, err := c.readControlMessage()
	if err != nil {
		return "", fmt.Errorf("databento: read greeting: %w", err)
	}
	if _, ok := greeting["lsg_version"]; !ok {
		return "", fmt.Errorf("%w: missing lsg_version in greeting", ErrProtocol)
	}

	// Read challenge: cram=...\n
	challenge, err := c.readControlMessage()
	if err != nil {
		return "", fmt.Errorf("databento: read challenge: %w", err)
	}
	cramKey, ok := challenge["cram"]
	if !ok {
		return "", fmt.Errorf("%w: missing cram in challenge", ErrProtocol)
	}

	// Compute CRAM response: SHA256(challenge|apikey)-bucketid
	// bucket_id is the last 5 characters of the API key
	bucketID := c.apiKey[len(c.apiKey)-5:]
	h := sha256.New()
	h.Write([]byte(cramKey))
	h.Write([]byte(c.apiKey))
	cramReply := hex.EncodeToString(h.Sum(nil)) + "-" + bucketID

	// Send authentication request
	authMsg := fmt.Sprintf("auth=%s|dataset=%s|encoding=dbn\n", cramReply, c.dataset)
	if _, err := c.conn.Write([]byte(authMsg)); err != nil {
		return "", fmt.Errorf("databento: send auth: %w", err)
	}

	// Read authentication response
	resp, err := c.readControlMessage()
	if err != nil {
		return "", fmt.Errorf("databento: read auth response: %w", err)
	}
	if resp["success"] != "1" {
		errMsg := resp["error"]
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return "", fmt.Errorf("%w: %s", ErrAuth, errMsg)
	}

	return resp["session_id"], nil
}

// readControlMessage reads a pipe-delimited control message.
// Format: "key1=value1|key2=value2|...\n"
func (c *Client) readControlMessage() (map[string]string, error) {
	c.conn.SetReadDeadline(time.Now().Add(DefaultTimeout))
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(line, "\n")

	result := make(map[string]string)
	for _, part := range strings.Split(line, "|") {
		if idx := strings.Index(part, "="); idx > 0 {
			result[part[:idx]] = part[idx+1:]
		}
	}
	return result, nil
}

// GetKey reads the Databento API key from ~/.databento.key
func GetKey() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(home + "/.databento.key")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
