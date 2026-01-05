//go:build darwin

package netty

import (
	"context"
	"log"
	"net"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// TFODebug enables debug logging for TCP Fast Open
var TFODebug = false

const tcpFastopenForceEnable = 0x218

// DialTFO connects to the address and sends initial data in the SYN packet
// using TCP Fast Open. If data is empty, falls back to normal dial.
func DialTFO(ctx context.Context, network, address string, data []byte) (*net.TCPConn, error) {
	if len(data) == 0 {
		conn, err := Dialer.DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return conn.(*net.TCPConn), nil
	}

	// Resolve address
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, &net.OpError{Op: "dial", Net: network, Addr: nil, Err: err}
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, &net.OpError{Op: "dial", Net: network, Addr: nil, Err: err}
	}
	if len(ips) == 0 {
		return nil, &net.OpError{Op: "dial", Net: network, Addr: nil, Err: &net.DNSError{Err: "no addresses", Name: host}}
	}

	portNum, err := net.DefaultResolver.LookupPort(ctx, network, port)
	if err != nil {
		return nil, &net.OpError{Op: "dial", Net: network, Addr: nil, Err: err}
	}

	// Try each resolved address
	var firstErr error
	for _, ip := range ips {
		raddr := &net.TCPAddr{IP: ip.IP, Port: portNum, Zone: ip.Zone}
		conn, err := dialTFOSingle(ctx, raddr, data)
		if err == nil {
			return conn, nil
		}
		if firstErr == nil {
			firstErr = &net.OpError{Op: "dial", Net: network, Addr: raddr, Err: err}
		}
	}

	return nil, firstErr
}

func dialTFOSingle(_ context.Context, raddr *net.TCPAddr, data []byte) (*net.TCPConn, error) {
	// Determine address family
	family := unix.AF_INET
	if raddr.IP.To4() == nil {
		family = unix.AF_INET6
	}

	// Create socket (macOS needs ForkLock for CloseOnExec)
	syscall.ForkLock.RLock()
	fd, err := unix.Socket(family, unix.SOCK_STREAM, unix.IPPROTO_TCP)
	if err != nil {
		syscall.ForkLock.RUnlock()
		return nil, os.NewSyscallError("socket", err)
	}
	unix.CloseOnExec(fd)
	syscall.ForkLock.RUnlock()

	if err = unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, os.NewSyscallError("setnonblock", err)
	}

	// Set low-latency socket options
	unix.SetsockoptInt(fd, unix.IPPROTO_TCP, unix.TCP_NODELAY, 1)

	// Enable TFO force (disables Darwin's backoff mechanism)
	unix.SetsockoptInt(fd, unix.IPPROTO_TCP, tcpFastopenForceEnable, 1)

	// Build sockaddr
	var sa unix.Sockaddr
	if family == unix.AF_INET {
		sa4 := &unix.SockaddrInet4{Port: raddr.Port}
		copy(sa4.Addr[:], raddr.IP.To4())
		sa = sa4
	} else {
		sa6 := &unix.SockaddrInet6{Port: raddr.Port}
		copy(sa6.Addr[:], raddr.IP.To16())
		sa = sa6
	}

	// Wrap fd in os.File to get RawConn
	f := os.NewFile(uintptr(fd), "tfo")
	defer f.Close()

	rawConn, err := f.SyscallConn()
	if err != nil {
		return nil, err
	}

	// Connect with TFO using connectx()
	var n int
	var connectErr error
	var done bool

	writeErr := rawConn.Write(func(fd uintptr) bool {
		if done {
			return true // Already called connectx, just waiting for connection
		}
		var iov []unix.Iovec
		if len(data) > 0 {
			iov = []unix.Iovec{{Base: &data[0], Len: uint64(len(data))}}
		}
		var sent uintptr
		sent, connectErr = unix.Connectx(int(fd), 0, nil, sa, unix.SAE_ASSOCID_ANY, unix.CONNECT_DATA_IDEMPOTENT, iov, nil)
		n = int(sent)
		if TFODebug {
			log.Printf("TFO connectx: n=%d, err=%v, datalen=%d", n, connectErr, len(data))
		}
		if connectErr == unix.EINPROGRESS {
			done = true
			connectErr = nil
			return false
		}
		return true
	})

	if writeErr != nil {
		return nil, writeErr
	}

	if connectErr != nil {
		return nil, os.NewSyscallError("connectx", connectErr)
	}

	// Check for socket error after connect
	ctrlErr := rawConn.Control(func(fd uintptr) {
		nerr, _ := unix.GetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_ERROR)
		if nerr != 0 {
			connectErr = syscall.Errno(nerr)
		}
	})
	if ctrlErr != nil {
		return nil, ctrlErr
	}
	if connectErr != nil {
		return nil, os.NewSyscallError("connect", connectErr)
	}

	// Convert to net.TCPConn
	conn, err := net.FileConn(f)
	if err != nil {
		return nil, err
	}
	tc := conn.(*net.TCPConn)

	// Send any remaining data if connectx didn't send it all
	if n < len(data) {
		if _, err := tc.Write(data[n:]); err != nil {
			tc.Close()
			return nil, err
		}
	}

	// Configure keepalive
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(Dialer.KeepAlive)

	return tc, nil
}
