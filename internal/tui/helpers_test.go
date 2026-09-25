package tui

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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

func TestFixedWidth_CountsDisplayCells(t *testing.T) {
	tests := []struct {
		name string
		s    string
		n    int
		want int
	}{
		{"wide runes pad", "プロジェクト", 14, 14},
		{"wide runes truncate", "日本語のタイトルです", 10, 10},
		{"emoji pad", "ship it 🚀", 12, 12},
		{"combining mark", "a\u0301bc", 4, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FixedWidth(tt.s, tt.n)
			if w := DisplayWidth(got); w != tt.want {
				t.Errorf("FixedWidth(%q, %d) = %q (%d cells), want %d cells", tt.s, tt.n, got, w, tt.want)
			}
		})
	}
}

func TestFixedWidth_NonPositiveWidth(t *testing.T) {
	for _, n := range []int{0, -1, -100} {
		if got := FixedWidth("abc", n); got != "" {
			t.Errorf("FixedWidth(abc, %d) = %q, want the empty string", n, got)
		}
	}
}

func TestSanitizeRow(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"newline", "line one\nline two", "line one line two"},
		{"carriage return", "a\rb", "a b"},
		{"tab", "a\tb", "a b"},
		{"collapse runs", "a   \n\n b", "a b"},
		{"drops escapes", "a\x1b[31mb", "a[31mb"},
		{"drops control chars", "a\x00\x07b", "ab"},
		{"trims", "  padded  ", "padded"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeRow(tt.in); got != tt.want {
				t.Errorf("SanitizeRow(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeRow_RemovesLineBreaks(t *testing.T) {
	got := SanitizeRow("summary\nwith a break")
	if strings.ContainsAny(got, "\n\r") {
		t.Fatalf("SanitizeRow left a line break in %q", got)
	}
}

func TestFixedWidth_Empty(t *testing.T) {
	got := FixedWidth("", 3)
	if got != "   " {
		t.Errorf("got %q, want %q", got, "   ")
	}
}

func TestFormatStoryPoints(t *testing.T) {
	tests := []struct {
		points float64
		want   string
	}{
		{0, "—"},
		{-1, "—"},
		{math.NaN(), "—"},
		{3, "3"},
		{3.5, "3.5"},
	}
	for _, tt := range tests {
		if got := FormatStoryPoints(tt.points); got != tt.want {
			t.Errorf("FormatStoryPoints(%v) = %q, want %q", tt.points, got, tt.want)
		}
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
	// At 120 width, 55% = 66
	w := ListPaneWidth(120)
	if w != 66 {
		t.Errorf("ListPaneWidth(120) = %d, want 66", w)
	}
	// At 60 width, 55% = 33, which is above min of 30
	w = ListPaneWidth(60)
	if w != 33 {
		t.Errorf("ListPaneWidth(60) = %d, want 33", w)
	}
	// At 40 width, 55% = 22, but min is 30
	w = ListPaneWidth(40)
	if w != 30 {
		t.Errorf("ListPaneWidth(40) = %d, want 30", w)
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

func TestPickerOverlaySizeFitsTerminal(t *testing.T) {
	tests := []struct{ w, h int }{
		{120, 40}, {200, 60}, {90, 24}, {80, 20}, {60, 14}, {40, 10}, {300, 100},
	}
	for _, term := range tests {
		t.Run(fmt.Sprintf("%dx%d", term.w, term.h), func(t *testing.T) {
			modalW, innerW, listH := PickerOverlaySize(term.w, term.h)
			if modalW > term.w {
				t.Errorf("modal width %d exceeds terminal %d", modalW, term.w)
			}
			if innerW != modalW-2 {
				t.Errorf("innerW = %d, want modalW-2 = %d", innerW, modalW-2)
			}
			if maxRows := term.h - 5; listH > maxRows {
				t.Errorf("listH = %d exceeds the %d rows left after the modal chrome", listH, maxRows)
			}
			if listH < 1 {
				t.Errorf("listH = %d, want at least 1", listH)
			}
		})
	}
}

// TestRenderPickerOverlayKeepsFrameIntact pins the invariant every picker
// overlay depends on: no body line wraps, so the box keeps its height and both
// side borders on every row. Before this was guaranteed, lines two cells too
// wide wrapped and left stray separator fragments behind.
func TestRenderPickerOverlayKeepsFrameIntact(t *testing.T) {
	contentLines := 0
	content := func(innerW, listH int) string {
		lines := []string{
			FixedWidth(" input", innerW),
			MutedStyle.Render(strings.Repeat("─", innerW)),
		}
		// Deliberately wider than the modal to prove the frame clamps it.
		wide := FixedWidth(strings.Repeat("x", innerW+10), innerW-2)
		for i := 0; i < listH; i++ {
			lines = append(lines, "  "+wide)
		}
		contentLines = len(lines)
		return strings.Join(lines, "\n")
	}

	for _, term := range []struct{ w, h int }{{120, 40}, {90, 24}, {80, 20}, {60, 14}, {40, 10}} {
		t.Run(fmt.Sprintf("%dx%d", term.w, term.h), func(t *testing.T) {
			out := RenderPickerOverlay(content, "Linked Items", term.w, term.h)
			lines := strings.Split(out, "\n")
			if len(lines) != term.h {
				t.Fatalf("rendered %d lines, want %d", len(lines), term.h)
			}

			var modal []string
			for _, line := range lines {
				if strings.TrimSpace(ansi.Strip(line)) != "" {
					modal = append(modal, line)
				}
			}

			// Header + content + footer + two border rows.
			wantRows := contentLines + 4
			if len(modal) != wantRows {
				t.Errorf("modal is %d rows, want %d (a body line wrapped)", len(modal), wantRows)
			}

			for i, line := range lines {
				if got := DisplayWidth(line); got != term.w {
					t.Errorf("line %d is %d cells, want the full %d", i, got, term.w)
				}
			}

			for i, line := range modal {
				plain := ansi.Strip(line)
				if i == 0 || i == len(modal)-1 {
					if !strings.ContainsAny(plain, "╭╰") {
						t.Errorf("modal row %d is missing a corner: %q", i, strings.TrimSpace(plain))
					}
					continue
				}
				if got := strings.Count(plain, "│"); got != 2 {
					t.Errorf("modal row %d has %d side borders, want 2: %q", i, got, strings.TrimSpace(plain))
				}
			}
		})
	}
}

func TestRenderPickerModalSwapsNarrowFooter(t *testing.T) {
	content := func(innerW, _ int) string { return FixedWidth(" row", innerW) }

	wide := RenderPickerModal("Title", content, PickerFooter, 120, 40)
	if !strings.Contains(ansi.Strip(wide), "ctrl+p/n") {
		t.Error("wide modal should show the full footer")
	}

	narrow := RenderPickerModal("Title", content, PickerFooter, 60, 14)
	plain := ansi.Strip(narrow)
	if strings.Contains(plain, "ctrl+p/n") {
		t.Error("narrow modal should swap to the compact footer")
	}
	if !strings.Contains(plain, "esc: cancel") {
		t.Errorf("narrow modal lost its footer: %q", plain)
	}
}

func TestRenderPickerModalSmallTerminal(t *testing.T) {
	content := func(innerW, _ int) string { return FixedWidth(" row", innerW) }
	out := RenderPickerModal("Title", content, PickerFooter, 30, 8)
	if !strings.Contains(ansi.Strip(out), "Terminal too small") {
		t.Errorf("expected a fallback message, got %q", ansi.Strip(out))
	}
	for i, line := range strings.Split(out, "\n") {
		if got := DisplayWidth(line); got > 30 {
			t.Errorf("line %d is %d cells, want at most 30", i, got)
		}
	}
}

func TestRenderPickerOverlayDefaultsSize(t *testing.T) {
	content := func(innerW, _ int) string { return FixedWidth(" row", innerW) }
	out := RenderPickerOverlay(content, "Title", 0, 0)
	lines := strings.Split(out, "\n")
	if len(lines) != 40 {
		t.Fatalf("rendered %d lines, want the 40-line default", len(lines))
	}
	for i, line := range lines {
		if got := DisplayWidth(line); got != 120 {
			t.Errorf("line %d is %d cells, want the 120-cell default", i, got)
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
