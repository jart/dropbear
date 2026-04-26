package osi

import "testing"

func TestHumanize(t *testing.T) {
	tests := []struct {
		arg  string
		want string
	}{
		{"SPXW  260409C06450000", "SPXW 2026-04-09 C 6450"},
		{"SPXW260409C06450000", "SPXW 2026-04-09 C 6450"},
		{"invalid osi", "invalid osi"},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			if got := Humanize(tt.arg); got != tt.want {
				t.Errorf("Humanize(%#v) got %#v want %#v", tt.arg, got, tt.want)
			}
		})
	}
}
