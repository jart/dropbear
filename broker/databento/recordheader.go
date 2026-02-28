package databento

import "dropbear/clocky"

// RecordHeader is the 16-byte header present in all DBN records.
type RecordHeader struct {
	Length       uint8       // record length in 32-bit words (e.g., 14 = 56 bytes for MBO)
	RType        RType       // record type (0xa0 = MBO, 0x01 = Mbp1, etc.)
	PublisherID  uint16      // identifies data source
	InstrumentID uint32      // numeric instrument identifier
	TSEvent      clocky.Time // exchange timestamp in nanoseconds since UNIX epoch
}
