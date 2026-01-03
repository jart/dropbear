package ds

import (
	"os"
)

// TickWriter writes ticks to a coindata file.
// Format: [uint32 length][tick]... [uint32 0x00000000]
type TickWriter struct {
	file *os.File
}

// NewTickWriter creates a new tick writer for the given path.
// Creates the file if it doesn't exist, appends if it does.
func NewTickWriter(path string) (*TickWriter, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &TickWriter{file: file}, nil
}

// Write writes a pre-built tick record (from TickBuilder.Build).
func (w *TickWriter) Write(data []byte) error {
	_, err := w.file.Write(data)
	return err
}

// Close writes the EOF marker and closes the file.
func (w *TickWriter) Close() error {
	// Write EOF marker (4 zero bytes)
	var eof [4]byte
	if _, err := w.file.Write(eof[:]); err != nil {
		w.file.Close()
		return err
	}
	return w.file.Close()
}
