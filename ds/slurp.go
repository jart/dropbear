package ds

import "os"

// MustSlurp reads an entire file into a string.
// It panics if the file cannot be read.
func MustSlurp(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(data)
}
