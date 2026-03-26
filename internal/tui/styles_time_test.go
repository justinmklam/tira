package tui

import (
	"testing"
	"time"
)

func TestDaysInColumn_ValidDate(t *testing.T) {
	// Date 10 days ago should return approximately 10 days
	tenDaysAgo := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	got := DaysInColumn(tenDaysAgo)
	if got < 9 || got > 11 {
		t.Errorf("DaysInColumn(%q) = %d, want approximately 10", tenDaysAgo, got)
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
	today := time.Now().Format("2006-01-02")
	got := DaysInColumn(today)
	if got != 0 {
		t.Errorf("DaysInColumn(%q) = %d, want 0", today, got)
	}
}

func TestDaysInColumn_Yesterday(t *testing.T) {
	// Yesterday should return 1 day
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	got := DaysInColumn(yesterday)
	if got != 1 {
		t.Errorf("DaysInColumn(%q) = %d, want 1", yesterday, got)
	}
}

func TestDaysColor_Thresholds(t *testing.T) {
	tests := []struct {
		days int
		want string
	}{
		{0, string(ColorSuccess)},
		{1, string(ColorSuccess)},
		{2, string(ColorSuccess)},
		{3, string(ColorWarning)},
		{5, string(ColorWarning)},
		{6, string(ColorCaution)},
		{9, string(ColorCaution)},
		{10, string(ColorError)},
		{15, string(ColorError)},
		{100, string(ColorError)},
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
