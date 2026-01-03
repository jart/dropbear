package ds

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"encoding/binary"
	"fmt"
	"io"
	"unsafe"
)

// Tick is a zero-copy view into serialized market data.
// The underlying data must remain valid for the lifetime of the Tick.
// All bounds checking is done once in NewTick; accessors use unsafe reads.
type Tick struct {
	ptr      unsafe.Pointer // base pointer for fast access
	data     []byte         // keeps GC reference
	askOff   int32          // cached offset to ask count field
	tradeOff int32          // cached offset to trade count field
}

// NewTick creates a Tick view from serialized data.
// The data should be the tick payload (not including the length prefix).
// All bounds are validated here; accessors are unchecked for speed.
func NewTick(data []byte) Tick {
	n := len(data)
	if n < 13 {
		return Tick{}
	}
	ptr := unsafe.Pointer(&data[0])
	bidCount := int(*(*uint32)(unsafe.Add(ptr, 9)))
	askOff := 13 + bidCount*16
	if n < askOff+4 {
		return Tick{}
	}
	askCount := int(*(*uint32)(unsafe.Add(ptr, askOff)))
	tradeOff := askOff + 4 + askCount*16
	if n < tradeOff+4 {
		return Tick{}
	}
	// Validate trade data bounds
	tradeCount := int(*(*uint32)(unsafe.Add(ptr, tradeOff)))
	if n < tradeOff+4+tradeCount*25 {
		return Tick{}
	}
	return Tick{ptr: ptr, data: data, askOff: int32(askOff), tradeOff: int32(tradeOff)}
}

// IsZero returns true if this is an empty/uninitialized tick.
func (t Tick) IsZero() bool {
	return t.ptr == nil
}

// Time returns the timestamp of this tick.
func (t Tick) Time() clocky.Time {
	return clocky.Time(*(*uint64)(t.ptr))
}

// Snap returns true if this is a full snapshot (vs incremental update).
func (t Tick) Snap() bool {
	return *(*byte)(unsafe.Add(t.ptr, 8)) == 1
}

// BidCount returns the number of bid levels.
func (t Tick) BidCount() int {
	return int(*(*uint32)(unsafe.Add(t.ptr, 9)))
}

// Bid returns the bid level at index i.
func (t Tick) Bid(i int) Level {
	p := unsafe.Add(t.ptr, 13+i*16)
	return Level{
		Price: decimal.Decimal(*(*uint64)(p)),
		Size:  decimal.Decimal(*(*uint64)(unsafe.Add(p, 8))),
	}
}

// AskCount returns the number of ask levels.
func (t Tick) AskCount() int {
	return int(*(*uint32)(unsafe.Add(t.ptr, int(t.askOff))))
}

// Ask returns the ask level at index i.
func (t Tick) Ask(i int) Level {
	p := unsafe.Add(t.ptr, int(t.askOff)+4+i*16)
	return Level{
		Price: decimal.Decimal(*(*uint64)(p)),
		Size:  decimal.Decimal(*(*uint64)(unsafe.Add(p, 8))),
	}
}

// TradeCount returns the number of trades.
func (t Tick) TradeCount() int {
	return int(*(*uint32)(unsafe.Add(t.ptr, int(t.tradeOff))))
}

// Trade returns the trade at index i.
func (t Tick) Trade(i int) Trade {
	p := unsafe.Add(t.ptr, int(t.tradeOff)+4+i*25)
	var side Side
	if *(*byte)(p) == 0 {
		side = SideBuy
	} else {
		side = SideSell
	}
	return Trade{
		Side:     side,
		Time:     clocky.Time(*(*uint64)(unsafe.Add(p, 1))),
		Price:    decimal.Decimal(*(*uint64)(unsafe.Add(p, 9))),
		Quantity: decimal.Decimal(*(*uint64)(unsafe.Add(p, 17))),
	}
}

// HasL2 returns true if this tick contains any bid or ask data.
func (t Tick) HasL2() bool {
	return t.askOff > 13 || t.AskCount() > 0
}

// HasTrades returns true if this tick contains any trades.
func (t Tick) HasTrades() bool {
	return t.TradeCount() > 0
}

// Bytes returns the underlying serialized data.
func (t Tick) Bytes() []byte {
	return t.data
}

// Serialize writes the tick payload to a writer (legacy format, no length prefix).
func (t Tick) Serialize(w io.Writer) error {
	_, err := w.Write(t.data)
	return err
}

// TickBuilder constructs ticks for serialization.
// Used by broker streams to build ticks from live market data.
type TickBuilder struct {
	Time   clocky.Time
	Snap   bool
	Bids   []Level
	Asks   []Level
	Trades []Trade
}

// Reset clears the builder for reuse.
func (b *TickBuilder) Reset() {
	b.Time = 0
	b.Snap = false
	b.Bids = b.Bids[:0]
	b.Asks = b.Asks[:0]
	b.Trades = b.Trades[:0]
}

// Encode appends the tick payload to buf and returns the extended buffer.
// This is the payload only, without a length prefix.
func (b *TickBuilder) Encode(buf []byte) []byte {
	buf = b.Time.Encode(buf)
	if b.Snap {
		buf = append(buf, 1)
	} else {
		buf = append(buf, 0)
	}
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(b.Bids)))
	for _, level := range b.Bids {
		buf = level.Encode(buf)
	}
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(b.Asks)))
	for _, level := range b.Asks {
		buf = level.Encode(buf)
	}
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(b.Trades)))
	for _, trade := range b.Trades {
		buf = trade.Encode(buf)
	}
	return buf
}

// Build appends a length-prefixed tick record to buf and returns it.
// Format: [uint32 length][tick payload]
func (b *TickBuilder) Build(buf []byte) []byte {
	// Reserve space for length prefix
	start := len(buf)
	buf = append(buf, 0, 0, 0, 0)
	// Encode payload
	buf = b.Encode(buf)
	// Write length (payload size only, not including the 4-byte length field)
	payloadLen := len(buf) - start - 4
	binary.LittleEndian.PutUint32(buf[start:], uint32(payloadLen))
	return buf
}

// Serialize writes the tick to a writer using the legacy format (no length prefix).
func (b *TickBuilder) Serialize(writer io.Writer) error {
	data := b.Encode(nil)
	wrote, err := writer.Write(data)
	if err != nil {
		return err
	}
	if wrote != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

// Deserialize reads a tick from a reader using the legacy format (no length prefix).
// This populates the builder from serialized data.
func (b *TickBuilder) Deserialize(reader io.Reader) error {
	err := b.Time.Deserialize(reader)
	if err != nil {
		return err
	}
	b.Snap, err = deserializeBool(reader)
	if err != nil {
		return err
	}
	bidCount, err := deserializeUint32(reader)
	if err != nil {
		return err
	}
	b.Bids = growSlice(b.Bids, int(bidCount))
	for i := range bidCount {
		err = b.Bids[i].Deserialize(reader)
		if err != nil {
			return err
		}
	}
	askCount, err := deserializeUint32(reader)
	if err != nil {
		return err
	}
	b.Asks = growSlice(b.Asks, int(askCount))
	for i := range askCount {
		err = b.Asks[i].Deserialize(reader)
		if err != nil {
			return err
		}
	}
	tradeCount, err := deserializeUint32(reader)
	if err != nil {
		return err
	}
	b.Trades = growTrades(b.Trades, int(tradeCount))
	for i := range tradeCount {
		err = b.Trades[i].Deserialize(reader)
		if err != nil {
			return err
		}
	}
	return nil
}

// ToTick converts the builder to a Tick view.
// The returned Tick is only valid while buf remains unchanged.
func (b *TickBuilder) ToTick(buf []byte) (Tick, []byte) {
	start := len(buf)
	buf = b.Encode(buf)
	return NewTick(buf[start:]), buf
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
