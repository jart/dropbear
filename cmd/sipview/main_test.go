package main

import (
	"dropbear/broker/alpaca/sip"
	"testing"
)

func TestTypeName(t *testing.T) {
	tests := []struct {
		mt   sip.MessageType
		want string
	}{
		{sip.MessageTypeTrade, "TRD"},
		{sip.MessageTypeQuote, "QTE"},
		{sip.MessageTypeBar, "BAR"},
		{sip.MessageTypeDailyBar, "DLY"},
		{sip.MessageTypeUpdatedBar, "UPD"},
		{sip.MessageTypeStatus, "STS"},
		{sip.MessageTypeLULD, "LLD"},
		{sip.MessageTypeImbalance, "IMB"},
		{sip.MessageTypeCorrection, "COR"},
		{sip.MessageTypeCancelError, "CXE"},
	}
	for _, tt := range tests {
		if got := typeName(tt.mt); got != tt.want {
			t.Errorf("typeName(%v) = %q, want %q", tt.mt, got, tt.want)
		}
	}
}

func TestCommaInt(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1,000"},
		{12345, "12,345"},
		{1000000, "1,000,000"},
		{123456789, "123,456,789"},
	}
	for _, tt := range tests {
		if got := commaInt(tt.n); got != tt.want {
			t.Errorf("commaInt(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestIndexWidth(t *testing.T) {
	tests := []struct {
		n    int
		want int
	}{
		{1, 1},
		{9, 1},
		{10, 2},
		{99, 2},
		{100, 3},
		{999, 3},
		{1000, 4},
		{1000000, 7},
	}
	for _, tt := range tests {
		if got := indexWidth(tt.n); got != tt.want {
			t.Errorf("indexWidth(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}
