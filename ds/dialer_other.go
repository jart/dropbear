//go:build !linux

package ds

import (
	"net"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// Dialer is a low-latency net.Dialer that's portable across OSes.
var Dialer = &net.Dialer{
	Timeout:   5 * time.Second,
	KeepAlive: 9 * time.Second,
	Control: func(network, address string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) {
			syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1)
		})
	},
}

func enableQuickAck(conn *net.TCPConn) {}

func underlyingTCPConn(_ *websocket.Conn) *net.TCPConn {
	return nil
}
