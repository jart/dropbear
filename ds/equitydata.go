package ds

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	equityDataDir     string
	equityDataDirOnce sync.Once
)

// EquityDataDir returns the path to the equitydata directory.
// It checks the following locations in order:
//
//   - ~/equitydata
//   - /fast/equitydata
//   - /disk/equitydata
//   - /usr/local/equitydata
//
// The result is cached after the first call.
// Creates ~/equitydata if none exists.
func EquityDataDir() string {
	equityDataDirOnce.Do(func() {
		defaultLocation := os.ExpandEnv("$HOME/equitydata")
		candidates := []string{
			defaultLocation,
			"/fast/equitydata",
			"/disk/equitydata",
			"/usr/local/equitydata",
		}
		for _, path := range candidates {
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				equityDataDir = path
				return
			}
		}
		os.Mkdir(defaultLocation, 0755)
		equityDataDir = defaultLocation
	})
	return equityDataDir
}

// EquityMinutesDir returns the path to the equity minutes data directory.
// Returns empty string if the equitydata directory is not found.
func EquityMinutesDir() string {
	return filepath.Join(EquityDataDir(), "minutes")
}
