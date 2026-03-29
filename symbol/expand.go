package symbol

import (
	"os"
	"strings"
)

// Expand expands a whitespace-separated list of symbols and/or file paths.
// If an item is a path to an existing file, its contents are parsed for symbols.
// Otherwise, the item is used as a symbol directly.
func Expand(flagValue string) ([]Symbol, error) {
	var result []Symbol
	for s := range strings.FieldsSeq(flagValue) {
		if info, err := os.Stat(s); err == nil && !info.IsDir() {
			data, err := os.ReadFile(s)
			if err != nil {
				return nil, err
			}
			symbols, err := ParseFile(string(data))
			if err != nil {
				return nil, err
			}
			result = append(result, symbols...)
		} else {
			symbol, err := Parse(s)
			if err != nil {
				return nil, err
			}
			result = append(result, symbol)
		}
	}
	return result, nil
}
