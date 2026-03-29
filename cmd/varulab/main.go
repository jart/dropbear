package main

import (
	"dropbear/db"
	"dropbear/ds"
	"dropbear/loggy"
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"
)

var (
	listenFlag   = flag.String("listen", "0.0.0.0:8585", "web dashboard bind address")
	jobsFlag     = flag.Int("j", 0, "max concurrent backtests (defaults to number of cores)")
	downloadFlag = flag.Bool("download", true, "enable background downloader")
	symbolsFlag  = flag.String("symbols", defaultSymbols, "comma-separated symbols to backtest")
)

var (
	gScheduler *Scheduler
	gGitRev    string
)

func main() {
	loggy.Init()
	flag.Parse()
	loggy.AlsoLogToFile()
	log.Println(strings.Join(os.Args, " "))

	validateConfig()

	// get git revision for reproducibility
	gGitRev = gitRevision()
	log.Printf("git revision: %s", gGitRev)

	// database
	database := db.Get()

	// migration 2026-03-29: remove old git_rev-based dedup index and
	// deduplicate rows so the new (symbol, date, flags) index can be created
	database.Exec(`DROP INDEX IF EXISTS idx_varulab_runs_dedup`)
	database.Exec(`DELETE FROM varulab_runs WHERE id NOT IN (
		SELECT MIN(id) FROM varulab_runs GROUP BY symbol, date, flags
	)`)

	if err := Migrate(database); err != nil {
		log.Fatalf("failed to migrate schema: %v", err)
	}

	// fix rows with duplicate flags (e.g. "-bearish -bearish -spread=.5 -spread=.5")
	cleanupDuplicateFlags(database)

	// reset any runs left in running/claimed state from a previous crash
	res, _ := database.Exec(`UPDATE varulab_runs SET status = 'pending' WHERE status IN ('running', 'claimed')`)
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("reset %d stale running/claimed runs to pending", n)
	}

	// cancel pending runs for symbols no longer configured
	symbols := parseSymbols()
	symSet := map[string]bool{}
	for _, s := range symbols {
		symSet[s] = true
	}
	var staleSymbols []string
	rows, _ := database.Query(`SELECT DISTINCT symbol FROM varulab_runs WHERE status = 'pending'`)
	if rows != nil {
		for rows.Next() {
			var s string
			rows.Scan(&s)
			if !symSet[s] {
				staleSymbols = append(staleSymbols, s)
			}
		}
		rows.Close()
	}
	for _, s := range staleSymbols {
		res, err := database.Exec(`DELETE FROM varulab_runs WHERE status = 'pending' AND symbol = ?`, s)
		if err == nil {
			if n, _ := res.RowsAffected(); n > 0 {
				log.Printf("deleted %d pending runs for removed symbol %s", n, s)
			}
		}
	}

	// scheduler
	jobs := *jobsFlag
	if jobs <= 0 {
		jobs = ds.PhysicalCores()
	}
	log.Printf("using %d concurrent backtest slots", jobs)
	gScheduler = NewScheduler(database, jobs)

	// generate initial runs
	n := gScheduler.GenerateRuns(gGitRev)
	log.Printf("generated %d pending runs", n)

	// start scheduler
	go gScheduler.Run()

	// start downloader
	if *downloadFlag {
		quit := make(chan struct{})
		go runDownloader(parseSymbols(), quit)
		defer close(quit)
	}

	// start periodic state broadcasts
	go func() {
		for {
			time.Sleep(5 * time.Second)
			broadcastState(database)
		}
	}()

	// start web server
	startWeb()

	// wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	<-sigCh
	log.Println("shutting down...")
	gScheduler.Stop()
}

func parseSymbols() []string {
	return strings.Fields(*symbolsFlag)
}

func gitRevision() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func now() int64 {
	return time.Now().UnixNano()
}
