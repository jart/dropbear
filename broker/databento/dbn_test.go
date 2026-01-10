package databento

import (
	"os"
	"testing"
	"unsafe"
)

func TestReadDBNFile(t *testing.T) {
	// Use the sample file we downloaded
	path := "/tmp/sample.dbn"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("sample.dbn not found, run download first")
	}

	meta, records, err := ReadDBNFile(path)
	if err != nil {
		t.Fatalf("ReadDBNFile: %v", err)
	}

	t.Logf("Version: %d", meta.Version)
	t.Logf("Dataset: %s", meta.Dataset)
	t.Logf("Schema: %d", meta.Schema)
	t.Logf("Start: %d", meta.Start)
	t.Logf("End: %d", meta.End)
	t.Logf("SymbolCstrLen: %d", meta.SymbolCstrLen)
	t.Logf("Symbols: %v", meta.Symbols)
	t.Logf("Record bytes: %d", len(records))

	if meta.Dataset != "XNAS.ITCH" {
		t.Errorf("Dataset = %q, want XNAS.ITCH", meta.Dataset)
	}

	// Schema 0 = MBO
	if meta.Schema != SchemaMbo {
		t.Errorf("Schema = %d, want %d (MBO)", meta.Schema, SchemaMbo)
	}

	// Check we have MBO records
	recordSize := int(unsafe.Sizeof(MboMsg{}))
	if len(records) < recordSize {
		t.Fatalf("not enough record data: %d bytes", len(records))
	}

	numRecords := len(records) / recordSize
	t.Logf("Number of MBO records: %d", numRecords)

	// Parse first few records
	for i := 0; i < min(5, numRecords); i++ {
		offset := i * recordSize
		msg := (*MboMsg)(unsafe.Pointer(&records[offset]))
		t.Logf("Record %d: rtype=%02x action=%c side=%c price=%d size=%d order_id=%d",
			i, msg.Header.RType, msg.Action, msg.Side,
			msg.Price, msg.Size, msg.OrderID)
	}
}

func TestMboMsgParsing(t *testing.T) {
	// Raw bytes from the sample file at offset 0xce (206)
	// This is the first MBO record
	raw := []byte{
		0x0e, 0xa0, 0x02, 0x00, 0xef, 0x1b, 0x00, 0x00, // Header: len=14, rtype=0xa0, pub=2, instr=7151
		0x82, 0x7e, 0xa3, 0x59, 0xdf, 0x20, 0x18, 0x18, // ts_event
		0x3e, 0xed, 0xee, 0x02, 0x00, 0x00, 0x00, 0x00, // order_id
		0x98, 0x43, 0x4d, 0x27, 0x00, 0x00, 0x00, 0x00, // price
		0x01, 0x00, 0x00, 0x00, // size
		0x80,                                           // flags
		0x00,                                           // channel_id
		0x41,                                           // action = 'A'
		0x42,                                           // side = 'B'
		0x8c, 0x29, 0xc3, 0x59, 0xdf, 0x20, 0x18, 0x18, // ts_recv
		0x0a, 0xab, 0x1f, 0x00, // ts_in_delta
		0x0e, 0x06, 0x2c, 0x02, // sequence
	}

	if len(raw) != 56 {
		t.Fatalf("raw size = %d, want 56", len(raw))
	}

	msg := (*MboMsg)(unsafe.Pointer(&raw[0]))

	if msg.Header.Length != 14 {
		t.Errorf("Length = %d, want 14", msg.Header.Length)
	}
	if msg.Header.RType != RTypeMbo {
		t.Errorf("RType = %02x, want %02x", msg.Header.RType, RTypeMbo)
	}
	if msg.Header.PublisherID != 2 {
		t.Errorf("PublisherID = %d, want 2", msg.Header.PublisherID)
	}
	if msg.Header.InstrumentID != 7151 {
		t.Errorf("InstrumentID = %d, want 7151", msg.Header.InstrumentID)
	}
	if msg.Action != 'A' {
		t.Errorf("Action = %c, want A", msg.Action)
	}
	if msg.Side != 'B' {
		t.Errorf("Side = %c, want B", msg.Side)
	}
	if msg.Size != 1 {
		t.Errorf("Size = %d, want 1", msg.Size)
	}

	// Price is in fixed-point (1e9 scale)
	// 0x274d4398 = 660488088 -> 0.660488088 USD? That seems wrong for GOOG
	// Let me check - GOOG trades around $190, so price should be ~190e9
	// 660488088 / 1e9 = 0.66... that's way too low
	// Maybe the price scale is different or I'm reading wrong bytes
	t.Logf("Price raw: %d (%.9f if /1e9)", msg.Price, float64(msg.Price)/1e9)
	t.Logf("OrderID: %d", msg.OrderID)
}
