package main

import (
	"dropbear/cboe"
	"dropbear/clocky"
	"dropbear/symbol"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// activeDirs returns the subset of dataDirs that exist on disk.
func activeDirs() []string {
	var dirs []string
	for _, d := range kDataDirs {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// diskUsagePercent returns the percentage of disk used for the filesystem
// containing the given path.
func diskUsagePercent(path string) float64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 100 // assume full on error
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	if total == 0 {
		return 100
	}
	return float64(total-free) / float64(total) * 100
}

// downloadDir returns the first active data dir with <90% disk usage,
// or empty string if all are full.
func downloadDir() string {
	for _, d := range activeDirs() {
		if diskUsagePercent(d) < 90 {
			return d
		}
	}
	return ""
}

// findDbnFile searches all active data dirs for a .dbn file.
func findDbnFile(sym symbol.Symbol, date string) string {
	for _, d := range activeDirs() {
		path := filepath.Join(d, sym.String(), date+".dbn")
		if fileExists(path) {
			return path
		}
	}
	return ""
}

// runDownloader downloads .dbn files breadth-first backwards in time.
// It iterates one day at a time across all symbols so that recent data
// for every symbol is available before going deeper into history.
func runDownloader(symbols []symbol.Symbol, quit <-chan struct{}) {
	date := clocky.Time(time.Now().Add(-24 * time.Hour).UnixNano())

	for {
		select {
		case <-quit:
			return
		default:
		}

		if date.Format("2006-01-02") < kEarliestDate {
			log.Printf("downloader reached earliest date %s, stopping", kEarliestDate)
			<-quit
			return
		}

		year, month, day := date.Date()
		if !cboe.IsTradingDay(year, month, day) {
			date = date.Add(-clocky.Day)
			continue
		}

		for _, sym := range symbols {
			select {
			case <-quit:
				return
			default:
			}

			if !shouldDownload(sym, date) {
				continue
			}

			dateStr := date.Format("2006-01-02")
			if findDbnFile(sym, dateStr) != "" {
				continue
			}

			dest := downloadDir()
			if dest == "" {
				log.Printf("all data dirs are >=90%% full, pausing downloads")
				select {
				case <-quit:
					return
				case <-time.After(10 * time.Minute):
					continue
				}
			}

			path := filepath.Join(dest, sym.String(), dateStr+".dbn")
			log.Printf("downloading %s %s -> %s", sym, dateStr, dest)
			if err := downloadDbn(sym, dateStr, path); err != nil {
				log.Printf("download failed %s %s: %v", sym, dateStr, err)
				continue
			}
			log.Printf("downloaded %s %s", sym, dateStr)
			for _, dataset := range equityDatasets(sym) {
				if err := injectEquity(sym, path, dataset); err != nil {
					log.Printf("equity inject failed %s %s %s: %v", sym, dateStr, dataset, err)
				}
			}
			n := gScheduler.GenerateRuns(gGitRev)
			if n > 0 {
				log.Printf("generated %d new runs after download", n)
			}
			notifyScheduler()
		}

		date = date.Add(-clocky.Day)
	}
}

func shouldDownload(sym symbol.Symbol, date clocky.Time) bool {
	year, month, day := date.Date()
	return cboe.HasOptionChain(sym, year, month, day)
}

func downloadDbn(sym symbol.Symbol, date, outputPath string) error {
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// use a temp file to avoid partial downloads being picked up
	tmpPath := outputPath + ".tmp"
	defer os.Remove(tmpPath)

	args := []string{
		"run", "./broker/databento/cmd/dbndownload",
		"-date", date,
		"-sym", sym.String(),
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

// equityDatasets returns the Databento datasets for a symbol's underlying
// equity quotes across the three DMA-routable exchanges, or nil for index
// options with no tradeable underlying.
func equityDatasets(sym symbol.Symbol) []string {
	switch sym {
	case symbol.SPXW, symbol.RUTW, symbol.XSP, symbol.NDX,
		symbol.VIX, symbol.VIXW, symbol.SPEQX:
		return nil
	default:
		return []string{"EQUS.MINI"}
	}
}

func injectEquity(sym symbol.Symbol, dbnPath, dataset string) error {
	args := []string{
		"run", "./broker/databento/cmd/dbninject",
		"-file", dbnPath,
		"-dataset", dataset,
		"-sym", sym.String(),
		"-schema", "mbp-1",
	}
	log.Printf("exec: go %s", strings.Join(args, " "))
	cmd := exec.Command("go", args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dbninject: %w", err)
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// discoverDbnFiles finds all .dbn files under the configured data directory.
type DbnFile struct {
	Symbol symbol.Symbol
	Date   string
	Path   string
}

func discoverDbnFiles() []DbnFile {
	symbols := map[symbol.Symbol]bool{}
	for _, s := range parseSymbols() {
		symbols[s] = true
	}
	seen := map[string]bool{} // dedup "SYM/DATE" across dirs
	var files []DbnFile
	for _, datadir := range activeDirs() {
		entries, err := os.ReadDir(datadir)
		if err != nil {
			continue
		}
		for _, symDir := range entries {
			if !symDir.IsDir() {
				continue
			}
			sym, err := symbol.Parse(symDir.Name())
			if err != nil {
				continue
			}
			if !symbols[sym] {
				continue
			}
			symPath := filepath.Join(datadir, sym.String())
			dateEntries, err := os.ReadDir(symPath)
			if err != nil {
				continue
			}
			for _, de := range dateEntries {
				name := de.Name()
				if filepath.Ext(name) != ".dbn" {
					continue
				}
				date := name[:len(name)-4]
				key := sym.String() + "/" + date
				if seen[key] {
					continue
				}
				seen[key] = true
				files = append(files, DbnFile{
					Symbol: sym,
					Date:   date,
					Path:   filepath.Join(symPath, name),
				})
			}
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Date > files[j].Date
	})
	return files
}
