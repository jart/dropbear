// FileReader reads records from a DBN file on disk.
// It provides the same Read() interface as Client, so it can be used
// for backtesting with the same event loop that handles live data.
package databento

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"syscall"
)

// FileReader reads DBN records from a file.
type FileReader struct {
	off      int64
	data     []byte
	Metadata Metadata
}

// OpenFile opens a DBN file and reads its metadata header.
func OpenFile(path string) (*FileReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("databento: open %s: %w", path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := int(info.Size())
	data, err := syscall.Mmap(int(f.Fd()), 0, size, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	r := &FileReader{data: data}
	reader := bytes.NewReader(data)
	if err := decodeMetadata(reader, &r.Metadata); err != nil {
		syscall.Munmap(data)
		return nil, fmt.Errorf("databento: read metadata from %s: %w", path, err)
	}
	r.off, err = reader.Seek(0, io.SeekCurrent)
	if err != nil {
		panic("not possible")
	}
	return r, nil
}

// Read reads the next record from the file. Returns io.EOF at end of file.
// You can't modify the returned record, since it points to read-only memory.
func (r *FileReader) Read() (any, error) {
	if r.off == int64(len(r.data)) {
		return nil, io.EOF
	}
	n := int64(r.data[r.off]) * 4
	if n < 16 {
		return nil, fmt.Errorf("dbn: record size %d too small (min 16)", n)
	}
	record := r.data[r.off : r.off+n]
	r.off += n
	return castRecord(r.Metadata.Version, record)
}

// Close closes the underlying file.
// This will destroy objects returned by Read, since they point to the memory-mapped file.
func (r *FileReader) Close() error {
	err := syscall.Munmap(r.data)
	r.data = nil
	return err
}
