package main

import (
	"os"
	"time"
)

type Config struct {
	PushoverToken    string
	PushoverUser     string
	StateFile        string
	AdvisoryInterval time.Duration
	SPANInterval     time.Duration
	MarginsInterval  time.Duration

	// Watchlist: CME product codes to watch in SPAN file and margins API.
	// CL = WTI crude, BRN = Brent, NG = natural gas, GC = gold, SI = silver
	WatchProducts []string
}

func loadConfig() Config {
	return Config{
		PushoverToken:    mustEnv("PUSHOVER_TOKEN"),
		PushoverUser:     mustEnv("PUSHOVER_USER"),
		StateFile:        getEnv("STATE_FILE", "state.json"),
		AdvisoryInterval: parseDuration(getEnv("ADVISORY_INTERVAL", "2m")),
		SPANInterval:     parseDuration(getEnv("SPAN_INTERVAL", "15m")),
		MarginsInterval:  parseDuration(getEnv("MARGINS_INTERVAL", "5m")),
		WatchProducts:    []string{"CL", "BRN", "NG", "GC", "SI"},
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("missing required env var: " + key)
	}
	return v
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		panic("invalid duration: " + s)
	}
	return d
}
