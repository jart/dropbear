package databento

import (
	"dropbear/clocky"
	"strings"
	"unsafe"
)

// StatusMsg is an exchange status record (RType 0x12, 40 bytes).
// Unchanged across DBN versions v1/v2/v3.
type StatusMsg struct {
	Header                RecordHeader // 16 bytes
	TSRecv                clocky.Time  // capture-server-received timestamp
	Action                StatusAction // primary status action
	Reason                StatusReason // cause of halt or status change
	TradingEvent          TradingEvent // further information about the update
	IsTrading             TriState     // whether the instrument is trading
	IsQuoting             TriState     // whether the instrument is quoting
	IsShortSellRestricted TriState     // whether short selling is restricted
	_reserved             [7]byte
}

func (m *StatusMsg) Encode() []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(m)), unsafe.Sizeof(*m))
}

func (m *StatusMsg) InstrumentID() uint32 {
	return m.Header.InstrumentID
}

func (m *StatusMsg) GetTSRecv() clocky.Time {
	return m.TSRecv
}

func (m *StatusMsg) String() string {
	return m.GoString()
}

func (m *StatusMsg) GoString() string {
	var b strings.Builder
	b.WriteString("StatusMsg{\n")
	appendName(&b, "Header")
	appendField(&b, &m.Header)
	appendName(&b, "TSRecv")
	appendTimestamp(&b, m.TSRecv)
	appendName(&b, "Action")
	appendField(&b, m.Action)
	appendName(&b, "Reason")
	appendField(&b, m.Reason)
	appendName(&b, "TradingEvent")
	appendField(&b, m.TradingEvent)
	appendName(&b, "IsTrading")
	appendField(&b, m.IsTrading)
	appendName(&b, "IsQuoting")
	appendField(&b, m.IsQuoting)
	appendName(&b, "IsShortSellRestricted")
	appendField(&b, m.IsShortSellRestricted)
	b.WriteString("}")
	return b.String()
}
