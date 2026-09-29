package app

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/justinmklam/tira/internal/tui"
)

type commentInputModel struct {
	ta           textarea.Model
	confirmAbort bool
	completed    bool
	aborted      bool
	width        int
	height       int
}

func newCommentInputModel(width, height int) *commentInputModel {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	// Prompt must be cleared before SetWidth: the textarea memoises promptWidth
	// at SetWidth time, so the default "┃ " prompt would otherwise leave a
	// two-cell gutter behind on every line. View then indents each rendered line
	// by one cell, so the block measures exactly `width`.
	ta.Prompt = ""
	ta.SetWidth(max(width-1, 10))
	taH := height - 6
	if taH < 4 {
		taH = 4
	}
	if taH > 20 {
		taH = 20
	}
	ta.SetHeight(taH)
	_ = ta.Focus()
	return &commentInputModel{ta: ta, width: width, height: height}
}

func (m *commentInputModel) setSize(w, h int) {
	m.width = w
	m.height = h
	m.ta.SetWidth(max(w-1, 10))
	taH := h - 6
	if taH < 4 {
		taH = 4
	}
	if taH > 20 {
		taH = 20
	}
	m.ta.SetHeight(taH)
}

func (m *commentInputModel) isDirty() bool {
	return strings.TrimSpace(m.ta.Value()) != ""
}

func (m *commentInputModel) Init() tea.Cmd { return textarea.Blink }

func (m *commentInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if m.confirmAbort {
			switch key.String() {
			case "y", "enter":
				m.aborted = true
			case "n", "esc":
				m.confirmAbort = false
			}
			return m, nil
		}

		switch key.String() {
		case "esc":
			if m.isDirty() {
				m.confirmAbort = true
			} else {
				m.aborted = true
			}
			return m, nil
		case "ctrl+s":
			if strings.TrimSpace(m.ta.Value()) != "" {
				m.completed = true
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	return m, cmd
}

func (m *commentInputModel) View() tea.View {
	var lines []string
	// The prompt is cleared, so View does not reserve its gutter; indent each row
	// by one cell so the block measures exactly m.width, matching the summary row.
	for _, line := range strings.Split(m.ta.View(), "\n") {
		lines = append(lines, " "+line)
	}

	var hint string
	if m.confirmAbort {
		hint = lipgloss.NewStyle().Foreground(tui.ColorWarning).
			Render("  Discard comment? (y/n)")
	} else {
		hint = tui.MutedStyle.Render("  ctrl+s: save   esc: cancel")
	}
	lines = append(lines, hint)
	return tea.NewView(strings.Join(lines, "\n"))
}
