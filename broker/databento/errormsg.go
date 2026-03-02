// ErrorMsg and SystemMsg records from the Live Subscription Gateway.
// Source: ~/vendor/databento-cpp/include/databento/record.hpp
//
// These records have variable wire sizes across protocol versions (v1 sends
// 80 bytes, v3 sends 320 bytes) so CastRecord builds them from raw bytes
// rather than using unsafe pointer casts.
package databento

// ErrorMsg is sent by the gateway to indicate an error.
type ErrorMsg struct {
	Header RecordHeader
	Err    string
}

// SystemMsg is sent by the gateway for non-error messages such as heartbeats
// and subscription acknowledgements.
type SystemMsg struct {
	Header RecordHeader
	Msg    string
}

// IsHeartbeat returns true if this is a heartbeat message.
func (m SystemMsg) IsHeartbeat() bool {
	return len(m.Msg) >= 9 && m.Msg[:9] == "Heartbeat"
}
