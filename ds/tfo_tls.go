package ds

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"
)

// tfoTLSDialer provides TLS connections that use TCP Fast Open.
// The TLS ClientHello is sent in the TCP SYN packet, saving one RTT.
type tfoTLSDialer struct {
	config *tls.Config
}

// NewTFOTLSDialer creates a new TFO-enabled TLS dialer.
func NewTFOTLSDialer(config *tls.Config) *tfoTLSDialer {
	return &tfoTLSDialer{config: config}
}

// DialTLSContext dials a TLS connection using TCP Fast Open.
// The first TLS handshake message (ClientHello) is sent in the SYN packet.
func (d *tfoTLSDialer) DialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// Extract hostname for SNI
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	// Create a lazy connection that will TFO-dial on first write
	lazy := &lazyTFOConn{
		ctx:     ctx,
		network: network,
		addr:    addr,
	}

	// Configure TLS with SNI
	tlsConfig := d.config.Clone()
	if tlsConfig.ServerName == "" {
		tlsConfig.ServerName = host
	}

	// Wrap in TLS - when TLS writes ClientHello, it triggers the TFO dial
	tlsConn := tls.Client(lazy, tlsConfig)

	// Set deadline for handshake
	if deadline, ok := ctx.Deadline(); ok {
		tlsConn.SetDeadline(deadline)
	}

	// Perform TLS handshake - this will trigger the TFO connect
	if err := tlsConn.Handshake(); err != nil {
		lazy.Close()
		return nil, err
	}

	// Clear deadline
	tlsConn.SetDeadline(time.Time{})

	return tlsConn, nil
}

// lazyTFOConn is a net.Conn that doesn't connect until the first Write.
// The first Write's data is used as the TFO payload (sent in the SYN).
type lazyTFOConn struct {
	ctx     context.Context
	network string
	addr    string

	mu       sync.Mutex
	conn     *net.TCPConn // nil until first write
	writeErr error        // sticky error from connect
}

func (c *lazyTFOConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.writeErr != nil {
		return 0, c.writeErr
	}

	if c.conn == nil {
		// First write - do TFO connect with this data
		conn, err := DialTFO(c.ctx, c.network, c.addr, b)
		if err != nil {
			c.writeErr = err
			return 0, err
		}
		c.conn = conn
		return len(b), nil // TFO already sent the data
	}

	return c.conn.Write(b)
}

func (c *lazyTFOConn) Read(b []byte) (int, error) {
	c.mu.Lock()
	conn := c.conn
	err := c.writeErr
	c.mu.Unlock()

	if err != nil {
		return 0, err
	}
	if conn == nil {
		return 0, &net.OpError{Op: "read", Net: c.network, Err: errNotConnected}
	}
	return conn.Read(b)
}

func (c *lazyTFOConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *lazyTFOConn) LocalAddr() net.Addr {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.LocalAddr()
	}
	return nil
}

func (c *lazyTFOConn) RemoteAddr() net.Addr {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.RemoteAddr()
	}
	// Return a placeholder so TLS can use it for SNI
	addr, _ := net.ResolveTCPAddr(c.network, c.addr)
	return addr
}

func (c *lazyTFOConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.SetDeadline(t)
	}
	return nil
}

func (c *lazyTFOConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.SetReadDeadline(t)
	}
	return nil
}

func (c *lazyTFOConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return c.conn.SetWriteDeadline(t)
	}
	return nil
}

var errNotConnected = &net.AddrError{Err: "not connected", Addr: ""}

// NewTFOHTTPClient creates an http.Client that uses TCP Fast Open for TLS connections.
func NewTFOHTTPClient(tlsConfig *tls.Config) *http.Client {
	dialer := &tfoTLSDialer{config: tlsConfig}

	return &http.Client{
		Transport: &http.Transport{
			DialContext:           Dialer.DialContext, // fallback for non-TLS
			DialTLSContext:        dialer.DialTLSContext,
			MaxConnsPerHost:       4,
			MaxIdleConns:          4,
			MaxIdleConnsPerHost:   4,
			IdleConnTimeout:       45 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 0,
			ForceAttemptHTTP2:     false, // HTTP/1.1 for low latency
			DisableCompression:    true,
		},
		Timeout: 20 * time.Second,
	}
}
