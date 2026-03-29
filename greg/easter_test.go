package greg

import (
	"dropbear/clocky"
	"testing"
)

func TestEasterSunday(t *testing.T) {
	// Known Easter dates
	tests := []struct {
		year      int
		wantMonth clocky.Month
		wantDay   int
	}{
		{2020, clocky.April, 12},
		{2021, clocky.April, 4},
		{2022, clocky.April, 17},
		{2023, clocky.April, 9},
		{2024, clocky.March, 31},
		{2025, clocky.April, 20},
		{2026, clocky.April, 5},
		{2027, clocky.March, 28},
		{2028, clocky.April, 16},
		{2029, clocky.April, 1},
		{2030, clocky.April, 21},
	}
	for _, tt := range tests {
		gotMonth, gotDay := EasterSunday(tt.year)
		if gotMonth != tt.wantMonth || gotDay != tt.wantDay {
			t.Errorf("EasterSunday(%d) = %v %d, want %v %d",
				tt.year, gotMonth, gotDay, tt.wantMonth, tt.wantDay)
		}
	}
}

func TestGoodFriday(t *testing.T) {
	// Good Friday is always 2 days before Easter
	tests := []struct {
		year      int
		wantMonth clocky.Month
		wantDay   int
	}{
		{2020, clocky.April, 10},
		{2021, clocky.April, 2},
		{2022, clocky.April, 15},
		{2023, clocky.April, 7},
		{2024, clocky.March, 29}, // Easter is March 31, so Good Friday is March 29
		{2025, clocky.April, 18},
		{2026, clocky.April, 3},
		{2027, clocky.March, 26}, // Easter is March 28
	}
	for _, tt := range tests {
		gotMonth, gotDay := GoodFriday(tt.year)
		if gotMonth != tt.wantMonth || gotDay != tt.wantDay {
			t.Errorf("GoodFriday(%d) = %v %d, want %v %d",
				tt.year, gotMonth, gotDay, tt.wantMonth, tt.wantDay)
		}
	}
}

func BenchmarkEasterSunday(b *testing.B) {
	for b.Loop() {
		EasterSunday(2026)
	}
}
