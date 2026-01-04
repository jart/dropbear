package indicators

import (
	"dropbear/clocky"
	"errors"
	"os"
	"syscall"
	"unsafe"
)

// candleSize is the size of a Candle in bytes (6 x int64 = 48 bytes).
const candleSize = 48

// candleReader provides zero-copy iteration over equity candle files.
// Files are memory-mapped for efficient access.
type CandleFile struct {
	data  []byte
	count int
	pos   int
}

// openCandleFile opens an equitydata file for reading.
// The file format is raw 48-byte candles with no length prefix.
// The returned candleFile is guaranteed to have at least one candle.
func OpenCandleFile(path string) (*CandleFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	count := int(size / candleSize)
	if count == 0 || size%candleSize != 0 {
		return nil, errors.New("invalid candle file size")
	}
	data, err := syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	// Tell the kernel we'll read sequentially
	return &CandleFile{
		data:  data,
		count: count,
	}, nil
}

// Close closes the candle file.
func (r *CandleFile) Close() error {
	err := syscall.Munmap(r.data)
	r.data = nil
	return err
}

// Count returns the number of candles in the file.
func (r *CandleFile) Count() int {
	return r.count
}

// Get returns a pointer to the candle at the given index.
func (r *CandleFile) Get(index int) *Candle {
	return (*Candle)(unsafe.Pointer(&r.data[index*candleSize]))
}

// EOF returns whether we have reached the end of the file.
func (r *CandleFile) EOF() bool {
	return r.pos == r.count
}

// Read returns a pointer to the next candle in the file.
// Returns nil when there are no more candles.
// The returned pointer is only valid while the file is open.
func (r *CandleFile) Read() *Candle {
	if r.pos == r.count {
		return nil
	}
	candle := r.Get(r.pos)
	r.pos++
	return candle
}

// Peek returns a pointer to the next candle in the file without advancing the position.
// Returns nil when there are no more candles.
// The returned pointer is only valid while the file is open.
func (r *CandleFile) Peek() *Candle {
	if r.pos == r.count {
		return nil
	}
	candle := r.Get(r.pos)
	return candle
}

// Seek sets position to first candle >= target time.
func (r *CandleFile) Seek(target clocky.Time) {
	n := r.count
	lo, hi := 0, n
	for lo < hi {
		mid := (lo + hi) / 2
		candle := (*Candle)(unsafe.Pointer(&r.data[mid*candleSize]))
		if candle.Start < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	r.pos = lo
}
