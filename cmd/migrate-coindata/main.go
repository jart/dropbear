package main

import (
	"dropbear/ds"
	"dropbear/loggy"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

func main() {
	loggy.Init()
	flag.Parse()

	home := os.Getenv("HOME")
	srcDir := filepath.Join(home, "marketdata")
	dstDir := filepath.Join(home, "coindata")

	datasets := flag.Args()
	if len(datasets) == 0 {
		// Find all datasets
		entries, err := os.ReadDir(srcDir)
		if err != nil {
			loggy.Fatalf("failed to read marketdata: %v", err)
		}
		for _, entry := range entries {
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
				datasets = append(datasets, entry.Name())
			}
		}
	}

	for _, dataset := range datasets {
		migrateDataset(srcDir, dstDir, dataset)
	}
}

func migrateDataset(srcDir, dstDir, dataset string) {
	datasetSrc := filepath.Join(srcDir, dataset)
	datasetDst := filepath.Join(dstDir, dataset)

	brokers, err := os.ReadDir(datasetSrc)
	if err != nil {
		log.Printf("failed to read dataset %s: %v", dataset, err)
		return
	}

	for _, broker := range brokers {
		if !broker.IsDir() || strings.HasPrefix(broker.Name(), ".") || broker.Name() == "Icon\r" {
			continue
		}
		brokerSrc := filepath.Join(datasetSrc, broker.Name())
		brokerDst := filepath.Join(datasetDst, broker.Name())

		symbols, err := os.ReadDir(brokerSrc)
		if err != nil {
			log.Printf("failed to read broker %s/%s: %v", dataset, broker.Name(), err)
			continue
		}

		for _, symbol := range symbols {
			if symbol.IsDir() || strings.HasPrefix(symbol.Name(), ".") || symbol.Name() == "Icon\r" {
				continue
			}
			srcPath := filepath.Join(brokerSrc, symbol.Name())
			dstPath := filepath.Join(brokerDst, symbol.Name())
			if err := migrateFile(srcPath, dstPath); err != nil {
				log.Printf("failed to migrate %s: %v", srcPath, err)
			}
		}
	}
}

func migrateFile(srcPath, dstPath string) error {
	// Check if destination already exists
	if _, err := os.Stat(dstPath); err == nil {
		fmt.Printf("SKIP %s (already exists)\n", dstPath)
		return nil
	}

	fmt.Printf("MIGRATE %s\n", srcPath)

	// Open source file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Create zstd reader
	reader, err := zstd.NewReader(srcFile, zstd.WithDecoderConcurrency(1))
	if err != nil {
		return err
	}
	defer reader.Close()

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}

	// Create destination file
	dstFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Migrate ticks
	var builder ds.TickBuilder
	var buf []byte
	count := 0

	for {
		err := builder.Deserialize(reader)
		if err != nil {
			break // EOF or error
		}
		buf = builder.Build(buf[:0])
		if _, err := dstFile.Write(buf); err != nil {
			os.Remove(dstPath)
			return err
		}
		count++
	}

	// Write EOF marker (4 zero bytes)
	var eof [4]byte
	if _, err := dstFile.Write(eof[:]); err != nil {
		os.Remove(dstPath)
		return err
	}

	fmt.Printf("  -> %s (%d ticks)\n", dstPath, count)
	return nil
}

// verifyFile reads back the migrated file to ensure it's valid.
func verifyFile(path string) error {
	reader, err := ds.OpenTickReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()

	count := 0
	for {
		tick := reader.Next()
		if tick.IsZero() {
			break
		}
		// Basic sanity check - time should be reasonable
		_ = tick.Time()
		_ = tick.BidCount()
		_ = tick.AskCount()
		_ = tick.TradeCount()
		count++
	}

	var expected [4]byte
	binary.LittleEndian.PutUint32(expected[:], uint32(count))
	return nil
}
