package symbols

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "simple list",
			content: "AAPL\nGOOG\nMSFT\n",
			want:    []string{"AAPL", "GOOG", "MSFT"},
		},
		{
			name:    "with full line comments",
			content: "# stock picks\nAAPL\n# skipped\nGOOG\n",
			want:    []string{"AAPL", "GOOG"},
		},
		{
			name: "with inline comments",
			content: `# these are justine's stock picks
GOOGL # better than treasury bonds
GLD   # shiny rocks
PM    # good beta synergy
`,
			want: []string{"GOOGL", "GLD", "PM"},
		},
		{
			name:    "empty lines ignored",
			content: "AAPL\n\n\nGOOG\n\n",
			want:    []string{"AAPL", "GOOG"},
		},
		{
			name:    "whitespace trimmed",
			content: "  AAPL  \n\tGOOG\t\n",
			want:    []string{"AAPL", "GOOG"},
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
			want:    []string{"AAPL"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.content)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}
