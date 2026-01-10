package databento

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// MboFile is a memory-mapped file of MBO records.
type MboFile struct {
	data    []byte
	Records []MboMsg
}

// OpenMboFile opens an mmap'd MBO file for reading.
func OpenMboFile(path string) (*MboFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	size := fi.Size()
	recordSize := int64(unsafe.Sizeof(MboMsg{}))
	if size%recordSize != 0 {
		return nil, fmt.Errorf("file size %d not divisible by record size %d", size, recordSize)
	}

	data, err := unix.Mmap(int(f.Fd()), 0, int(size), unix.PROT_READ, unix.MAP_PRIVATE)
	if err != nil {
		return nil, fmt.Errorf("mmap: %w", err)
	}

	numRecords := size / recordSize
	records := unsafe.Slice((*MboMsg)(unsafe.Pointer(&data[0])), numRecords)

	return &MboFile{
		data:    data,
		Records: records,
	}, nil
}

// Close unmaps the file.
func (m *MboFile) Close() error {
	if m.data != nil {
		return unix.Munmap(m.data)
	}
	return nil
}

// Len returns the number of records.
func (m *MboFile) Len() int {
	return len(m.Records)
}
