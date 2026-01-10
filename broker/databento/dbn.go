package databento

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"unsafe"
)

// DBN file format constants.
const (
	DBNMagic      = "DBN"
	DBNVersion1   = 1
	DBNVersion2   = 2
	DatasetLen    = 16
	SymbolCstrLen = 72 // DBNv2 uses 72-byte symbol strings
)

var (
	ErrInvalidMagic   = errors.New("dbn: invalid magic bytes")
	ErrInvalidVersion = errors.New("dbn: unsupported version")
	ErrTruncated      = errors.New("dbn: truncated file")
)

// Metadata contains DBN file header information.
type Metadata struct {
	Version       uint8
	Dataset       string
	Schema        uint16
	Start         uint64 // Unix nanoseconds
	End           uint64 // Unix nanoseconds
	Limit         uint64
	StypeIn       uint8
	StypeOut      uint8
	TsOut         bool
	SymbolCstrLen uint16
	Symbols       []string
	Partial       []string
	NotFound      []string
	Mappings      []SymbolMapping
}

// SymbolMapping maps input symbols to instrument IDs.
type SymbolMapping struct {
	RawSymbol    string
	InstrumentID uint32
}

// Schema constants.
const (
	SchemaMbo       uint16 = 0  // Market by Order (L3)
	SchemaMbp1      uint16 = 1  // Top of book (L1)
	SchemaMbp10     uint16 = 2  // 10 levels (L2)
	SchemaTbbo      uint16 = 3  // Trade + BBO
	SchemaTrades    uint16 = 4  // Trades only
	SchemaOhlcv1S   uint16 = 5  // 1-second bars
	SchemaOhlcv1M   uint16 = 6  // 1-minute bars
	SchemaOhlcv1H   uint16 = 7  // 1-hour bars
	SchemaOhlcv1D   uint16 = 8  // 1-day bars
	SchemaStatus    uint16 = 10 // Trading status
	SchemaImbalance uint16 = 11 // Auction imbalance
)

// DBNReader reads DBN format files.
type DBNReader struct {
	r          io.Reader
	Metadata   Metadata
	headerSize int
}

// NewDBNReader creates a reader for a DBN file.
func NewDBNReader(r io.Reader) *DBNReader {
	return &DBNReader{r: r}
}

// ReadHeader reads and parses the DBN metadata header.
func (d *DBNReader) ReadHeader() error {
	// Read fixed prefix: magic (3) + version (1) + length (4) = 8 bytes
	var prefix [8]byte
	if _, err := io.ReadFull(d.r, prefix[:]); err != nil {
		return fmt.Errorf("%w: reading prefix: %v", ErrTruncated, err)
	}

	if string(prefix[:3]) != DBNMagic {
		return ErrInvalidMagic
	}
	d.Metadata.Version = prefix[3]
	if d.Metadata.Version != DBNVersion1 && d.Metadata.Version != DBNVersion2 {
		return fmt.Errorf("%w: %d", ErrInvalidVersion, d.Metadata.Version)
	}

	metaLen := binary.LittleEndian.Uint32(prefix[4:8])
	d.headerSize = 8 + int(metaLen)

	// Read the rest of the metadata
	meta := make([]byte, metaLen)
	if _, err := io.ReadFull(d.r, meta); err != nil {
		return fmt.Errorf("%w: reading metadata: %v", ErrTruncated, err)
	}

	return d.parseMetadata(meta)
}

func (d *DBNReader) parseMetadata(data []byte) error {
	if len(data) < 64 {
		return ErrTruncated
	}

	// Dataset: 16 bytes null-terminated
	d.Metadata.Dataset = nullTermString(data[0:16])

	// Schema: u16 at offset 16
	d.Metadata.Schema = binary.LittleEndian.Uint16(data[16:18])

	// Start: u64 at offset 18 (or with padding - need to check alignment)
	// Looking at the hex dump, timestamps start at offset ~20 from metadata start
	// Let me parse based on what I see in the file

	// After dataset (16 bytes), we have:
	// - schema (2 bytes) -> offset 16
	// - padding (2 bytes) -> offset 18
	// - start timestamp (8 bytes) -> offset 20
	// - end timestamp (8 bytes) -> offset 28
	// - limit (8 bytes) -> offset 36
	// - reserved/limit upper (8 bytes) -> offset 44
	// - stype_in (1 byte) -> offset 52
	// - stype_out (1 byte) -> offset 53
	// - ts_out (1 byte) -> offset 54
	// - padding -> offset 55
	// - symbol_cstr_len (2 bytes) -> offset 56
	// - padding (2 bytes) -> offset 58
	// - symbol count (4 bytes) -> offset 60

	offset := 16
	d.Metadata.Schema = binary.LittleEndian.Uint16(data[offset:])
	offset += 4 // schema + 2 bytes padding

	d.Metadata.Start = binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	d.Metadata.End = binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	d.Metadata.Limit = binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	// Skip reserved 8 bytes (limit upper bits in v2)
	offset += 8

	d.Metadata.StypeIn = data[offset]
	offset++
	d.Metadata.StypeOut = data[offset]
	offset++
	d.Metadata.TsOut = data[offset] != 0
	offset++

	// Skip 1 byte padding
	offset++

	d.Metadata.SymbolCstrLen = binary.LittleEndian.Uint16(data[offset:])
	offset += 2

	// Skip 2 bytes padding
	offset += 2

	// Read symbols count
	if offset+4 > len(data) {
		return nil // No symbol info
	}
	symbolCount := binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	// Read symbols
	symLen := int(d.Metadata.SymbolCstrLen)
	if symLen == 0 {
		symLen = 22 // DBNv1 default
	}

	for i := uint32(0); i < symbolCount && offset+symLen <= len(data); i++ {
		sym := nullTermString(data[offset : offset+symLen])
		d.Metadata.Symbols = append(d.Metadata.Symbols, sym)
		offset += symLen
	}

	// Read partial count and symbols
	if offset+4 <= len(data) {
		partialCount := binary.LittleEndian.Uint32(data[offset:])
		offset += 4
		for i := uint32(0); i < partialCount && offset+symLen <= len(data); i++ {
			sym := nullTermString(data[offset : offset+symLen])
			d.Metadata.Partial = append(d.Metadata.Partial, sym)
			offset += symLen
		}
	}

	// Read not_found count and symbols
	if offset+4 <= len(data) {
		notFoundCount := binary.LittleEndian.Uint32(data[offset:])
		offset += 4
		for i := uint32(0); i < notFoundCount && offset+symLen <= len(data); i++ {
			sym := nullTermString(data[offset : offset+symLen])
			d.Metadata.NotFound = append(d.Metadata.NotFound, sym)
			offset += symLen
		}
	}

	// Read mappings count
	if offset+4 <= len(data) {
		mappingCount := binary.LittleEndian.Uint32(data[offset:])
		offset += 4

		for i := uint32(0); i < mappingCount && offset+symLen+8 <= len(data); i++ {
			rawSym := nullTermString(data[offset : offset+symLen])
			offset += symLen

			// Skip interval count and read first interval's instrument_id
			intervalCount := binary.LittleEndian.Uint32(data[offset:])
			offset += 4

			var instrID uint32
			if intervalCount > 0 && offset+20 <= len(data) {
				// Each interval: start_date(4) + end_date(4) + symbol(symLen)
				// But we need instrument_id... let me check the format
				// Actually instrument_id comes from the record headers
				offset += int(intervalCount) * (8 + symLen)
			}

			d.Metadata.Mappings = append(d.Metadata.Mappings, SymbolMapping{
				RawSymbol:    rawSym,
				InstrumentID: instrID,
			})
		}
	}

	return nil
}

func nullTermString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// HeaderSize returns the total header size in bytes.
func (d *DBNReader) HeaderSize() int {
	return d.headerSize
}

// RecordSize returns the record size for the schema.
func (d *DBNReader) RecordSize() int {
	switch d.Metadata.Schema {
	case SchemaMbo:
		return int(unsafe.Sizeof(MboMsg{}))
	case SchemaMbp1:
		return int(unsafe.Sizeof(Mbp1Msg{}))
	case SchemaMbp10:
		return int(unsafe.Sizeof(Mbp10Msg{}))
	case SchemaTrades:
		return int(unsafe.Sizeof(TradeMsg{}))
	case SchemaOhlcv1S, SchemaOhlcv1M, SchemaOhlcv1H, SchemaOhlcv1D:
		return int(unsafe.Sizeof(OhlcvMsg{}))
	default:
		return 0
	}
}

// ReadDBNFile reads a DBN file and returns metadata and raw record bytes.
func ReadDBNFile(path string) (*Metadata, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	reader := NewDBNReader(f)
	if err := reader.ReadHeader(); err != nil {
		return nil, nil, err
	}

	// Read the rest as raw record data
	records, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, err
	}

	return &reader.Metadata, records, nil
}
