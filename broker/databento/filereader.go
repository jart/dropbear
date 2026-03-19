// FileReader reads records from a DBN file on disk.
// It provides the same Read() interface as Client, so it can be used
// for backtesting with the same event loop that handles live data.
package databento

import (
	"bufio"
	"fmt"
	"io"
	"os"
)

// FileReader reads DBN records from a file.
type FileReader struct {
	file     *os.File
	reader   *bufio.Reader
	Metadata Metadata
}

// OpenFile opens a DBN file and reads its metadata header.
func OpenFile(path string) (*FileReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("databento: open %s: %w", path, err)
	}
	r := &FileReader{
		file:   f,
		reader: bufio.NewReaderSize(f, 256*1024),
	}
	if err := decodeMetadata(r.reader, &r.Metadata); err != nil {
		f.Close()
		return nil, fmt.Errorf("databento: read metadata from %s: %w", path, err)
	}
	return r, nil
}

// Read reads the next record from the file. Returns io.EOF at end of file.
func (r *FileReader) Read() (any, error) {
	rec, err := decodeRecord(r.reader)
	if err != nil {
		return nil, err
	}
	return castRecord(r.Metadata.Version, rec)
}

// Close closes the underlying file.
func (r *FileReader) Close() error {
	return r.file.Close()
}

// ReadAll reads all records from a DBN file and returns them as a slice.
func ReadAll(path string) (*Metadata, []any, error) {
	r, err := OpenFile(path)
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()
	var records []any
	for {
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				return &r.Metadata, records, nil
			}
			return &r.Metadata, records, err
		}
		records = append(records, rec)
	}
}
