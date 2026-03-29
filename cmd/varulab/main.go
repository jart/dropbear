package main

import (
	"dropbear/db"
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
	datadirFlag  = flag.String("datadir", os.ExpandEnv("$HOME/databento"), "data directory")
	jobsFlag     = flag.Int("j", 50, "max concurrent backtests")
	downloadFlag = flag.Bool("download", true, "enable background downloader")
	symbolsFlag  = flag.String("symbols", defaultSymbols, "comma-separated symbols to backtest")
)

const defaultSymbols = "AAPL,AMD,AMZN,GOOG,GOOGL,HOOD,INTC,IWM,META,MSFT,MU,NFLX,NVDA,ORCL,PLTR,QQQ,SHOP,SPXW,SPY,TSLA,TSM,VRT"

var (
	gScheduler *Scheduler
	gGitRev    string
)

func main() {
	loggy.Init()
	flag.Parse()
	log.Println(strings.Join(os.Args, " "))

	// get git revision for reproducibility
	gGitRev = gitRevision()
	log.Printf("git revision: %s", gGitRev)

	// database
	database := db.Get()
	if err := Migrate(database); err != nil {
		log.Fatalf("failed to migrate schema: %v", err)
	}

	// scheduler
	gScheduler = NewScheduler(database, *jobsFlag)

	// generate initial runs
	n := gScheduler.GenerateRuns(gGitRev)
	log.Printf("generated %d pending runs", n)

	// start scheduler
	go gScheduler.Run()

	// start downloader
	if *downloadFlag {
		quit := make(chan struct{})
		go runDownloader(*datadirFlag, parseSymbols(), quit)
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
	return strings.Split(*symbolsFlag, ",")
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
