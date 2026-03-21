package tui

import (
	"testing"
)

func TestDaysInColumn_ValidDate(t *testing.T) {
	// Date 10 days ago should return approximately 10 days
	got := DaysInColumn("2026-03-11")
	if got < 9 || got > 11 {
		t.Errorf("DaysInColumn(\"2026-03-11\") = %d, want approximately 10", got)
	}
}

func TestDaysInColumn_EmptyDate(t *testing.T) {
	got := DaysInColumn("")
	if got != 0 {
		t.Errorf("DaysInColumn(\"\") = %d, want 0", got)
	}
}

func TestDaysInColumn_InvalidDate(t *testing.T) {
	got := DaysInColumn("not-a-date")
	if got != 0 {
		t.Errorf("DaysInColumn(\"not-a-date\") = %d, want 0", got)
	}
}

func TestDaysInColumn_Today(t *testing.T) {
	// Today's date should return 0 days
	got := DaysInColumn("2026-03-21")
	if got != 0 {
		t.Errorf("DaysInColumn(\"2026-03-21\") = %d, want 0", got)
	}
}

func TestDaysInColumn_Yesterday(t *testing.T) {
	// Yesterday should return 1 day
	got := DaysInColumn("2026-03-20")
	if got != 1 {
		t.Errorf("DaysInColumn(\"2026-03-20\") = %d, want 1", got)
	}
}

func TestDaysColor_Thresholds(t *testing.T) {
	tests := []struct {
		days int
		want string
	}{
		{0, string(ColorGreen)},
		{1, string(ColorGreen)},
		{2, string(ColorGreen)},
		{3, string(ColorYellow)},
		{5, string(ColorYellow)},
		{6, string(ColorOrange)},
		{9, string(ColorOrange)},
		{10, string(ColorRed)},
		{15, string(ColorRed)},
		{100, string(ColorRed)},
	}
	for _, tt := range tests {
		t.Run(string(rune(tt.days+'0')), func(t *testing.T) {
			got := string(DaysColor(tt.days))
			if got != tt.want {
				t.Errorf("DaysColor(%d) = %q, want %q", tt.days, got, tt.want)
			}
		})
	}
}
