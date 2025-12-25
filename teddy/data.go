package teddy

import (
	"dropbear/ds"
	"dropbear/loggy"
	"os"

	"github.com/klauspost/compress/zstd"
)

type recordedMarketData struct {
	file   *os.File
	reader *zstd.Decoder
}

func openRecordedMarketData(name string, exchange ds.Exchange, symbol string) *recordedMarketData {
	home := os.Getenv("HOME")
	path := home + "/marketdata/" + name + "/" + exchange.String() + "/" + symbol
	file, err := os.Open(path)
	if err != nil {
		loggy.Fatalf("failed to open market data file: %s: %v", path, err)
	}
	reader, err := zstd.NewReader(file)
	if err != nil {
		loggy.Fatalf("failed to create zstd reader: %v", err)
	}
	return &recordedMarketData{
		file:   file,
		reader: reader,
	}
}

func (m *recordedMarketData) Read(tick *ds.Tick) error {
	return tick.Deserialize(m.reader)
}

func (m *recordedMarketData) Close() {
	m.reader.Close()
	m.file.Close()
}
