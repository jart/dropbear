// DBN decoder for reading Databento Binary Encoding streams.
// Source: ~/vendor/databento-cpp/src/dbn_decoder.cpp
// Source: ~/vendor/databento-cpp/src/dbn_constants.hpp
package databento

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
	"unsafe"

	"dropbear/clocky"
)

func castRecord(version uint8, rec []byte) (any, error) {
	if len(rec) < 2 {
		return nil, errors.New("record too short")
	}
	rtype := RType(rec[1])
	switch rtype {
	case RTypeCMBP1, RTypeTCBBO:
		if len(rec) != int(unsafe.Sizeof(CMBP1{})) {
			return nil, errors.New("record size mismatch for CMBP1/TCBBO")
		}
		return (*CMBP1)(unsafe.Pointer(&rec[0])), nil
	case RTypeCMBP1S, RTypeCMBP1M, RTypeBBO1S, RTypeBBO1M:
		if len(rec) != int(unsafe.Sizeof(CBBO{})) {
			return nil, errors.New("record size mismatch for CMBP1S/CMBP1M/BBO1S/BBO1M")
		}
		return (*CBBO)(unsafe.Pointer(&rec[0])), nil
	case RTypeInstrumentDef:
		if version == 1 {
			if len(rec) != int(unsafe.Sizeof(instrumentV1{})) {
				return nil, errors.New("record size mismatch for instrumentV1")
			}
			v1 := (*instrumentV1)(unsafe.Pointer(&rec[0]))
			return v1.ToV3(), nil
		} else {
			if len(rec) != int(unsafe.Sizeof(Instrument{})) {
				return nil, errors.New("record size mismatch for Instrument")
			}
			return (*Instrument)(unsafe.Pointer(&rec[0])), nil
		}
	case RTypeSymbolMapping:
		if version == 1 {
			if len(rec) != int(unsafe.Sizeof(symbolMappingMsgV1{})) {
				return nil, errors.New("record size mismatch for symbolMappingMsgV1")
			}
			v1 := (*symbolMappingMsgV1)(unsafe.Pointer(&rec[0]))
			return v1.ToV3(), nil
		} else {
			if len(rec) != int(unsafe.Sizeof(SymbolMappingMsg{})) {
				return nil, errors.New("record size mismatch for SymbolMappingMsg")
			}
			return (*SymbolMappingMsg)(unsafe.Pointer(&rec[0])), nil
		}
	case RTypeError:
		return &ErrorMsg{
			Header: *(*RecordHeader)(unsafe.Pointer(&rec[0])),
			Err:    convertBytesToString(rec[16:]),
		}, nil
	case RTypeSystem:
		return &SystemMsg{
			Header: *(*RecordHeader)(unsafe.Pointer(&rec[0])),
			Msg:    convertBytesToString(rec[16:]),
		}, nil
	default:
		return nil, errors.New("unknown record type")
	}
}

func decodeMetadata(r io.Reader, m *Metadata) error {
	var prelude [MetadataPreludeSize]byte
	if _, err := io.ReadFull(r, prelude[:]); err != nil {
		return err
	}
	if string(prelude[:3]) != "DBN" {
		return errors.New("dbn magic invalid")
	}
	m.Version = prelude[3]
	if m.Version > 3 {
		return errors.New("dbn version unsupported")
	}
	frameSize := binary.LittleEndian.Uint32(prelude[4:8])
	if frameSize < FixedMetadataLen {
		return errors.New("dbn metadata frame too small")
	}
	frame := make([]byte, frameSize)
	if _, err := io.ReadFull(r, frame); err != nil {
		return err
	}
	off := 0

	// dataset (16 bytes, null-terminated)
	m.Dataset = convertBytesToString(frame[off : off+DatasetLen])
	off += DatasetLen

	// schema (uint16)
	rawSchema := binary.LittleEndian.Uint16(frame[off:])
	off += 2
	if rawSchema != NullSchema {
		m.Schema = Schema(rawSchema)
	}

	// start (uint64 nanoseconds)
	m.Start = clocky.Time(binary.LittleEndian.Uint64(frame[off:]))
	off += 8

	// end (uint64 nanoseconds)
	m.End = clocky.Time(binary.LittleEndian.Uint64(frame[off:]))
	off += 8

	// limit (uint64)
	m.Limit = binary.LittleEndian.Uint64(frame[off:])
	off += 8

	// v1 has deprecated record_count here
	if m.Version == 1 {
		off += 8
	}

	// stype_in (uint8)
	rawSTypeIn := frame[off]
	off++
	if rawSTypeIn != NullSType {
		m.STypeIn = SType(rawSTypeIn)
	}

	// stype_out (uint8)
	m.STypeOut = SType(frame[off])
	off++

	// ts_out (uint8)
	m.TSOut = frame[off] != 0
	off++

	// symbol_cstr_len (uint16) - only in v2+
	if m.Version > 1 {
		m.SymbolCstrLen = binary.LittleEndian.Uint16(frame[off:])
		off += 2
	} else {
		m.SymbolCstrLen = 22 // kSymbolCstrLenV1
	}

	// skip reserved
	if m.Version == 1 {
		off += 47 // kMetadataReservedLenV1
	} else {
		off += 53 // kMetadataReservedLen
	}

	// schema_definition_length (uint32)
	schemaDefLen := binary.LittleEndian.Uint32(frame[off:])
	off += 4
	if schemaDefLen != 0 {
		return fmt.Errorf("dbn: schema definitions not supported (len=%d)", schemaDefLen)
	}

	// Variable-length sections
	var err error
	symLen := int(m.SymbolCstrLen)

	m.Symbols, off, err = decodeRepeatedSymbol(frame, off, symLen)
	if err != nil {
		return fmt.Errorf("dbn: decode symbols: %w", err)
	}

	m.Partial, off, err = decodeRepeatedSymbol(frame, off, symLen)
	if err != nil {
		return fmt.Errorf("dbn: decode partial: %w", err)
	}

	m.NotFound, off, err = decodeRepeatedSymbol(frame, off, symLen)
	if err != nil {
		return fmt.Errorf("dbn: decode not_found: %w", err)
	}

	m.Mappings, _, err = decodeSymbolMappings(frame, off, symLen)
	if err != nil {
		return fmt.Errorf("dbn: decode mappings: %w", err)
	}

	return nil
}

func decodeRepeatedSymbol(frame []byte, off, symLen int) ([]string, int, error) {
	if off+4 > len(frame) {
		return nil, off, fmt.Errorf("unexpected end of buffer reading symbol count")
	}
	count := int(binary.LittleEndian.Uint32(frame[off:]))
	off += 4
	need := count * symLen
	if off+need > len(frame) {
		return nil, off, fmt.Errorf("unexpected end of buffer reading %d symbols", count)
	}
	result := make([]string, count)
	for i := range count {
		result[i] = convertBytesToString(frame[off : off+symLen])
		off += symLen
	}
	return result, off, nil
}

func decodeSymbolMappings(frame []byte, off, symLen int) ([]SymbolMapping, int, error) {
	if off+4 > len(frame) {
		return nil, off, fmt.Errorf("unexpected end of buffer reading mapping count")
	}
	count := int(binary.LittleEndian.Uint32(frame[off:]))
	off += 4
	result := make([]SymbolMapping, count)
	for i := range count {
		var err error
		result[i], off, err = decodeSymbolMapping(frame, off, symLen)
		if err != nil {
			return nil, off, err
		}
	}
	return result, off, nil
}

func decodeSymbolMapping(frame []byte, off, symLen int) (SymbolMapping, int, error) {
	if off+symLen+4 > len(frame) {
		return SymbolMapping{}, off, fmt.Errorf("unexpected end of buffer reading symbol mapping")
	}
	var sm SymbolMapping
	sm.RawSymbol = convertBytesToString(frame[off : off+symLen])
	off += symLen
	intervalCount := int(binary.LittleEndian.Uint32(frame[off:]))
	off += 4
	// Each interval: start_date(4) + end_date(4) + symbol(symLen)
	intervalSize := 4 + 4 + symLen
	need := intervalCount * intervalSize
	if off+need > len(frame) {
		return SymbolMapping{}, off, fmt.Errorf("unexpected end of buffer reading %d mapping intervals", intervalCount)
	}
	sm.Intervals = make([]MappingInterval, intervalCount)
	for i := range intervalCount {
		startDate := binary.LittleEndian.Uint32(frame[off:])
		off += 4
		endDate := binary.LittleEndian.Uint32(frame[off:])
		off += 4
		symbol := convertBytesToString(frame[off : off+symLen])
		off += symLen
		sm.Intervals[i] = MappingInterval{
			StartDate: decodeISO8601Date(startDate),
			EndDate:   decodeISO8601Date(endDate),
			Symbol:    symbol,
		}
	}
	return sm, off, nil
}

// decodeRecord reads one record from the reader. Returns the raw bytes of the
// full record (including header). Returns nil, io.EOF at end of stream.
func decodeRecord(r io.Reader) ([]byte, error) {
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
