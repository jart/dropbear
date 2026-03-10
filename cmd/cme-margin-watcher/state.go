package main

import (
	"encoding/json"
	"log"
	"os"
	"sync"
)

// State persists across restarts so we don't re-alert on old data.
type State struct {
	mu sync.Mutex

	// Advisory RSS: GUIDs we've already fired on
	SeenAdvisories map[string]bool `json:"seen_advisories"`

	// SPAN: last known price scan range per product code (raw integer from file)
	SPANScanRanges map[string]int64 `json:"span_scan_ranges"`

	// Margins API: last known maintenance margin string per product code
	MaintMargins map[string]string `json:"maint_margins"`

	filename string
}

func loadState(filename string) *State {
	s := &State{
		SeenAdvisories: make(map[string]bool),
		SPANScanRanges: make(map[string]int64),
		MaintMargins:   make(map[string]string),
		filename:       filename,
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return s
	}
	if err := json.Unmarshal(data, s); err != nil {
		log.Printf("warn: could not parse state file %s: %v", filename, err)
	}
	// Re-set maps in case JSON had nulls
	if s.SeenAdvisories == nil {
		s.SeenAdvisories = make(map[string]bool)
	}
	if s.SPANScanRanges == nil {
		s.SPANScanRanges = make(map[string]int64)
	}
	if s.MaintMargins == nil {
		s.MaintMargins = make(map[string]string)
	}
	return s
}

func (s *State) save() {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := json.MarshalIndent(s, "", "  ")
	if err := os.WriteFile(s.filename, data, 0644); err != nil {
		log.Printf("warn: could not save state: %v", err)
	}
}

func (s *State) hasSeenAdvisory(guid string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.SeenAdvisories[guid]
}

func (s *State) markAdvisorySeen(guid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SeenAdvisories[guid] = true
}

// updateSPAN returns (changed, oldVal). If oldVal==0, it's first time seen (no alert).
func (s *State) updateSPAN(product string, newVal int64) (changed bool, oldVal int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	oldVal = s.SPANScanRanges[product]
	changed = oldVal != 0 && oldVal != newVal
	s.SPANScanRanges[product] = newVal
	return
}

// updateMargin returns (changed, oldVal). Empty oldVal means first time seen (no alert).
func (s *State) updateMargin(product string, newVal string) (changed bool, oldVal string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	oldVal = s.MaintMargins[product]
	changed = oldVal != "" && oldVal != newVal
	s.MaintMargins[product] = newVal
	return
}
