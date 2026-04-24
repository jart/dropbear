package osi

import "testing"

func TestCanonicalize(t *testing.T) {
	type args struct {
		osi string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"SPXW  260409C06450000", args{"SPXW  260409C06450000"}, "SPXW  260409C06450000"},
		{"SPXW260409C06450000", args{"SPXW260409C06450000"}, "SPXW  260409C06450000"},
		{"invalid osi", args{"INVALID_OSI"}, "INVALID_OSI"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Canonicalize(tt.args.osi); got != tt.want {
				t.Errorf("Canonicalize() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUncanonicalize(t *testing.T) {
	type args struct {
		osi string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"SPXW  260409C06450000", args{"SPXW  260409C06450000"}, "SPXW260409C06450000"},
		{"SPXW260409C06450000", args{"SPXW260409C06450000"}, "SPXW260409C06450000"},
		{"invalid osi", args{"INVALID_OSI"}, "INVALID_OSI"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Uncanonicalize(tt.args.osi); got != tt.want {
				t.Errorf("Uncanonicalize() = %v, want %v", got, tt.want)
			}
		})
	}
}

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
