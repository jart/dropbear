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
package symbol

import "strings"

// ParseFile parses symbol list content and returns the symbols.
func ParseFile(content string) ([]Symbol, error) {
	var symbols []Symbol
	for line := range strings.SplitSeq(content, "\n") {
		// strip inline comment
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		// extract symbol
		sym := strings.TrimSpace(line)
		if sym != "" {
			parsedSym, err := Parse(sym)
			if err != nil {
				return nil, err
			}
			symbols = append(symbols, parsedSym)
		}
	}
	return symbols, nil
}
