package databento

import (
	"dropbear/clocky"
	"strings"
	"unsafe"
)

// SymbolMappingMsg maps a symbol from one SType to another (v2+).
type SymbolMappingMsg struct {
	Header         RecordHeader
	STypeIn        SType
	STypeInSymbol  [71]byte
	STypeOut       SType
	STypeOutSymbol [71]byte
	StartTS        clocky.Time
	EndTS          clocky.Time
}

func (sm *SymbolMappingMsg) GetSTypeInSymbol() string {
	return convertBytesToString(sm.STypeInSymbol[:])
}

func (sm *SymbolMappingMsg) GetSTypeOutSymbol() string {
	return convertBytesToString(sm.STypeOutSymbol[:])
}

func (sm *SymbolMappingMsg) GoString() string {
	var b strings.Builder
	b.WriteString("SymbolMappingMsg{\n")
	appendName(&b, "Header")
	appendField(&b, &sm.Header)
	appendName(&b, "STypeIn")
	appendField(&b, sm.STypeIn)
	appendName(&b, "STypeInSymbol")
	appendByteString(&b, sm.STypeInSymbol[:])
	appendName(&b, "STypeOut")
	appendField(&b, sm.STypeOut)
	appendName(&b, "STypeOutSymbol")
	appendByteString(&b, sm.STypeOutSymbol[:])
	appendName(&b, "StartTS")
	appendTimestamp(&b, sm.StartTS)
	appendName(&b, "EndTS")
	appendTimestamp(&b, sm.EndTS)
	b.WriteString("}")
	return b.String()
}

// SymbolMappingMsgV1 maps a symbol from one SType to another (v1).
type symbolMappingMsgV1 struct {
	Header         RecordHeader
	STypeInSymbol  [22]byte
	STypeOutSymbol [22]byte
	_              [4]byte // padding
	StartTS        clocky.Time
	EndTS          clocky.Time
}

func (sm *symbolMappingMsgV1) GetSTypeInSymbol() string {
	return convertBytesToString(sm.STypeInSymbol[:])
}

func (sm *symbolMappingMsgV1) GetSTypeOutSymbol() string {
	return convertBytesToString(sm.STypeOutSymbol[:])
}

func (sm *symbolMappingMsgV1) ToV3() *SymbolMappingMsg {
	var out SymbolMappingMsg
	out.Header = sm.Header
	out.Header.Length = uint8(unsafe.Sizeof(SymbolMappingMsg{}) / 4)
	copy(out.STypeInSymbol[:], sm.STypeInSymbol[:])
	copy(out.STypeOutSymbol[:], sm.STypeOutSymbol[:])
	out.StartTS = sm.StartTS
	out.EndTS = sm.EndTS
	return &out
}
