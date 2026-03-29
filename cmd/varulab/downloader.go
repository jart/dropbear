package main

import (
	"dropbear/clocky"
	"dropbear/ds/nyse"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// zeroDTE symbols have 0DTE options every trading day.
// All other symbols only have weekly options expiring on Fridays.
var zeroDTESymbols = map[string]bool{
	"SPXW": true,
	"NDX":  true,
	"XSP":  true,
	"SPY":  true,
	"QQQ":  true,
	"IWM":  true,
}

// runDownloader downloads .dbn files breadth-first backwards in time.
// It iterates one day at a time across all symbols so that recent data
// for every symbol is available before going deeper into history.
func runDownloader(datadir string, symbols []string, quit <-chan struct{}) {
	// start from yesterday (today's data may still be in-flight)
	day := clocky.Time(time.Now().Add(-24 * time.Hour).UnixNano())

	for {
		select {
		case <-quit:
			return
		default:
		}

		if !nyse.IsTradingDay(day) {
			day = day.Add(-clocky.Day)
			continue
		}

		downloaded := false
		for _, sym := range symbols {
			select {
			case <-quit:
				return
			default:
			}

			if !shouldDownload(sym, day) {
				continue
			}

			dateStr := day.Format("2006-01-02")
			path := filepath.Join(datadir, sym, dateStr+".dbn")

			if fileExists(path) {
				continue
			}

			log.Printf("downloading %s %s", sym, dateStr)
			if err := downloadDbn(sym, dateStr, path); err != nil {
				log.Printf("download failed %s %s: %v", sym, dateStr, err)
				continue
			}
			log.Printf("downloaded %s %s", sym, dateStr)
			downloaded = true
			notifyScheduler()
		}

		if !downloaded {
			// all files for this day exist or were skipped
		}
		day = day.Add(-clocky.Day)
	}
}

func shouldDownload(sym string, day clocky.Time) bool {
	if zeroDTESymbols[sym] {
		return true
	}
	return day.Weekday() == clocky.Friday
}

func downloadDbn(sym, date, outputPath string) error {
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// use a temp file to avoid partial downloads being picked up
	tmpPath := outputPath + ".tmp"
	defer os.Remove(tmpPath)

	args := []string{"run", "./broker/databento/cmd/dbndownload",
		"-date", date,
		"-sym", sym,
		"-schema", "cmbp-1",
		"-j", "50",
		"-o", tmpPath,
	}
	log.Printf("exec: go %s", strings.Join(args, " "))
	cmd := exec.Command("go", args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dbndownload: %w", err)
	}

	return os.Rename(tmpPath, outputPath)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// discoverDbnFiles finds all .dbn files under the configured data directory.
type DbnFile struct {
	Symbol string
	Date   string
	Path   string
}

func discoverDbnFiles() []DbnFile {
	symbols := map[string]bool{}
	for _, s := range parseSymbols() {
		symbols[s] = true
	}
	var files []DbnFile
	entries, err := os.ReadDir(*datadirFlag)
	if err != nil {
		return files
	}
	for _, symDir := range entries {
		if !symDir.IsDir() || !symbols[symDir.Name()] {
			continue
		}
		sym := symDir.Name()
		symPath := filepath.Join(*datadirFlag, sym)
		dateEntries, err := os.ReadDir(symPath)
		if err != nil {
			continue
		}
		for _, de := range dateEntries {
			name := de.Name()
			if filepath.Ext(name) != ".dbn" {
				continue
			}
			date := name[:len(name)-4] // strip .dbn
			files = append(files, DbnFile{
				Symbol: sym,
				Date:   date,
				Path:   filepath.Join(symPath, name),
			})
		}
	}
	return files
}
