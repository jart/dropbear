package netty

import "dropbear/clocky"

// Reconnect calls fn in a loop forever, applying exponential backoff between
// failed attempts (capped at roughly 30s). If an attempt stays up longer
// than 30s before failing, the backoff resets to its minimum so a feed that
// drops after being healthy for a while doesn't inherit a long wait from an
// earlier failure streak. logf is called with the error after every failed
// attempt; pass nil to disable logging.
func Reconnect(name string, fn func() error, logf func(format string, v ...any)) {
	try := 0
	for {
		start := clocky.Now()
		err := fn()
		if err != nil && logf != nil {
			logf("%s error, reconnecting: %v", name, err)
		}
		if clocky.Since(start) > 30*clocky.Second {
			try = 0 // connection was healthy so reset backoff
		}
		clocky.Sleep(clocky.Duration(15<<min(try, 11)) * clocky.Millisecond)
		try++
	}
}
