// Package symbols parses symbol list files with comments.
//
// The file format supports:
//   - One symbol per line
//   - Lines starting with # are comments
//   - Inline comments after # are stripped
//   - Empty lines and whitespace are ignored
//
// Example:
//
//	# these are justine's stock picks
//	GOOGL # better than treasury bonds
//	GLD   # shiny rocks
//	PM    # good beta synergy
package symbols

import "strings"

// Parse parses symbol list content and returns the symbols.
func Parse(content string) []string {
	var symbols []string
	for _, line := range strings.Split(content, "\n") {
		// strip inline comment
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		// extract symbol
		sym := strings.TrimSpace(line)
		if sym != "" {
			symbols = append(symbols, sym)
		}
	}
	return symbols
}
