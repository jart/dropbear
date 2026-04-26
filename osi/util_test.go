package osi

import "testing"

func TestIsOptionsSymbol(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want bool
	}{
		{"valid osi symbol", "SPXW  260409C06450000", true},
		{"noncanonical osi symbol", "SPXW260409C06450000", true},
		{"stock symbol", "SPXW", false},
		{"random stuff", "@", false},
		{"empty string", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsOptionsSymbol(tt.arg); got != tt.want {
				t.Errorf("IsOptionsSymbol() = %v, want %v", got, tt.want)
			}
		})
	}
}
