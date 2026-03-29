package symbol

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestExpand(t *testing.T) {
	// create temp file with symbols
	dir := t.TempDir()
	path := filepath.Join(dir, "picks.conf")
	content := `# my picks
AAPL # good company
GOOG
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		flagValue string
		want      []Symbol
		broken    bool
	}{
		{
			name:      "plain symbols",
			flagValue: "AAPL GOOG MSFT",
			want:      []Symbol{AAPL, GOOG, MSFT},
		},
		{
			name:      "file path",
			flagValue: path,
			want:      []Symbol{AAPL, GOOG},
		},
		{
			name:      "mixed symbols and file",
			flagValue: "MSFT " + path + " NVDA",
			want:      []Symbol{MSFT, AAPL, GOOG, NVDA},
		},
		{
			name:      "empty",
			flagValue: "",
			want:      nil,
		},
		{
			name:      "nonexistent file treated as symbol",
			flagValue: "/nonexistent/path",
			broken:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Expand(tt.flagValue)
			if err != nil && !tt.broken {
				t.Fatalf("Expand() error: %v", err)
			}
			if tt.broken && err == nil {
				t.Fatalf("Expand() expected error but got nil")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Expand() = %v, want %v", got, tt.want)
			}
		})
	}
}
