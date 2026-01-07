package symbols

import (
	"os"
	"strings"
)

// Expand expands a whitespace-separated list of symbols and/or file paths.
// If an item is a path to an existing file, its contents are parsed for symbols.
// Otherwise, the item is used as a symbol directly.
func Expand(flagValue string) []string {
	var result []string
	for _, s := range strings.Fields(flagValue) {
		if info, err := os.Stat(s); err == nil && !info.IsDir() {
			data, err := os.ReadFile(s)
			if err != nil {
				panic(err)
			}
			result = append(result, Parse(string(data))...)
		} else {
			result = append(result, s)
		}
	}
	return result
}
