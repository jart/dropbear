package databento

import (
	"dropbear/clocky"
	"encoding/binary"
	"strconv"
	"strings"
)

// Metadata contains DBN file header information.
type Metadata struct {
	Version       uint8           // dbn schema version number
	Dataset       string          // dataset code
	Schema        Schema          // optional data record schema which affects type of record present
	Start         clocky.Time     // unix timestamp of query start, or first record if file was split
	End           clocky.Time     // unix timestamp of query end, or last record if file was split
	Limit         uint64          // maximum number of records for query
	STypeIn       SType           // input symbology type
	STypeOut      SType           // output symbology type
	TSOut         bool            // whether records contain appended send timestamp
	SymbolCstrLen uint16          // length in bytes of fixed-length symbol strings, including nul terminator byte
	Symbols       []string        // original queryinput symbols from request
	Partial       []string        // symbols that didn't resolve for at least one dayin query time range
	NotFound      []string        // symbols that didn't resolve for any day in query time range
	Mappings      []SymbolMapping // symbol mappings containing a native symbol and its mapping intervals
}

type SymbolMapping struct {
	RawSymbol string
	Intervals []MappingInterval
}

type MappingInterval struct {
	StartDate clocky.Time // inclusive
	EndDate   clocky.Time // exclusive
	Symbol    string      // resolved symbol for this interval (in stype_out)
}

func (m *Metadata) GoString() string {
	var b strings.Builder
	b.WriteString("Metadata{\n")
	appendName(&b, "Version")
	appendField(&b, m.Version)
	appendName(&b, "Dataset")
	appendField(&b, m.Dataset)
	appendName(&b, "Schema")
	appendField(&b, m.Schema)
	appendName(&b, "Start")
	appendTimestamp(&b, m.Start)
	appendName(&b, "End")
	appendTimestamp(&b, m.End)
	appendName(&b, "Limit")
	appendField(&b, m.Limit)
	appendName(&b, "STypeIn")
	appendField(&b, m.STypeIn)
	appendName(&b, "STypeOut")
	appendField(&b, m.STypeOut)
	appendName(&b, "TSOut")
	appendField(&b, m.TSOut)
	appendName(&b, "SymbolCstrLen")
	appendField(&b, m.SymbolCstrLen)
	appendName(&b, "Symbols")
	appendField(&b, m.Symbols)
	appendName(&b, "Partial")
	appendField(&b, m.Partial)
	appendName(&b, "NotFound")
	appendField(&b, m.NotFound)
	b.WriteString("/* ")
	b.WriteString(strconv.Itoa(len(m.Mappings)))
	b.WriteString(" Mappings */\n")
	b.WriteString("}")
	return b.String()
}

// Encode serializes the metadata to DBN wire format.
func (m *Metadata) Encode() []byte {
	symLen := int(m.SymbolCstrLen)

	// compute frame size
	frameSize := FixedMetadataLen + 4       // +4 for schema_definition_length
	frameSize += 4 + len(m.Symbols)*symLen  // symbols
	frameSize += 4 + len(m.Partial)*symLen  // partial
	frameSize += 4 + len(m.NotFound)*symLen // not_found
	frameSize += 4                          // mappings count
	for _, sm := range m.Mappings {
		frameSize += symLen + 4 // raw_symbol + interval_count
		frameSize += len(sm.Intervals) * (4 + 4 + symLen)
	}

	frame := make([]byte, frameSize)
	off := 0

	// dataset (16 bytes, null-padded)
	copy(frame[off:off+DatasetLen], m.Dataset)
	off += DatasetLen

	// schema (uint16)
	binary.LittleEndian.PutUint16(frame[off:], uint16(m.Schema))
	off += 2

	// start (uint64 nanoseconds)
	binary.LittleEndian.PutUint64(frame[off:], uint64(m.Start))
	off += 8

	// end (uint64 nanoseconds)
	binary.LittleEndian.PutUint64(frame[off:], uint64(m.End))
	off += 8

	// limit (uint64)
	binary.LittleEndian.PutUint64(frame[off:], m.Limit)
	off += 8

	// stype_in (uint8)
	frame[off] = uint8(m.STypeIn)
	off++

	// stype_out (uint8)
	frame[off] = uint8(m.STypeOut)
	off++

	// ts_out (uint8)
	if m.TSOut {
		frame[off] = 1
	}
	off++

	// symbol_cstr_len (uint16)
	binary.LittleEndian.PutUint16(frame[off:], m.SymbolCstrLen)
	off += 2

	// reserved (53 bytes)
	off += 53

	// schema_definition_length (uint32) - always 0
	off += 4

	// variable-length sections
	off = encodeRepeatedSymbol(frame, off, m.Symbols, symLen)
	off = encodeRepeatedSymbol(frame, off, m.Partial, symLen)
	off = encodeRepeatedSymbol(frame, off, m.NotFound, symLen)
	encodeSymbolMappings(frame, off, m.Mappings, symLen)

	// build prelude + frame
	version := m.Version
	if version == 0 {
		version = 2
	}
	var prelude [MetadataPreludeSize]byte
	copy(prelude[:3], "DBN")
	prelude[3] = version
	binary.LittleEndian.PutUint32(prelude[4:8], uint32(frameSize))

	result := make([]byte, MetadataPreludeSize+frameSize)
	copy(result, prelude[:])
	copy(result[MetadataPreludeSize:], frame)
	return result
}
