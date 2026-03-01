package databento

import (
	"dropbear/clocky"
	"strings"
)

// RecordHeader is the 16-byte header present in all DBN records.
type RecordHeader struct {
	Length       uint8       // record length in 32-bit words (e.g., 14 = 56 bytes for MBO)
	RType        RType       // record type (0xa0 = MBO, 0x01 = Mbp1, etc.)
	PublisherID  uint16      // identifies data source
	InstrumentID uint32      // numeric instrument identifier
	TSEvent      clocky.Time // exchange timestamp in nanoseconds since UNIX epoch
}

func (h *RecordHeader) GoString() string {
	var b strings.Builder
	b.WriteString("RecordHeader{\n")
	appendName(&b, "Length")
	appendField(&b, h.Length)
	appendName(&b, "RType")
	appendField(&b, h.RType)
	appendName(&b, "PublisherID")
	appendField(&b, h.PublisherID)
	appendName(&b, "InstrumentID")
	appendField(&b, h.InstrumentID)
	appendName(&b, "TSEvent")
	appendTimestamp(&b, h.TSEvent)
	b.WriteString("}")
	return b.String()
}
