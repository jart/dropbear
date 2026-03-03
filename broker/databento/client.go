// Package databento provides a client for Databento's live market data feed.
//
// The client connects to Databento's Live Subscription Gateway (LSG) using
// a binary protocol over TCP. Authentication uses CRAM-SHA256.
//
// Reference: https://github.com/databento/databento-cpp
package databento

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"dropbear/clocky"
)

const (
	DefaultPort    = 13000
	DefaultTimeout = 30 * time.Second
)

var (
	ErrAuth     = errors.New("databento: authentication failed")
	ErrProtocol = errors.New("databento: protocol error")
)

// Client connects to Databento's Live Subscription Gateway.
type Client struct {
	conn     net.Conn
	reader   *bufio.Reader
	apiKey   ApiKey
	dataset  string
	subID    int      // incrementing subscription ID
	metadata Metadata // stored after Start for version upgrades
}

// Subscription describes a live data subscription request.
type Subscription struct {
	Schema   Schema
	SType    SType
	Symbols  []string
	Start    clocky.Time // optional start time (zero means omit)
	Snapshot bool
}

// Dial connects to the Databento LSG for the given dataset and authenticates.
// Dataset examples: "XNAS.ITCH" (NASDAQ), "OPRA.PILLAR" (options)
func Dial(dataset string, apiKey ApiKey) (*Client, error) {
	c := &Client{
		apiKey:  apiKey,
		dataset: dataset,
	}
	addr := fmt.Sprintf("%s.lsg.databento.com:%d", strings.ToLower(strings.ReplaceAll(dataset, ".", "-")), DefaultPort)
	conn, err := net.DialTimeout("tcp", addr, DefaultTimeout)
	if err != nil {
		return nil, fmt.Errorf("databento: connect to %s: %w", addr, err)
	}
	c.conn = conn
	c.reader = bufio.NewReader(conn)
	if err := c.authenticate(); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

// Close closes the connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Subscribe sends a subscription request to the gateway. Symbols are chunked
// into groups of 128 per message (LSG maximum is 500).
func (c *Client) Subscribe(sub Subscription) error {
	const maxSymbolsPerMsg = 128
	snapshot := "0"
	if sub.Snapshot {
		snapshot = "1"
	}
	for i := 0; i < len(sub.Symbols); i += maxSymbolsPerMsg {
		end := min(i+maxSymbolsPerMsg, len(sub.Symbols))
		chunk := sub.Symbols[i:end]
		c.subID++
		isLast := "0"
		if end == len(sub.Symbols) {
			isLast = "1"
		}
		msg := fmt.Sprintf("schema=%s|stype_in=%s|id=%d|symbols=%s|snapshot=%s|is_last=%s",
			sub.Schema.String(),
			sub.SType.String(),
			c.subID,
			strings.Join(chunk, " "),
			snapshot,
			isLast,
		)
		if !sub.Start.IsZero() {
			msg += "|start=" + strconv.FormatInt(sub.Start.UnixNano(), 10)
		}
		msg += "\n"
		if _, err := c.conn.Write([]byte(msg)); err != nil {
			return fmt.Errorf("databento: send subscribe: %w", err)
		}
	}
	return nil
}

// Start sends the start_session command, reads the DBN metadata header, and
// begins streaming. After Start returns, call NextRecord to read records.
func (c *Client) Start() (*Metadata, error) {
	if _, err := c.conn.Write([]byte("start_session\n")); err != nil {
		return nil, fmt.Errorf("databento: send start_session: %w", err)
	}
	// Clear read deadline for streaming
	c.conn.SetReadDeadline(time.Time{})
	err := decodeMetadata(c.reader, &c.metadata)
	if err != nil {
		return nil, fmt.Errorf("databento: read metadata: %w", err)
	}
	return &c.metadata, nil
}

// Read reads the next record from the stream.
func (c *Client) Read() (any, error) {
	rec, err := decodeRecord(c.reader)
	if err != nil {
		return nil, err
	}
	obj, err := castRecord(c.metadata.Version, rec)
	if err != nil {
		return nil, err
	}
	return obj, nil
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

// authenticate performs CRAM-SHA256 authentication with the gateway.
func (c *Client) authenticate() error {
	// Read greeting: lsg_version=...\n
	greeting, err := c.readControlMessage()
	if err != nil {
		return fmt.Errorf("databento: read greeting: %w", err)
	}
	if _, ok := greeting["lsg_version"]; !ok {
		return fmt.Errorf("%w: missing lsg_version in greeting", ErrProtocol)
	}

	// Read challenge: cram=...\n
	challenge, err := c.readControlMessage()
	if err != nil {
		return fmt.Errorf("databento: read challenge: %w", err)
	}
	cramKey, ok := challenge["cram"]
	if !ok {
		return fmt.Errorf("%w: missing cram in challenge", ErrProtocol)
	}

	// Compute CRAM response: SHA256(challenge|apikey)-bucketid
	bucketID := string(c.apiKey)[len(c.apiKey)-5:]
	h := sha256.New()
	h.Write([]byte(cramKey))
	h.Write([]byte("|"))
	h.Write([]byte(c.apiKey))
	cramReply := hex.EncodeToString(h.Sum(nil)) + "-" + bucketID

	// Send authentication request
	authMsg := fmt.Sprintf("auth=%s|dataset=%s|encoding=dbn|ts_out=0|compression=none\n", cramReply, c.dataset)
	if _, err := c.conn.Write([]byte(authMsg)); err != nil {
		return fmt.Errorf("databento: send auth: %w", err)
	}

	// Read authentication response
	resp, err := c.readControlMessage()
	if err != nil {
		return fmt.Errorf("databento: read auth response: %w", err)
	}
	if resp["success"] != "1" {
		errMsg := resp["error"]
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return fmt.Errorf("%w: %s", ErrAuth, errMsg)
	}
	return nil
}
