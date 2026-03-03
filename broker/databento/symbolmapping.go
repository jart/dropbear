package databento

import (
	"dropbear/clocky"
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
