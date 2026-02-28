package databento

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
	"time"
	"unsafe"

	"dropbear/clocky"
)

// TestDecodeMetadata constructs a minimal valid DBN v3 metadata header and
// verifies all fields are parsed correctly.
func TestDecodeMetadata(t *testing.T) {
	var buf bytes.Buffer

	// Build fixed metadata frame
	var frame bytes.Buffer
	symLen := uint16(SymbolCstrLen) // 71

	// dataset (16 bytes, null-padded)
	ds := [DatasetLen]byte{}
	copy(ds[:], "OPRA.PILLAR")
	frame.Write(ds[:])

	// schema (uint16) = Definition (9)
	binary.Write(&frame, binary.LittleEndian, uint16(SchemaDefinition))

	// start (uint64 nanos)
	binary.Write(&frame, binary.LittleEndian, uint64(1000000000))

	// end (uint64 nanos)
	binary.Write(&frame, binary.LittleEndian, uint64(2000000000))

	// limit (uint64)
	binary.Write(&frame, binary.LittleEndian, uint64(500))

	// stype_in (uint8) = Parent (4)
	frame.WriteByte(uint8(STypeParent))

	// stype_out (uint8) = InstrumentID (0)
	frame.WriteByte(uint8(STypeInstrumentID))

	// ts_out (uint8) = false
	frame.WriteByte(0)

	// symbol_cstr_len (uint16)
	binary.Write(&frame, binary.LittleEndian, symLen)

	// reserved (53 bytes)
	frame.Write(make([]byte, 53))

	// schema_definition_length (uint32) = 0
	binary.Write(&frame, binary.LittleEndian, uint32(0))

	// === variable-length sections ===

	// symbols: count=1, then 1 symbol of symLen bytes
	binary.Write(&frame, binary.LittleEndian, uint32(1))
	sym := make([]byte, symLen)
	copy(sym, "SPXW.OPT")
	frame.Write(sym)

	// partial: count=0
	binary.Write(&frame, binary.LittleEndian, uint32(0))

	// not_found: count=0
	binary.Write(&frame, binary.LittleEndian, uint32(0))

	// mappings: count=1, one mapping with 1 interval
	binary.Write(&frame, binary.LittleEndian, uint32(1))
	// raw_symbol
	rawSym := make([]byte, symLen)
	copy(rawSym, "SPXW  260227C05000")
	frame.Write(rawSym)
	// interval_count
	binary.Write(&frame, binary.LittleEndian, uint32(1))
	// start_date (YYYYMMDD)
	binary.Write(&frame, binary.LittleEndian, uint32(20260227))
	// end_date (YYYYMMDD)
	binary.Write(&frame, binary.LittleEndian, uint32(20260228))
	// symbol (symLen bytes)
	mappedSym := make([]byte, symLen)
	copy(mappedSym, "12345")
	frame.Write(mappedSym)

	frameBytes := frame.Bytes()

	// Write prelude: "DBN" + version(3) + frame_size(uint32)
	buf.WriteString(DBNMagic)
	buf.WriteByte(DBNVersion)
	binary.Write(&buf, binary.LittleEndian, uint32(len(frameBytes)))
	buf.Write(frameBytes)

	meta, err := DecodeMetadata(&buf)
	if err != nil {
		t.Fatalf("DecodeMetadata: %v", err)
	}

	if meta.Version != DBNVersion {
		t.Errorf("Version: got %d, want %d", meta.Version, DBNVersion)
	}
	if meta.Dataset != "OPRA.PILLAR" {
		t.Errorf("Dataset: got %q, want %q", meta.Dataset, "OPRA.PILLAR")
	}
	if meta.Schema != SchemaDefinition {
		t.Errorf("Schema: got %v, want %v", meta.Schema, SchemaDefinition)
	}
	if meta.Start != clocky.Time(1000000000) {
		t.Errorf("Start: got %v, want %v", meta.Start, clocky.Time(1000000000))
	}
	if meta.End != clocky.Time(2000000000) {
		t.Errorf("End: got %v, want %v", meta.End, clocky.Time(2000000000))
	}
	if meta.Limit != 500 {
		t.Errorf("Limit: got %d, want 500", meta.Limit)
	}
	if meta.STypeIn != STypeParent {
		t.Errorf("STypeIn: got %v, want %v", meta.STypeIn, STypeParent)
	}
	if meta.STypeOut != STypeInstrumentID {
		t.Errorf("STypeOut: got %v, want %v", meta.STypeOut, STypeInstrumentID)
	}
	if meta.TSOut != false {
		t.Errorf("TSOut: got %v, want false", meta.TSOut)
	}
	if meta.SymbolCstrLen != SymbolCstrLen {
		t.Errorf("SymbolCstrLen: got %d, want %d", meta.SymbolCstrLen, SymbolCstrLen)
	}
	if len(meta.Symbols) != 1 || meta.Symbols[0] != "SPXW.OPT" {
		t.Errorf("Symbols: got %v, want [SPXW.OPT]", meta.Symbols)
	}
	if len(meta.Partial) != 0 {
		t.Errorf("Partial: got %v, want []", meta.Partial)
	}
	if len(meta.NotFound) != 0 {
		t.Errorf("NotFound: got %v, want []", meta.NotFound)
	}
	if len(meta.Mappings) != 1 {
		t.Fatalf("Mappings: got %d, want 1", len(meta.Mappings))
	}
	if meta.Mappings[0].RawSymbol != "SPXW  260227C05000" {
		t.Errorf("Mappings[0].RawSymbol: got %q, want %q", meta.Mappings[0].RawSymbol, "SPXW  260227C05000")
	}
	if len(meta.Mappings[0].Intervals) != 1 {
		t.Fatalf("Mappings[0].Intervals: got %d, want 1", len(meta.Mappings[0].Intervals))
	}
	if meta.Mappings[0].Intervals[0].Symbol != "12345" {
		t.Errorf("Mappings[0].Intervals[0].Symbol: got %q, want %q",
			meta.Mappings[0].Intervals[0].Symbol, "12345")
	}
}

// TestDecodeRecord verifies that DecodeRecord reads a properly sized record.
func TestDecodeRecord(t *testing.T) {
	// Create a minimal 16-byte record (length=4 means 4*4=16 bytes)
	var buf bytes.Buffer
	record := make([]byte, 16)
	record[0] = 4                                                    // length = 4 (4*4=16 bytes)
	record[1] = byte(RTypeInstrumentDef)                             // rtype
	binary.LittleEndian.PutUint16(record[2:4], 42)                   // publisher_id
	binary.LittleEndian.PutUint32(record[4:8], 12345)                // instrument_id
	binary.LittleEndian.PutUint64(record[8:16], 9999999999000000000) // ts_event
	buf.Write(record)

	rec, err := DecodeRecord(&buf)
	if err != nil {
		t.Fatalf("DecodeRecord: %v", err)
	}
	if len(rec) != 16 {
		t.Fatalf("record length: got %d, want 16", len(rec))
	}

	hdr := (*RecordHeader)(unsafe.Pointer(&rec[0]))
	if hdr.Length != 4 {
		t.Errorf("Length: got %d, want 4", hdr.Length)
	}
	if hdr.RType != RTypeInstrumentDef {
		t.Errorf("RType: got %v, want %v", hdr.RType, RTypeInstrumentDef)
	}
	if hdr.PublisherID != 42 {
		t.Errorf("PublisherID: got %d, want 42", hdr.PublisherID)
	}
	if hdr.InstrumentID != 12345 {
		t.Errorf("InstrumentID: got %d, want 12345", hdr.InstrumentID)
	}
}

// TestDecodeRecordEOF verifies EOF is returned cleanly at end of stream.
func TestDecodeRecordEOF(t *testing.T) {
	var buf bytes.Buffer
	_, err := DecodeRecord(&buf)
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

// TestDecodeMetadataV1 verifies that we can handle the v1 format with
// record_count and shorter reserved section.
func TestDecodeMetadataV1(t *testing.T) {
	var buf bytes.Buffer
	var frame bytes.Buffer

	// dataset
	ds := [DatasetLen]byte{}
	copy(ds[:], "GLBX.MDP3")
	frame.Write(ds[:])

	// schema
	binary.Write(&frame, binary.LittleEndian, uint16(SchemaMBO))

	// start, end
	binary.Write(&frame, binary.LittleEndian, uint64(3000000000))
	binary.Write(&frame, binary.LittleEndian, uint64(4000000000))

	// limit
	binary.Write(&frame, binary.LittleEndian, uint64(0))

	// v1: record_count (8 bytes, deprecated)
	binary.Write(&frame, binary.LittleEndian, uint64(NullRecordCount))

	// stype_in, stype_out, ts_out
	frame.WriteByte(uint8(STypeRawSymbol))
	frame.WriteByte(uint8(STypeInstrumentID))
	frame.WriteByte(1) // ts_out = true

	// v1 does NOT have symbol_cstr_len field
	// reserved (47 bytes for v1)
	frame.Write(make([]byte, 47))

	// schema_definition_length
	binary.Write(&frame, binary.LittleEndian, uint32(0))

	// symbols: count=0
	binary.Write(&frame, binary.LittleEndian, uint32(0))
	// partial: count=0
	binary.Write(&frame, binary.LittleEndian, uint32(0))
	// not_found: count=0
	binary.Write(&frame, binary.LittleEndian, uint32(0))
	// mappings: count=0
	binary.Write(&frame, binary.LittleEndian, uint32(0))

	frameBytes := frame.Bytes()

	buf.WriteString(DBNMagic)
	buf.WriteByte(1) // version 1
	binary.Write(&buf, binary.LittleEndian, uint32(len(frameBytes)))
	buf.Write(frameBytes)

	meta, err := DecodeMetadata(&buf)
	if err != nil {
		t.Fatalf("DecodeMetadata v1: %v", err)
	}
	if meta.Version != 1 {
		t.Errorf("Version: got %d, want 1", meta.Version)
	}
	if meta.Dataset != "GLBX.MDP3" {
		t.Errorf("Dataset: got %q, want %q", meta.Dataset, "GLBX.MDP3")
	}
	if meta.Schema != SchemaMBO {
		t.Errorf("Schema: got %v, want %v", meta.Schema, SchemaMBO)
	}
	if meta.SymbolCstrLen != 22 {
		t.Errorf("SymbolCstrLen: got %d, want 22", meta.SymbolCstrLen)
	}
	if meta.TSOut != true {
		t.Errorf("TSOut: got %v, want true", meta.TSOut)
	}
}

// TestDecodeISO8601Date verifies YYYYMMDD integer to clocky.Time conversion.
func TestDecodeISO8601Date(t *testing.T) {
	tests := []struct {
		input uint32
		want  clocky.Time // expected UTC midnight nanos
	}{
		{20260227, clocky.Time(time.Date(2026, 2, 27, 0, 0, 0, 0, time.UTC).UnixNano())},
		{20251231, clocky.Time(time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC).UnixNano())},
		{20200101, clocky.Time(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano())},
		{0, 0},
	}
	for _, tt := range tests {
		got := decodeISO8601Date(tt.input)
		if got != tt.want {
			t.Errorf("decodeISO8601Date(%d): got %d, want %d", tt.input, got, tt.want)
		}
	}
}

// TestDecodeMetadataBadMagic verifies error on bad magic bytes.
func TestDecodeMetadataBadMagic(t *testing.T) {
	buf := bytes.NewReader([]byte("XYZ\x03\x64\x00\x00\x00"))
	_, err := DecodeMetadata(buf)
	if err == nil {
		t.Fatal("expected error for bad magic")
	}
}

// TestDecodeMetadataTruncated verifies error on truncated input.
func TestDecodeMetadataTruncated(t *testing.T) {
	buf := bytes.NewReader([]byte("DBN"))
	_, err := DecodeMetadata(buf)
	if err == nil {
		t.Fatal("expected error for truncated prelude")
	}
}
