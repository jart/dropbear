package teddy

import (
	"dropbear/ds"
	"dropbear/loggy"
	"fmt"
	"os"
	"path"

	"github.com/klauspost/compress/zstd"
)

// marketDataSource provides tick data for backtesting.
type marketDataSource interface {
	// Next returns the next tick. Returns an empty tick (IsZero) at EOF.
	Next() ds.Tick
	Close()
}

// openMarketData opens a market data file, auto-detecting format.
func openMarketData(name string, broker ds.Broker, symbol string) marketDataSource {
	for _, dir := range []string{os.ExpandEnv("$HOME/coindata"), "/fast/coindata", "/usr/local/coindata"} {
		coinPath := path.Join(dir, name, broker.String(), symbol)
		if _, err := os.Stat(coinPath); err == nil {
			reader, err := ds.OpenTickReader(coinPath)
			if err != nil {
				loggy.Fatalf("failed to open coindata file: %s: %v", coinPath, err)
			}
			return &coindataSource{reader: reader}
		}
	}
	panic(fmt.Sprintf("coindata file not found for %s/%s", broker.String(), symbol))
}

// coindataSource wraps TickReader for the marketDataSource interface.
type coindataSource struct {
	reader *ds.TickReader
}

func (c *coindataSource) Next() ds.Tick {
	return c.reader.Next()
}

func (c *coindataSource) Close() {
	c.reader.Close()
}

// recordedMarketData reads legacy zstd-compressed tick files.
type recordedMarketData struct {
	file    *os.File
	reader  *zstd.Decoder
	builder ds.TickBuilder
	buf     []byte
}

func openRecordedMarketData(path string) *recordedMarketData {
	file, err := os.Open(path)
	if err != nil {
		loggy.Fatalf("failed to open market data file: %s: %v", path, err)
	}
	reader, err := zstd.NewReader(file, zstd.WithDecoderConcurrency(1))
	if err != nil {
		loggy.Fatalf("failed to create zstd reader: %v", err)
	}
	return &recordedMarketData{
		file:   file,
		reader: reader,
	}
}

func (m *recordedMarketData) Next() ds.Tick {
	err := m.builder.Deserialize(m.reader)
	if err != nil {
		return ds.Tick{} // EOF
	}
	// Convert builder to Tick view
	// Note: we reuse buf, so the returned Tick is only valid until next call
	m.buf = m.buf[:0]
	tick, buf := m.builder.ToTick(m.buf)
	m.buf = buf
	return tick
}

func (m *recordedMarketData) Close() {
	m.reader.Close()
	m.file.Close()
}
