package app

import (
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/justinmklam/tira/internal/tui"
)

// TestViewCommentFormBorderTitle pins the comment modal's shape: the title draws
// the issue key and summary in the frame's top border, and a newline in either
// never splits the frame.
func TestViewCommentFormBorderTitle(t *testing.T) {
	const w, h = 120, 40
	overlayW, overlayH := tui.OverlaySize(w, h)

	form := newCommentInputModel(overlayW-4, overlayH-4)
	form.ta.SetValue("first line")
	m := boardModel{
		activeView:     viewComment,
		commentKey:     "DEMO-1",
		commentSummary: "evil\nsummary \x1b[31mred",
		commentForm:    form,
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

	// The sanitised summary rides in the top border beside the key, exactly once,
	// and no longer occupies a body row inside the frame.
	if !strings.Contains(top, "Add Comment · DEMO-1 · evil summary") {
		t.Errorf("top border does not carry the issue summary: %q", top)
	}
	rows := 0
	for _, line := range lines {
		if strings.Contains(ansi.Strip(line), "evil summary") {
			rows++
		}
	}
	if rows != 1 {
		t.Errorf("sanitised summary appears on %d rows, want 1 (the top border only)", rows)
	}
	// A blank padding row separates the border title from the textarea's first
	// content row.
	if body := ansi.Strip(lines[topIdx+1]); strings.Contains(body, "first line") {
		t.Errorf("text starts immediately under the top border, no padding row: %q", body)
	}
	if body := ansi.Strip(lines[topIdx+2]); !strings.Contains(body, "first line") {
		t.Errorf("textarea content is not two rows below the top border: %q", body)
	}
}

// TestCommentFormHasNoTextareaPromptBars pins that the comment textarea, like
// the edit form's, carries no `┃` prompt gutter.
func TestCommentFormHasNoTextareaPromptBars(t *testing.T) {
	m := newCommentInputModel(98, 34)
	plain := ansi.Strip(m.View().Content)
	for _, bar := range []string{"┃", "▌"} {
		if strings.Contains(plain, bar) {
			t.Errorf("comment form still renders the textarea prompt bar %q", bar)
		}
	}
}

// TestCommentFormTextareaMeasuresWidth pins the prompt-free width rule: every
// textarea row measures exactly the width setSize was given, so the modal frame
// never has to pad or clamp it.
func TestCommentFormTextareaMeasuresWidth(t *testing.T) {
	const w, h = 98, 34
	m := newCommentInputModel(w, h)
	lines := strings.Split(m.View().Content, "\n")
	if got, want := len(lines), m.ta.Height()+1; got != want {
		t.Fatalf("comment form rendered %d rows, want %d", got, want)
	}
	for i, line := range lines[:len(lines)-1] {
		if got := tui.DisplayWidth(line); got != w {
			t.Errorf("textarea row %d width = %d, want %d: %q", i, got, w, ansi.Strip(line))
		}
	}
}
