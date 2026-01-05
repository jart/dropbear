package cubby

import (
	"dropbear/broker/alpaca"
	"dropbear/clocky"
	"errors"
	"os"
	"syscall"
	"unsafe"
)

// barSize is the size of a Bar in bytes.
const barSize = int(unsafe.Sizeof(alpaca.Bar{}))

// Bars provides zero-copy iteration over equity bar files.
type Bars struct {
	data  []byte
	count int
	pos   int
}

// openBars opens an equitydata file for reading.
// The file format is raw 48-byte bars with no length prefix.
// The returned bars is guaranteed to have at least one bar.
func OpenBars(path string) (*Bars, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size := int(info.Size())
	count := size / barSize
	if count == 0 || size%barSize != 0 {
		return nil, errors.New("invalid bar file size")
	}
	data, err := syscall.Mmap(int(file.Fd()), 0, size, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	// Tell the kernel we'll read sequentially
	return &Bars{
		data:  data,
		count: count,
	}, nil
}

// Close closes the bar file.
func (r *Bars) Close() error {
	err := syscall.Munmap(r.data)
	r.data = nil
	return err
}

// Count returns the number of bars in the file.
func (r *Bars) Count() int {
	return r.count
}

// Get returns a pointer to the bar at the given index.
func (r *Bars) Get(index int) *alpaca.Bar {
	return (*alpaca.Bar)(unsafe.Pointer(&r.data[index*barSize]))
}

// EOF returns whether we have reached the end of the file.
func (r *Bars) EOF() bool {
	return r.pos == r.count
}

// Read returns a pointer to the next bar in the file.
// Returns nil when there are no more bars.
// The returned pointer is only valid while the file is open.
func (r *Bars) Read() *alpaca.Bar {
	if r.pos == r.count {
		return nil
	}
	bar := r.Get(r.pos)
	r.pos++
	return bar
}

// Peek returns a pointer to the next bar in the file without advancing the position.
// Returns nil when there are no more bars.
// The returned pointer is only valid while the file is open.
func (r *Bars) Peek() *alpaca.Bar {
	if r.pos == r.count {
		return nil
	}
	bar := r.Get(r.pos)
	return bar
}

// Seek sets position to first bar >= target time.
func (r *Bars) Seek(target clocky.Time) {
	n := r.count
	lo, hi := 0, n
	for lo < hi {
		mid := (lo + hi) / 2
		bar := (*alpaca.Bar)(unsafe.Pointer(&r.data[mid*barSize]))
		if bar.Timestamp < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	r.pos = lo
}
