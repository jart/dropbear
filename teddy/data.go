package teddy

import (
	"dropbear/ds"
	"dropbear/loggy"
	"fmt"
	"os"
	"path"
)

// marketDataSource provides tick data for backtesting.
type marketDataSource interface {
	// Next returns the next tick. Returns an empty tick (IsZero) at EOF.
	Next() ds.Tick
	Close()
}

// openMarketData opens a market data file.
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
