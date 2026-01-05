//go:build linux

package netty

import (
	"crypto/tls"
	"net"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// Dialer is a low-latency net.Dialer that exploits the advantages of Linux.
var Dialer = &net.Dialer{
	Timeout:   5 * time.Second,
	KeepAlive: 9 * time.Second,
	Control: func(network, address string, c syscall.RawConn) error {
		return c.Control(func(fd uintptr) {
			syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1)
			syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_QUICKACK, 1)
		})
	},
}

// enableQuickAck re-enables TCP_QUICKACK on the given connection.
func enableQuickAck(conn *net.TCPConn) {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return
	}
	rawConn.Control(func(fd uintptr) {
		syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_QUICKACK, 1)
	})
}

// underlyingTCPConn extracts the underlying *net.TCPConn from a websocket.Conn.
func underlyingTCPConn(conn *websocket.Conn) *net.TCPConn {
	netConn := conn.UnderlyingConn()
	if tlsConn, ok := netConn.(*tls.Conn); ok {
		netConn = tlsConn.NetConn()
	}
	tcpConn, _ := netConn.(*net.TCPConn)
	return tcpConn
}
