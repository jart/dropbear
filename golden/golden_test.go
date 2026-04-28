package golden

import "testing"

func TestMatch(t *testing.T) {
	tests := []struct {
		name     string
		actual   string
		template string
		wantErr  bool
	}{
		{
			name:     "exact match",
			actual:   "hello world",
			template: "hello world",
		},
		{
			name:     "skip prefix",
			actual:   "2026-04-28T15:59:30 hello world",
			template: "...hello world",
		},
		{
			name:     "skip middle",
			actual:   "hello cruel world",
			template: "hello...world",
		},
		{
			name:     "fuzzy number match",
			actual:   "value=105",
			template: "value=100[10]",
		},
		{
			name:     "fuzzy number too far",
			actual:   "value=200",
			template: "value=100[10]",
			wantErr:  true,
		},
		{
			name:     "fuzzy decimal out of range",
			actual:   "pnl=89.9",
			template: "pnl=100[10]",
			wantErr:  true,
		},
		{
			name:     "fuzzy decimal within tolerance",
			actual:   "pnl=95.5",
			template: "pnl=100[10]",
		},
		{
			name:     "negative fuzzy",
			actual:   "fees=-30",
			template: "fees=-33.8[5]",
		},
		{
			name:     "skip then fuzzy",
			actual:   "some prefix stuff P&L: 190.14",
			template: "...P&L: 92[100]",
		},
		{
			name:     "multiple lines",
			actual:   "line1\nINTC: pos=100\nline3",
			template: "...INTC: pos=100[50]\n...",
		},
		{
			name:     "literal number not fuzzy",
			actual:   "tracked: 12",
			template: "tracked: 12",
		},
		{
			name:     "skip at end",
			actual:   "hello world and more stuff",
			template: "hello...",
		},
		{
			name:     "leading newline stripped",
			actual:   "hello",
			template: "\nhello",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Match(tt.actual, tt.template)
			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
