package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	ta.SetWidth(max(width-4, 10))
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
	m.ta.SetWidth(max(w-4, 10))
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
	if key, ok := msg.(tea.KeyMsg); ok {
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

func (m *commentInputModel) View() string {
	var lines []string
	lines = append(lines, m.ta.View())

	var hint string
	if m.confirmAbort {
		hint = lipgloss.NewStyle().Foreground(tui.ColorWarning).
			Render("  Discard comment? (y/n)")
	} else {
		hint = tui.MutedStyle.Render("  ctrl+s: save   esc: cancel")
	}
	lines = append(lines, hint)
	return strings.Join(lines, "\n")
}
