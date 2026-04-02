package main

import (
	"database/sql"
	"dropbear/db"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Experiment struct {
	Name string
	Path string
	DB   *sql.DB
}

type ExperimentInfo struct {
	Name    string
	ModTime time.Time
}

func varulabDir() string {
	dir := os.ExpandEnv("$HOME/.varulab")
	os.MkdirAll(dir, 0755)
	return dir
}

func openExperiment(name string) (*Experiment, error) {
	path := filepath.Join(varulabDir(), name+".sqlite3")
	database, err := db.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open experiment %s: %w", name, err)
	}
	if err := Migrate(database); err != nil {
		database.Close()
		return nil, fmt.Errorf("migrate experiment %s: %w", name, err)
	}
	return &Experiment{Name: name, Path: path, DB: database}, nil
}

func listExperiments() []ExperimentInfo {
	matches, _ := filepath.Glob(filepath.Join(varulabDir(), "*.sqlite3"))
	var infos []ExperimentInfo
	for _, path := range matches {
		name := strings.TrimSuffix(filepath.Base(path), ".sqlite3")
		info := ExperimentInfo{Name: name}
		if fi, err := os.Stat(path); err == nil {
			info.ModTime = fi.ModTime()
		}
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].ModTime.After(infos[j].ModTime)
	})
	return infos
}

func renameExperiment(oldName, newName string) error {
	dir := varulabDir()
	oldPath := filepath.Join(dir, oldName+".sqlite3")
	newPath := filepath.Join(dir, newName+".sqlite3")
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("experiment %s already exists", newName)
	}
	return os.Rename(oldPath, newPath)
}

func deleteExperiment(name string) error {
	path := filepath.Join(varulabDir(), name+".sqlite3")
	return os.Remove(path)
}

func newExperimentName() string {
	return time.Now().Format("2006-01-02T150405")
}

func latestExperimentName() (string, error) {
	matches, _ := filepath.Glob(filepath.Join(varulabDir(), "*.sqlite3"))
	if len(matches) == 0 {
		return "", fmt.Errorf("no experiments found in %s", varulabDir())
	}
	var bestPath string
	var bestTime time.Time
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if bestPath == "" || info.ModTime().After(bestTime) {
			bestPath = path
			bestTime = info.ModTime()
		}
	}
	return strings.TrimSuffix(filepath.Base(bestPath), ".sqlite3"), nil
}
