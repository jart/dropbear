// migrate-equitydata converts zstd-compressed equity candle files to mmap-friendly format.
//
// Usage:
//
//	go run ./cmd/migrate-equitydata
//
// Input:  ~/equitydata/minutes/<SYMBOL>/<YYYY-MM>  (zstd compressed)
// Output: ~/equitydata2/minutes/<SYMBOL>/<YYYY-MM> (raw 48-byte candles)
package main

import (
	"dropbear/indicators"
	"dropbear/loggy"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/klauspost/compress/zstd"
)

func main() {
	loggy.Init()

	srcBase := os.ExpandEnv("$HOME/equitydata/minutes")
	dstBase := os.ExpandEnv("$HOME/equitydata2/minutes")

	// List all symbols
	entries, err := os.ReadDir(srcBase)
	if err != nil {
		loggy.Fatalf("failed to read source directory: %v", err)
	}

	var symbols []string
	for _, entry := range entries {
		if entry.IsDir() {
			symbols = append(symbols, entry.Name())
		}
	}

	fmt.Printf("Found %d symbols to migrate\n", len(symbols))

	// Process symbols in parallel
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	var converted, skipped atomic.Int64

	for _, symbol := range symbols {
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			srcDir := filepath.Join(srcBase, symbol)
			dstDir := filepath.Join(dstBase, symbol)

			// Create destination directory
			if err := os.MkdirAll(dstDir, 0755); err != nil {
				fmt.Printf("ERROR: failed to create %s: %v\n", dstDir, err)
				return
			}

			// List monthly files
			files, err := os.ReadDir(srcDir)
			if err != nil {
				fmt.Printf("ERROR: failed to read %s: %v\n", srcDir, err)
				return
			}

			for _, file := range files {
				if file.IsDir() {
					continue
				}
				srcPath := filepath.Join(srcDir, file.Name())
				dstPath := filepath.Join(dstDir, file.Name())

				// Skip if destination exists and is newer
				srcInfo, err := os.Stat(srcPath)
				if err != nil {
					continue
				}
				if dstInfo, err := os.Stat(dstPath); err == nil {
					if dstInfo.ModTime().After(srcInfo.ModTime()) {
						skipped.Add(1)
						continue
					}
				}

				if err := convertFile(srcPath, dstPath); err != nil {
					fmt.Printf("ERROR: %s: %v\n", srcPath, err)
				} else {
					converted.Add(1)
				}
			}
		}(symbol)
	}

	wg.Wait()
	fmt.Printf("Converted %d files, skipped %d (already up to date)\n", converted.Load(), skipped.Load())
}

func convertFile(srcPath, dstPath string) error {
	// Open source file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Create zstd decoder
	decoder, err := zstd.NewReader(srcFile)
	if err != nil {
		return err
	}
	defer decoder.Close()

	// Read all candles
	var candles []indicators.Candle
	for {
		var c indicators.Candle
		err := c.Deserialize(decoder)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return fmt.Errorf("deserialize: %w", err)
		}
		candles = append(candles, c)
	}

	if len(candles) == 0 {
		return nil // Empty file, skip
	}

	// Write to destination as raw candles
	dstFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Encode all candles
	buf := make([]byte, 0, len(candles)*48)
	for i := range candles {
		buf = candles[i].Encode(buf)
	}

	_, err = dstFile.Write(buf)
	return err
}
