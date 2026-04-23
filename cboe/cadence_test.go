package cboe

import (
	"dropbear/clocky"
	"dropbear/symbol"
	"testing"
	"time"
)

func TestGetNextOptionChain(t *testing.T) {
	tests := []struct {
		sym       symbol.Symbol
		now       clocky.Time
		wantChain clocky.Time
	}{
		// On 2026-04-22, the option chain expring 2026-04-24 was chosen, but should have chosen 0-DTE chain on the same day
		{symbol.GOOGL, clocky.Date(2026, clocky.April, 22, 13, 30, 0, 0, time.UTC), clocky.Date(2026, clocky.April, 22, 13, 30, 0, 0, time.UTC)},
		{symbol.SPXW, clocky.Date(2026, clocky.April, 22, 13, 30, 0, 0, time.UTC), clocky.Date(2026, clocky.April, 22, 13, 30, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		gotChainYear, gotChainMonth, gotChainDay := GetNextOptionChain(tt.sym, tt.now.Year(), tt.now.Month(), tt.now.Day())
		if gotChainYear != tt.wantChain.Year() || gotChainMonth != tt.wantChain.Month() || gotChainDay != tt.wantChain.Day() {
			t.Errorf("%s: GetNextOptionChain() got date = %d-%d-%d, want %d-%d-%d", tt.sym, gotChainYear, gotChainMonth, gotChainDay, tt.wantChain.Year(), tt.wantChain.Month(), tt.wantChain.Day())
		}
	}
}
