package ds

import (
	"encoding/binary"
	"os"
	"syscall"
)

// TickReader provides zero-copy iteration over a coindata file.
// The file is memory-mapped for efficient access.
type TickReader struct {
	file   *os.File
	data   []byte
	offset int
}

// OpenTickReader opens a coindata file for reading.
// The file format is: [uint32 length][tick]... [uint32 0x00000000]
func OpenTickReader(path string) (*TickReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		file.Close()
		return &TickReader{}, nil
	}
	data, err := syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		file.Close()
		return nil, err
	}
	return &TickReader{
		file: file,
		data: data,
	}, nil
}

// Next returns the next tick in the file.
// Returns an empty Tick (IsZero() == true) when there are no more ticks.
func (r *TickReader) Next() Tick {
	if r.offset+4 > len(r.data) {
		return Tick{}
	}
	length := binary.LittleEndian.Uint32(r.data[r.offset : r.offset+4])
	if length == 0 {
		return Tick{} // EOF marker
	}
	start := r.offset + 4
	end := start + int(length)
	if end > len(r.data) {
		return Tick{} // Truncated record
	}
	r.offset = end
	return NewTick(r.data[start:end])
}

// Reset returns the reader to the beginning of the file.
func (r *TickReader) Reset() {
	r.offset = 0
}

// Close unmaps the file and closes it.
func (r *TickReader) Close() error {
	if r.data != nil {
		syscall.Munmap(r.data)
		r.data = nil
	}
	if r.file != nil {
		err := r.file.Close()
		r.file = nil
		return err
	}
	return nil
}
