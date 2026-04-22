package netty

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const wsReadTimeout = 30 * time.Second

// fastWSDialer is optimized for low-latency market data streams.
var fastWSDialer = &websocket.Dialer{
	NetDialContext: Dialer.DialContext,
	TLSClientConfig: &tls.Config{
		SessionTicketsDisabled: false,
		ClientSessionCache:     tls.NewLRUClientSessionCache(32),
		NextProtos:             []string{"http/1.1"},
	},
}

// FastWSConn wraps websocket.Conn with low-latency optimizations applied.
type FastWSConn struct {
	*websocket.Conn
	tcpConn *net.TCPConn
}

// FastWSDial dials a websocket and configures it for low latency.
// Sets up ping and pong handlers and read deadlines automatically.
func FastWSDial(url string, requestHeader http.Header) (*FastWSConn, *http.Response, error) {
	conn, resp, err := fastWSDialer.Dial(url, requestHeader)
	if err != nil {
		return nil, resp, err
	}

	wsc := &FastWSConn{
		Conn:    conn,
		tcpConn: underlyingTCPConn(conn),
	}

	// Set up ping handler with TCP_QUICKACK refresh
	conn.SetPingHandler(func(data string) error {
		wsc.refreshQuickAck()
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(time.Second))
	})

	// Reset read deadline on pong
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		return nil
	})

	// Initial read deadline
	conn.SetReadDeadline(time.Now().Add(wsReadTimeout))

	// Send pings to keep low-volume connections alive
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)) != nil {
				return
			}
		}
	}()

	return wsc, resp, nil
}

// ReadMessage reads a message and resets the read deadline.
// Also refreshes TCP_QUICKACK on Linux for minimum ACK latency.
func (c *FastWSConn) ReadMessage() (int, []byte, error) {
	c.refreshQuickAck()
	c.Conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	return c.Conn.ReadMessage()
}

func (c *FastWSConn) refreshQuickAck() {
	if c.tcpConn != nil {
		enableQuickAck(c.tcpConn)
	}
}
