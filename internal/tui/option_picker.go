package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc":
		m.Aborted = true
	case "enter":
		if len(m.Items) == 0 {
			return m, nil // nothing selectable
		}
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
// Every line is at most innerW display cells wide and the highlighted row is
// always inside the rendered window. This signature is compatible with
// RenderPickerOverlay's pickerView parameter.
func (m OptionPickerModel) View(innerW, maxRows int) string {
	if innerW < 1 {
		innerW = 1
	}
	if maxRows < 1 {
		maxRows = 1
	}
	if len(m.Items) == 0 {
		return MutedStyle.Render(FixedWidth("  (no options)", innerW))
	}

	cursor := Clamp(m.Cursor, 0, len(m.Items)-1)
	start := 0
	if cursor >= maxRows {
		start = cursor - maxRows + 1
	}
	if last := len(m.Items) - maxRows; start > last {
		start = last
	}
	if start < 0 {
		start = 0
	}
	end := min(start+maxRows, len(m.Items))

	// "▶ " prefix (2) leaves innerW-2 cells for the label, so the row is
	// exactly innerW wide.
	var lines []string
	for i := start; i < end; i++ {
		label := FixedWidth(SanitizeRow(m.Items[i]), innerW-2)
		if i == cursor {
			lines = append(lines, lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render("▶ "+label))
		} else {
			lines = append(lines, MutedStyle.Render("  "+label))
		}
	}
	return strings.Join(lines, "\n")
}
