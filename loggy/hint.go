package loggy

import (
	"log"
	"time"
)

// Hinter rate-limits log messages to at most once per second per unique
// format string. Each goroutine that logs should have its own Hinter so
// no synchronization is needed.
type Hinter struct {
	times map[string]time.Time
}

func NewHinter() *Hinter {
	return &Hinter{times: map[string]time.Time{}}
}

func (h *Hinter) Hint(format string, args ...any) {
	now := time.Now()
	if last, ok := h.times[format]; ok && now.Sub(last) < time.Second {
		return
	}
	h.times[format] = now
	log.Printf(format, args...)
}
