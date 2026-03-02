// DBN decoder for reading Databento Binary Encoding streams.
// Source: ~/vendor/databento-cpp/src/dbn_decoder.cpp
// Source: ~/vendor/databento-cpp/src/dbn_constants.hpp
package databento

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"
	"unsafe"

	"dropbear/clocky"
)

// CastRecord interprets raw record bytes as a typed struct pointer based on
// the RType byte. Returns nil for unknown record types.
func CastRecord(rec []byte) any {
	if len(rec) < 2 {
		return nil
	}
	rtype := RType(rec[1])
	switch rtype {
	case RTypeCMBP1, RTypeTCBBO:
		if len(rec) < int(unsafe.Sizeof(CMBP1{})) {
			return nil
		}
		return (*CMBP1)(unsafe.Pointer(&rec[0]))
	case RTypeCMBP1S, RTypeCMBP1M, RTypeBBO1S, RTypeBBO1M:
		if len(rec) < int(unsafe.Sizeof(CBBO{})) {
			return nil
		}
		return (*CBBO)(unsafe.Pointer(&rec[0]))
	case RTypeInstrumentDef:
		if len(rec) < int(unsafe.Sizeof(Instrument{})) {
			return nil
		}
		return (*Instrument)(unsafe.Pointer(&rec[0]))
	case RTypeError:
		return ErrorMsg{
			Header: *(*RecordHeader)(unsafe.Pointer(&rec[0])),
			Err:    convertBytesToString(rec[16:]),
		}
	case RTypeSystem:
		return SystemMsg{
			Header: *(*RecordHeader)(unsafe.Pointer(&rec[0])),
			Msg:    convertBytesToString(rec[16:]),
		}
	default:
		return nil
	}
}

// DecodeMetadata reads the DBN prelude and metadata from the given reader.
//
// DBN binary format:
//   - Prelude (8 bytes): "DBN" (3 bytes) + version (1 byte) + frame_size (4 bytes)
//   - Fixed metadata (frame_size bytes): dataset, schema, timestamps, etc.
//   - Variable metadata: symbols, partial, not_found, mappings
func DecodeMetadata(r io.Reader) (Metadata, error) {
	// Read prelude: "DBN" + version + frame_size
	var prelude [MetadataPreludeSize]byte
	if _, err := io.ReadFull(r, prelude[:]); err != nil {
		return Metadata{}, fmt.Errorf("dbn: read prelude: %w", err)
	}
	if string(prelude[:3]) != DBNMagic {
		return Metadata{}, fmt.Errorf("dbn: bad magic: %q", prelude[:3])
	}
	version := prelude[3]
	frameSize := binary.LittleEndian.Uint32(prelude[4:8])
	if frameSize < FixedMetadataLen {
		return Metadata{}, fmt.Errorf("dbn: frame_size %d < fixed metadata len %d", frameSize, FixedMetadataLen)
	}

	// Read the frame
	frame := make([]byte, frameSize)
	if _, err := io.ReadFull(r, frame); err != nil {
		return Metadata{}, fmt.Errorf("dbn: read metadata frame: %w", err)
	}

	return decodeMetadataFields(version, frame)
}

func decodeMetadataFields(version uint8, buf []byte) (Metadata, error) {
	if version > DBNVersion {
		return Metadata{}, fmt.Errorf("dbn: version %d > supported %d", version, DBNVersion)
	}

	var m Metadata
	m.Version = version
	off := 0

	// dataset (16 bytes, null-terminated)
	m.Dataset = convertBytesToString(buf[off : off+DatasetLen])
	off += DatasetLen

	// schema (uint16)
	rawSchema := binary.LittleEndian.Uint16(buf[off:])
	off += 2
	if rawSchema != NullSchema {
		m.Schema = Schema(rawSchema)
	}

	// start (uint64 nanoseconds)
	m.Start = clocky.Time(binary.LittleEndian.Uint64(buf[off:]))
	off += 8

	// end (uint64 nanoseconds)
	m.End = clocky.Time(binary.LittleEndian.Uint64(buf[off:]))
	off += 8

	// limit (uint64)
	m.Limit = binary.LittleEndian.Uint64(buf[off:])
	off += 8

	// v1 has deprecated record_count here
	if version == 1 {
		off += 8
	}

	// stype_in (uint8)
	rawSTypeIn := buf[off]
	off++
	if rawSTypeIn != NullSType {
		m.STypeIn = SType(rawSTypeIn)
	}

	// stype_out (uint8)
	m.STypeOut = SType(buf[off])
	off++

	// ts_out (uint8)
	m.TSOut = buf[off] != 0
	off++

	// symbol_cstr_len (uint16) - only in v2+
	if version > 1 {
		m.SymbolCstrLen = binary.LittleEndian.Uint16(buf[off:])
		off += 2
	} else {
		m.SymbolCstrLen = 22 // kSymbolCstrLenV1
	}

	// skip reserved
	if version == 1 {
		off += 47 // kMetadataReservedLenV1
	} else {
		off += 53 // kMetadataReservedLen
	}

	// schema_definition_length (uint32)
	schemaDefLen := binary.LittleEndian.Uint32(buf[off:])
	off += 4
	if schemaDefLen != 0 {
		return Metadata{}, fmt.Errorf("dbn: schema definitions not supported (len=%d)", schemaDefLen)
	}

	// Variable-length sections
	var err error
	symLen := int(m.SymbolCstrLen)

	m.Symbols, off, err = decodeRepeatedSymbol(buf, off, symLen)
	if err != nil {
		return Metadata{}, fmt.Errorf("dbn: decode symbols: %w", err)
	}

	m.Partial, off, err = decodeRepeatedSymbol(buf, off, symLen)
	if err != nil {
		return Metadata{}, fmt.Errorf("dbn: decode partial: %w", err)
	}

	m.NotFound, off, err = decodeRepeatedSymbol(buf, off, symLen)
	if err != nil {
		return Metadata{}, fmt.Errorf("dbn: decode not_found: %w", err)
	}

	m.Mappings, _, err = decodeSymbolMappings(buf, off, symLen)
	if err != nil {
		return Metadata{}, fmt.Errorf("dbn: decode mappings: %w", err)
	}

	return m, nil
}

func decodeRepeatedSymbol(buf []byte, off, symLen int) ([]string, int, error) {
	if off+4 > len(buf) {
		return nil, off, fmt.Errorf("unexpected end of buffer reading symbol count")
	}
	count := int(binary.LittleEndian.Uint32(buf[off:]))
	off += 4
	need := count * symLen
	if off+need > len(buf) {
		return nil, off, fmt.Errorf("unexpected end of buffer reading %d symbols", count)
	}
	result := make([]string, count)
	for i := range count {
		result[i] = convertBytesToString(buf[off : off+symLen])
		off += symLen
	}
	return result, off, nil
}

func decodeSymbolMappings(buf []byte, off, symLen int) ([]SymbolMapping, int, error) {
	if off+4 > len(buf) {
		return nil, off, fmt.Errorf("unexpected end of buffer reading mapping count")
	}
	count := int(binary.LittleEndian.Uint32(buf[off:]))
	off += 4
	result := make([]SymbolMapping, count)
	for i := range count {
		var err error
		result[i], off, err = decodeSymbolMapping(buf, off, symLen)
		if err != nil {
			return nil, off, err
		}
	}
	return result, off, nil
}

func decodeSymbolMapping(buf []byte, off, symLen int) (SymbolMapping, int, error) {
	if off+symLen+4 > len(buf) {
		return SymbolMapping{}, off, fmt.Errorf("unexpected end of buffer reading symbol mapping")
	}
	var sm SymbolMapping
	sm.RawSymbol = convertBytesToString(buf[off : off+symLen])
	off += symLen

	intervalCount := int(binary.LittleEndian.Uint32(buf[off:]))
	off += 4

	// Each interval: start_date(4) + end_date(4) + symbol(symLen)
	intervalSize := 4 + 4 + symLen
	need := intervalCount * intervalSize
	if off+need > len(buf) {
		return SymbolMapping{}, off, fmt.Errorf("unexpected end of buffer reading %d mapping intervals", intervalCount)
	}

	sm.Intervals = make([]MappingInterval, intervalCount)
	for i := range intervalCount {
		startDate := binary.LittleEndian.Uint32(buf[off:])
		off += 4
		endDate := binary.LittleEndian.Uint32(buf[off:])
		off += 4
		symbol := convertBytesToString(buf[off : off+symLen])
		off += symLen
		sm.Intervals[i] = MappingInterval{
			StartDate: decodeISO8601Date(startDate),
			EndDate:   decodeISO8601Date(endDate),
			Symbol:    symbol,
		}
	}
	return sm, off, nil
}

// decodeISO8601Date converts a YYYYMMDD integer to a clocky.Time at midnight UTC.
func decodeISO8601Date(yyyymmdd uint32) clocky.Time {
	if yyyymmdd == 0 {
		return 0
	}
	year := int(yyyymmdd / 10000)
	remaining := yyyymmdd % 10000
	month := int(remaining / 100)
	day := int(remaining % 100)
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return clocky.Time(t.UnixNano())
}

// DecodeRecord reads one record from the reader. Returns the raw bytes of the
// full record (including header). Returns nil, io.EOF at end of stream.
func DecodeRecord(r io.Reader) ([]byte, error) {
	// Read the first byte: record length in 32-bit words
	var lengthByte [1]byte
	if _, err := io.ReadFull(r, lengthByte[:]); err != nil {
		return nil, err
	}
	recordSize := int(lengthByte[0]) * 4
	if recordSize < 16 {
		return nil, fmt.Errorf("dbn: record size %d too small (min 16)", recordSize)
	}
	record := make([]byte, recordSize)
	record[0] = lengthByte[0]
	if _, err := io.ReadFull(r, record[1:]); err != nil {
		return nil, fmt.Errorf("dbn: read record body: %w", err)
	}
	return record, nil
}

// UpgradeRecord upgrades a record from an older DBN version to v3 in place.
// Only InstrumentDef records need upgrading; other record types are unchanged
// across versions. Returns the (possibly new) record bytes.
func UpgradeRecord(version uint8, rec []byte) ([]byte, error) {
	if version >= DBNVersion {
		return rec, nil
	}
	if len(rec) < 2 {
		return rec, nil
	}
	rtype := RType(rec[1])
	if rtype != RTypeInstrumentDef {
		return rec, nil
	}
	switch {
	case version == 1 && len(rec) >= 360:
		v1 := (*InstrumentV1)(unsafe.Pointer(&rec[0]))
		v3 := v1.ToV3()
		out := make([]byte, unsafe.Sizeof(v3))
		copy(out, (*[520]byte)(unsafe.Pointer(&v3))[:])
		return out, nil
	case version == 2 && len(rec) >= 400:
		v2 := (*InstrumentV2)(unsafe.Pointer(&rec[0]))
		v3 := v2.ToV3()
		out := make([]byte, unsafe.Sizeof(v3))
		copy(out, (*[520]byte)(unsafe.Pointer(&v3))[:])
		return out, nil
	default:
		return nil, fmt.Errorf("dbn: cannot upgrade version %d InstrumentDef (%d bytes)", version, len(rec))
	}
}
