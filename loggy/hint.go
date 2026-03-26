package loggy

import (
	"log"
	"time"
)

var hintTimes = map[string]time.Time{}

// Hint logs a message at most once per second per unique format string.
// Use this for diagnostics that explain why something isn't happening,
// without spamming the log when the condition persists.
func Hint(format string, args ...any) {
	now := time.Now()
	if last, ok := hintTimes[format]; ok && now.Sub(last) < time.Second {
		return
	}
	hintTimes[format] = now
	log.Printf(format, args...)
}
