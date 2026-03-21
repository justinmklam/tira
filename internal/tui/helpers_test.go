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

func TestOverlaySize_Clamping(t *testing.T) {
	tests := []struct {
		name        string
		totalWidth  int
		totalHeight int
		wantW       int
		wantH       int
	}{
		{"normal", 120, 40, 102, 38},
		{"small", 60, 20, 60, 19},
		{"large", 200, 60, 140, 57},
		{"min width", 40, 30, 60, 28},
		{"min height", 100, 10, 85, 15},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotW, gotH := OverlaySize(tt.totalWidth, tt.totalHeight)
			if gotW != tt.wantW {
				t.Errorf("OverlaySize(%d, %d) width = %d, want %d", tt.totalWidth, tt.totalHeight, gotW, tt.wantW)
			}
			if gotH != tt.wantH {
				t.Errorf("OverlaySize(%d, %d) height = %d, want %d", tt.totalWidth, tt.totalHeight, gotH, tt.wantH)
			}
		})
	}
}

func TestOverlayViewportSize_MinValues(t *testing.T) {
	tests := []struct {
		name        string
		totalWidth  int
		totalHeight int
		minVpW      int
		minVpH      int
	}{
		{"normal", 120, 40, 20, 5},
		{"small", 60, 20, 20, 5},
		{"large", 200, 60, 20, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVpW, gotVpH := OverlayViewportSize(tt.totalWidth, tt.totalHeight)
			if gotVpW < tt.minVpW {
				t.Errorf("OverlayViewportSize(%d, %d) width = %d, want >= %d", tt.totalWidth, tt.totalHeight, gotVpW, tt.minVpW)
			}
			if gotVpH < tt.minVpH {
				t.Errorf("OverlayViewportSize(%d, %d) height = %d, want >= %d", tt.totalWidth, tt.totalHeight, gotVpH, tt.minVpH)
			}
		})
	}
}
