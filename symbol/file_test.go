package symbol

import (
	"reflect"
	"testing"
)

func TestParseFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []Symbol
	}{
		{
			name:    "simple list",
			content: "AAPL\nGOOG\nMSFT\n",
			want:    []Symbol{AAPL, GOOG, MSFT},
		},
		{
			name:    "with full line comments",
			content: "# stock picks\nAAPL\n# skipped\nGOOG\n",
			want:    []Symbol{AAPL, GOOG},
		},
		{
			name: "with inline comments",
			content: `# these are justine's stock picks
GOOGL # better than treasury bonds
GLD   # shiny rocks
PM    # good beta synergy
`,
			want: []Symbol{GOOGL, GLD, PM},
		},
		{
			name:    "empty lines ignored",
			content: "AAPL\n\n\nGOOG\n\n",
			want:    []Symbol{AAPL, GOOG},
		},
		{
			name:    "whitespace trimmed",
			content: "  AAPL  \n\tGOOG\t\n",
			want:    []Symbol{AAPL, GOOG},
		},
		{
			name:    "empty file",
			content: "",
			want:    nil,
		},
		{
			name:    "only comments",
			content: "# comment 1\n# comment 2\n",
			want:    nil,
		},
		{
			name:    "comment hash in middle",
			content: "AAPL#inline\n",
			want:    []Symbol{AAPL},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFile(tt.content)
			if err != nil {
				t.Fatalf("ParseFile() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseFile() = %v, want %v", got, tt.want)
			}
		})
	}
}
