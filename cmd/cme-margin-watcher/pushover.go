package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Pushover priority levels.
// Use PriorityHigh for margin changes — bypasses quiet hours, guaranteed Watch buzz.
// Use PriorityEmergency for truly critical stuff (requires ack in app, retries every 30s).
const (
	PriorityLow       = -1
	PriorityNormal    = 0
	PriorityHigh      = 1
	PriorityEmergency = 2
)

type Pushover struct {
	APIToken string
	UserKey  string
}

func (p *Pushover) Send(title, message string, priority int) error {
	vals := url.Values{
		"token":    {p.APIToken},
		"user":     {p.UserKey},
		"title":    {title},
		"message":  {message},
		"priority": {fmt.Sprintf("%d", priority)},
		"sound":    {"siren"},
	}
	if priority == PriorityEmergency {
		// Retry every 30s, expire after 1 hour if not acknowledged
		vals.Set("retry", "30")
		vals.Set("expire", "3600")
	}
	resp, err := http.PostForm("https://api.pushover.net/1/messages.json", vals)
	if err != nil {
		return fmt.Errorf("pushover POST: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("pushover returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Sendf is a convenience wrapper for formatted messages.
func (p *Pushover) Sendf(priority int, title, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	// Pushover message limit is 1024 chars
	if len(msg) > 1000 {
		msg = msg[:997] + "..."
	}
	// Trim leading/trailing whitespace that looks bad on Watch
	msg = strings.TrimSpace(msg)
	return p.Send(title, msg, priority)
}
