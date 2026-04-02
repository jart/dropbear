package main

import (
	"dropbear/ds"
	"dropbear/loggy"
	"dropbear/symbol"
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
	resumeFlag   = flag.Bool("resume", false, "resume the most recently modified experiment")
	openFlag     = flag.String("open", "", "open a specific experiment by name")
	newFlag      = flag.Bool("new", false, "create a new experiment")
)

var (
	gExperiment *Experiment
	gScheduler  *Scheduler
	gGitRev     string
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

	// open or create experiment
	var expName string
	if *openFlag != "" {
		expName = *openFlag
		log.Printf("opening experiment: %s", expName)
	} else if *resumeFlag {
		var err error
		expName, err = latestExperimentName()
		if err != nil {
			log.Fatalf("cannot resume: %v", err)
		}
		log.Printf("resuming experiment: %s", expName)
	} else if *newFlag {
		expName = newExperimentName()
		log.Printf("creating experiment: %s", expName)
	}

	if expName != "" {
		var err error
		gExperiment, err = openExperiment(expName)
		if err != nil {
			log.Fatalf("failed to open experiment: %v", err)
		}
		log.Printf("experiment: %s", gExperiment.Name)

		// reset any runs left in running/claimed state from a previous crash
		res, _ := gExperiment.DB.Exec(`UPDATE varulab_runs SET status = 'pending' WHERE status IN ('running', 'claimed')`)
		if n, _ := res.RowsAffected(); n > 0 {
			log.Printf("reset %d stale running/claimed runs to pending", n)
		}

		// cancel pending runs for symbols no longer configured
		symbols := parseSymbols()
		symSet := map[symbol.Symbol]bool{}
		for _, s := range symbols {
			symSet[s] = true
		}
		var staleSymbols []symbol.Symbol
		rows, _ := gExperiment.DB.Query(`SELECT DISTINCT symbol FROM varulab_runs WHERE status = 'pending'`)
		if rows != nil {
			for rows.Next() {
				var s string
				rows.Scan(&s)
				sym, err := symbol.Parse(s)
				if err != nil {
					log.Printf("skipping unparseable symbol in DB: %q", s)
					continue
				}
				if !symSet[sym] {
					staleSymbols = append(staleSymbols, sym)
				}
			}
			rows.Close()
		}
		for _, s := range staleSymbols {
			res, err := gExperiment.DB.Exec(`DELETE FROM varulab_runs WHERE status = 'pending' AND symbol = ?`, s)
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
		gScheduler = NewScheduler(gExperiment.DB, jobs)

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
				broadcastState(gExperiment.DB)
			}
		}()
	} else {
		log.Printf("read-only mode (no active experiment)")
	}

	// start web server
	startWeb()

	// wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	<-sigCh
	log.Println("shutting down...")
	gScheduler.Stop()
}

func parseSymbols() []symbol.Symbol {
	fields := strings.Fields(*symbolsFlag)
	syms := make([]symbol.Symbol, len(fields))
	for i, s := range fields {
		var err error
		syms[i], err = symbol.Parse(s)
		if err != nil {
			log.Fatalf("invalid symbol: %s", s)
		}
	}
	return syms
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
