package cubby

import (
	"dropbear/clocky"
	"dropbear/indicators"
	"os"
	"syscall"
	"unsafe"
)

// candleSize is the size of a Candle in bytes (6 x int64 = 48 bytes).
const candleSize = 48

// candleReader provides zero-copy iteration over equity candle files.
// Files are memory-mapped for efficient access.
type candleReader struct {
	data   []byte
	offset int
}

// openCandleReader opens an equitydata file for reading.
// The file format is raw 48-byte candles with no length prefix.
func openCandleReader(path string) (*candleReader, error) {
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
	if size == 0 {
		return &candleReader{}, nil
	}
	data, err := syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	// Tell the kernel we'll read sequentially
	return &candleReader{
		data: data,
	}, nil
}

// Next returns a pointer to the next candle in the file.
// Returns nil when there are no more candles.
// The returned pointer is only valid while the file is open.
func (r *candleReader) Next() *indicators.Candle {
	if r.offset+candleSize > len(r.data) {
		return nil
	}
	candle := (*indicators.Candle)(unsafe.Pointer(&r.data[r.offset]))
	r.offset += candleSize
	return candle
}

// SeekTo uses binary search to find the first candle >= target time.
func (r *candleReader) SeekTo(target clocky.Time) {
	n := len(r.data) / candleSize
	if n == 0 {
		return
	}
	// binary search for first candle >= target
	lo, hi := 0, n
	for lo < hi {
		mid := (lo + hi) / 2
		candle := (*indicators.Candle)(unsafe.Pointer(&r.data[mid*candleSize]))
		if candle.Start < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	r.offset = lo * candleSize
}

// Close closes the candle reader.
func (r *candleReader) Close() error {
	err := syscall.Munmap(r.data)
	r.data = nil
	return err
}
