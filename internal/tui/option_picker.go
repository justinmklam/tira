package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// OptionPickerModel is a simple static list picker with no text input.
// Navigate with j/k or ↑↓, confirm with enter, cancel with esc.
type OptionPickerModel struct {
	Items     []string
	Cursor    int
	Completed bool
	Aborted   bool
}

// NewOptionPickerModel creates a picker pre-seeded with items.
// The cursor is positioned on the first item whose value case-insensitively
// matches initialValue; otherwise it starts at 0.
func NewOptionPickerModel(items []string, initialValue string) OptionPickerModel {
	cursor := 0
	for i, item := range items {
		if strings.EqualFold(item, initialValue) {
			cursor = i
			break
		}
	}
	return OptionPickerModel{
		Items:  items,
		Cursor: cursor,
	}
}

// SelectedItem returns the currently highlighted item, or "" if items is empty.
func (m OptionPickerModel) SelectedItem() string {
	if m.Cursor < 0 || m.Cursor >= len(m.Items) {
		return ""
	}
	return m.Items[m.Cursor]
}

// Update handles key input.
func (m OptionPickerModel) Update(msg tea.Msg) (OptionPickerModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.Aborted = true
	case "enter":
		m.Completed = true
	case "j", "down", "ctrl+n":
		if m.Cursor < len(m.Items)-1 {
			m.Cursor++
		}
	case "k", "up", "ctrl+p":
		if m.Cursor > 0 {
			m.Cursor--
		}
	}
	return m, nil
}

// View renders the list content sized to innerW columns and at most maxRows rows.
// This signature is compatible with RenderPickerOverlay's pickerView parameter.
func (m OptionPickerModel) View(innerW, maxRows int) string {
	if len(m.Items) == 0 {
		return DimStyle.Render("  (no options)")
	}

	start := 0
	if m.Cursor >= maxRows {
		start = m.Cursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(m.Items) {
		end = len(m.Items)
	}

	labelW := innerW - 4 // reserve 2 chars for "▶ " / "  " prefix + 2 padding
	var lines []string
	for i := start; i < end; i++ {
		label := FixedWidth(m.Items[i], labelW)
		if i == m.Cursor {
			lines = append(lines, lipgloss.NewStyle().Foreground(ColorBlue).Bold(true).Render("▶ "+label))
		} else {
			lines = append(lines, DimStyle.Render("  "+label))
		}
	}
	return strings.Join(lines, "\n")
}
