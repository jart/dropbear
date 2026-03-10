package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("cme-margin-watcher starting")

	cfg := loadConfig()
	state := loadState(cfg.StateFile)

	push := &Pushover{
		APIToken: cfg.PushoverToken,
		UserKey:  cfg.PushoverUser,
	}

	// Send a startup ping so you know the daemon is alive
	if err := push.Sendf(PriorityNormal,
		"✅ CME Margin Watcher",
		"Started. Watching: %v\nAdvisory: every %s | SPAN: every %s | Margins: every %s",
		cfg.WatchProducts,
		cfg.AdvisoryInterval, cfg.SPANInterval, cfg.MarginsInterval,
	); err != nil {
		log.Printf("startup ping failed: %v (check PUSHOVER_TOKEN / PUSHOVER_USER)", err)
	}

	// Run all three monitors as goroutines
	go watchAdvisories(cfg, state, push)
	go watchSPAN(cfg, state, push)
	go watchMargins(cfg, state, push)

	// Block until SIGINT or SIGTERM
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	s := <-sig
	log.Printf("received %s, shutting down", s)
	state.save()
}
