package ds

import (
	"dropbear/clocky"
	"encoding/binary"
	"fmt"
	"io"
)

// Tick represents market data that was received at once.
type Tick struct {
	Time   clocky.Time
	Snap   bool
	Bids   []Level
	Asks   []Level
	Trades []Trade
}

func (t Tick) Encode(b []byte) []byte {
	b = t.Time.Encode(b)
	if t.Snap {
		b = append(b, 1)
	} else {
		b = append(b, 0)
	}
	b = binary.LittleEndian.AppendUint32(b, uint32(len(t.Bids)))
	for _, level := range t.Bids {
		b = level.Encode(b)
	}
	b = binary.LittleEndian.AppendUint32(b, uint32(len(t.Asks)))
	for _, level := range t.Asks {
		b = level.Encode(b)
	}
	b = binary.LittleEndian.AppendUint32(b, uint32(len(t.Trades)))
	for _, trade := range t.Trades {
		b = trade.Encode(b)
	}
	return b
}

func (t *Tick) Serialize(writer io.Writer) error {
	data := t.Encode(nil)
	wrote, err := writer.Write(data)
	if err != nil {
		return err
	}
	if wrote != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

func (t *Tick) Deserialize(reader io.Reader) error {
	err := t.Time.Deserialize(reader)
	if err != nil {
		return err
	}
	t.Snap, err = deserializeBool(reader)
	if err != nil {
		return err
	}
	bidCount, err := deserializeUint32(reader)
	if err != nil {
		return err
	}
	t.Bids = growSlice(t.Bids, int(bidCount))
	for i := range bidCount {
		err = t.Bids[i].Deserialize(reader)
		if err != nil {
			return err
		}
	}
	askCount, err := deserializeUint32(reader)
	if err != nil {
		return err
	}
	t.Asks = growSlice(t.Asks, int(askCount))
	for i := range askCount {
		err = t.Asks[i].Deserialize(reader)
		if err != nil {
			return err
		}
	}
	tradeCount, err := deserializeUint32(reader)
	if err != nil {
		return err
	}
	t.Trades = growTrades(t.Trades, int(tradeCount))
	for i := range tradeCount {
		err = t.Trades[i].Deserialize(reader)
		if err != nil {
			return err
		}
	}
	return nil
}

func growSlice(s []Level, n int) []Level {
	if cap(s) >= n {
		return s[:n]
	}
	return make([]Level, n)
}

func growTrades(s []Trade, n int) []Trade {
	if cap(s) >= n {
		return s[:n]
	}
	return make([]Trade, n)
}

func deserializeBool(reader io.Reader) (bool, error) {
	var b [1]byte
	_, err := io.ReadFull(reader, b[:])
	if err != nil {
		return false, err
	}
	switch b[0] {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("failed to deserialize bool")
	}
}

func deserializeUint32(reader io.Reader) (uint32, error) {
	var b [4]byte
	_, err := io.ReadFull(reader, b[:])
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b[:]), nil
}
