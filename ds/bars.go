package ds

import (
	"dropbear/clocky"
	"errors"
	"os"
	"syscall"
	"unsafe"
)

// Bars provides zero-copy iteration over equity bar files.
type Bars struct {
	path  string
	data  []byte
	count int
	pos   int
}

// OpenBars opens an equitydata file for reading.
// The file format is raw 64-byte bars with no length prefix.
// The returned Bars is guaranteed to have at least one bar.
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
	return &Bars{
		path:  path,
		data:  data,
		count: count,
	}, nil
}

// String returns a string representation of object.
func (r *Bars) String() string {
	return "Bars{" + r.path + "}"
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
func (r *Bars) Get(index int) *Bar {
	return (*Bar)(unsafe.Pointer(&r.data[index*barSize]))
}

// EOF returns whether we have reached the end of the file.
func (r *Bars) EOF() bool {
	return r.pos == r.count
}

// Read returns a pointer to the next bar in the file.
// Returns nil when there are no more bars.
func (r *Bars) Read() *Bar {
	if r.pos == r.count {
		return nil
	}
	bar := r.Get(r.pos)
	r.pos++
	return bar
}

// Peek returns a pointer to the next bar without advancing the position.
// Returns nil when there are no more bars.
func (r *Bars) Peek() *Bar {
	if r.pos == r.count {
		return nil
	}
	return r.Get(r.pos)
}

// Seek sets position to first bar >= target time.
func (r *Bars) Seek(target clocky.Time) {
	lo, hi := 0, r.count
	for lo < hi {
		mid := (lo + hi) / 2
		bar := (*Bar)(unsafe.Pointer(&r.data[mid*barSize]))
		if bar.Timestamp < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	r.pos = lo
}

const barSize = int(unsafe.Sizeof(Bar{}))
