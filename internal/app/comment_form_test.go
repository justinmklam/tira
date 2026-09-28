package app

import (
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/justinmklam/tira/internal/tui"
)

// TestViewCommentFormBorderTitle pins the comment modal's shape: the title is
// drawn in the frame's top border, the sanitised issue summary is the only row
// under it, and a newline in either never splits the frame.
func TestViewCommentFormBorderTitle(t *testing.T) {
	const w, h = 120, 40
	overlayW, overlayH := tui.OverlaySize(w, h)

	m := boardModel{
		activeView:     viewComment,
		commentKey:     "DEMO-1",
		commentSummary: "evil\nsummary \x1b[31mred",
		commentForm:    newCommentInputModel(overlayW-4, overlayH-4),
	}

	out := m.viewCommentForm(w, h)
	lines := strings.Split(out, "\n")
	if len(lines) != h {
		t.Fatalf("comment modal rendered %d lines, want %d — a newline split the frame", len(lines), h)
	}

	topIdx := -1
	for i, line := range lines {
		if strings.Contains(ansi.Strip(line), "╭─ Add Comment") {
			topIdx = i
			break
		}
	}
	if topIdx < 0 {
		t.Fatal("modal top border with the comment title not found")
	}

	top := ansi.Strip(lines[topIdx])
	if !strings.Contains(top, "Add Comment · DEMO-1") {
		t.Errorf("top border does not carry the key: %q", top)
	}
	for i, line := range lines {
		for _, r := range ansi.Strip(line) {
			if unicode.IsControl(r) {
				t.Errorf("line %d contains control rune %U: %q", i, r, ansi.Strip(line))
			}
		}
	}

	if got, want := tui.DisplayWidth(strings.TrimSpace(lines[topIdx])), overlayW-2; got != want {
		t.Errorf("top border width = %d, want %d", got, want)
	}

	// The sanitised summary sits directly under the top border, exactly once.
	rows := 0
	for _, line := range lines {
		if strings.Contains(ansi.Strip(line), "evil summary") {
			rows++
		}
	}
	if rows != 1 {
		t.Errorf("sanitised summary appears on %d rows, want 1", rows)
	}
	if second := ansi.Strip(lines[topIdx+1]); !strings.Contains(second, "│ evil summary") {
		t.Errorf("summary is not the framed row under the top border: %q", second)
	}
}
