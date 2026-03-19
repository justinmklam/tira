package tui

import (
	"strings"
	"testing"
)

func TestFixedWidth_Exact(t *testing.T) {
	got := FixedWidth("hello", 5)
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestFixedWidth_Truncate(t *testing.T) {
	got := FixedWidth("hello world", 5)
	if got != "hell…" {
		t.Errorf("got %q, want %q", got, "hell…")
	}
}

func TestFixedWidth_Pad(t *testing.T) {
	got := FixedWidth("hi", 5)
	if got != "hi   " {
		t.Errorf("got %q, want %q", got, "hi   ")
	}
}

func TestFixedWidth_TruncateToOne(t *testing.T) {
	got := FixedWidth("hello", 1)
	if got != "h" {
		t.Errorf("got %q, want %q", got, "h")
	}
}

func TestFixedWidth_Empty(t *testing.T) {
	got := FixedWidth("", 3)
	if got != "   " {
		t.Errorf("got %q, want %q", got, "   ")
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		v, lo, hi, want int
	}{
		{5, 0, 10, 5},
		{-1, 0, 10, 0},
		{15, 0, 10, 10},
		{0, 0, 0, 0},
	}
	for _, tt := range tests {
		got := Clamp(tt.v, tt.lo, tt.hi)
		if got != tt.want {
			t.Errorf("Clamp(%d, %d, %d) = %d, want %d", tt.v, tt.lo, tt.hi, got, tt.want)
		}
	}
}

func TestListPaneWidth(t *testing.T) {
	// At 120 width, 40% = 48
	w := ListPaneWidth(120)
	if w != 48 {
		t.Errorf("ListPaneWidth(120) = %d, want 48", w)
	}
	// At 60 width, 40% = 24, but min is 30
	w = ListPaneWidth(60)
	if w != 30 {
		t.Errorf("ListPaneWidth(60) = %d, want 30", w)
	}
}

func TestDetailPaneWidth(t *testing.T) {
	w := DetailPaneWidth(120)
	expected := 120 - ListPaneWidth(120) - 1
	if w != expected {
		t.Errorf("DetailPaneWidth(120) = %d, want %d", w, expected)
	}
	// Small width should return at least 20
	w = DetailPaneWidth(40)
	if w < 20 {
		t.Errorf("DetailPaneWidth(40) = %d, want >= 20", w)
	}
}

func TestSplitPanes(t *testing.T) {
	left := "A\nB"
	right := "X\nY\nZ"
	result := SplitPanes(left, right, 10, 3)
	lines := strings.Split(result, "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
	// Each line should contain the vertical bar separator.
	for i, line := range lines {
		if !strings.Contains(line, "│") {
			t.Errorf("line %d missing separator: %q", i, line)
		}
	}
}

func TestContainsCI(t *testing.T) {
	list := []string{"Bug", "Story", "Task"}

	if !ContainsCI(list, "bug") {
		t.Error("expected case-insensitive match for 'bug'")
	}
	if !ContainsCI(list, "STORY") {
		t.Error("expected case-insensitive match for 'STORY'")
	}
	if ContainsCI(list, "Epic") {
		t.Error("unexpected match for 'Epic'")
	}
	if ContainsCI(nil, "Bug") {
		t.Error("unexpected match on nil list")
	}
}
