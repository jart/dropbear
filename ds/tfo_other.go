//go:build !linux && !darwin

package ds

import (
	"context"
	"net"
)

// DialTFO connects to the address and sends initial data.
// On this platform, TFO is not supported so it falls back to normal dial + write.
func DialTFO(ctx context.Context, network, address string, data []byte) (*net.TCPConn, error) {
	conn, err := Dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	tc := conn.(*net.TCPConn)
	if len(data) > 0 {
		if _, err := tc.Write(data); err != nil {
			tc.Close()
			return nil, err
		}
	}
	return tc, nil
}
