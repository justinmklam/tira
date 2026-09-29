package app

import (
	"strings"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/justinmklam/tira/internal/tui"
)

func (m boardModel) View() tea.View {
	w, h := m.width, m.height
	if w == 0 {
		w = 120
	}
	if h == 0 {
		h = 40
	}

	var content string
	switch m.activeView {
	case viewEditLoading:
		msg := m.editSpinner.View() + tui.MutedStyle.Render(" Fetching issue…")
		content = lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, msg)

	case viewEdit:
		content = m.viewEditForm(w, h)

	case viewEditSaving:
		msg := m.editSpinner.View() + tui.MutedStyle.Render(" Saving…")
		content = lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, msg)

	case viewCreateLoading:
		msg := m.editSpinner.View() + tui.MutedStyle.Render(" Loading…")
		content = lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, msg)

	case viewCreate:
		content = m.viewEditForm(w, h)

	case viewCreateSaving:
		msg := m.editSpinner.View() + tui.MutedStyle.Render(" Creating issue…")
		content = lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, msg)

	case viewAssigneePicker:
		content = m.viewAssigneePickerOverlay(w, h)

	case viewTypePicker:
		content = m.viewTypePickerOverlay(w, h)

	case viewPriorityPicker:
		content = m.viewPriorityPickerOverlay(w, h)

	case viewHelp:
		content = m.viewHelpOverlay(w, h)

	case viewComment:
		content = m.viewCommentForm(w, h)

	case viewCommentSaving:
		msg := m.editSpinner.View() + tui.MutedStyle.Render(" Saving comment…")
		content = lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, msg)

	case ViewKanban:
		v := m.kanban.View()
		v.AltScreen = true
		return v

	case ViewEpics:
		v := m.epics.View()
		v.AltScreen = true
		return v

	default:
		v := m.backlog.View()
		v.AltScreen = true
		return v
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// viewEditForm renders the create/edit modal with its title in the frame's top
// border. innerW is that frame's outer width, so the form body it wraps keeps
// the overlayW-4 width board.go sizes it with.
func (m boardModel) viewEditForm(w, h int) string {
	if m.editForm == nil {
		return ""
	}
	overlayW, _ := tui.OverlaySize(w, h)
	innerW := overlayW - 2

	title := "New Issue"
	if m.activeView != viewCreate {
		title = "Edit " + m.editKey
	}

	// The form's View carries a trailing newline, which would render an extra
	// blank row inside the modal; drop it before wrapping the body in the frame.
	body := strings.TrimSuffix(m.editForm.View().Content, "\n")
	if m.editErr != "" {
		body += "\n\n" + lipgloss.NewStyle().Foreground(tui.ColorError).Render("  "+m.editErr)
	}

	// Frame truncates an over-wide title but does not strip control characters, so
	// the dynamic title is sanitised here rather than inside Frame.
	modal := tui.Frame(tui.SanitizeRow(title), body, innerW, 0, tui.ColorAccent, tui.ColorForegroundBright)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal)
}

func (m boardModel) viewAssigneePickerOverlay(w, h int) string {
	return tui.RenderPickerOverlay(
		func(innerW, listH int) string { return m.assigneePicker.View(innerW, listH) },
		"Set Assignee",
		w,
		h,
	)
}

func (m boardModel) viewTypePickerOverlay(w, h int) string {
	return tui.RenderPickerOverlay(
		func(innerW, listH int) string { return m.typePicker.View(innerW, listH) },
		"Set Issue Type",
		w,
		h,
	)
}

func (m boardModel) viewPriorityPickerOverlay(w, h int) string {
	return tui.RenderPickerOverlay(
		func(innerW, listH int) string { return m.priorityPicker.View(innerW, listH) },
		"Set Priority",
		w,
		h,
	)
}

func (m boardModel) viewHelpOverlay(w, h int) string {
	overlayW, overlayH := tui.HelpOverlaySize(w, h)
	innerW := overlayW - 2
	innerH := overlayH - 2 // account for border only

	// Get the help content
	helpContent := m.helpModel.View(innerW, innerH)

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorAccent).
		Width(innerW).
		Render(helpContent)

	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal)
}

// viewCommentForm renders the comment modal with its title in the frame's top
// border: the issue key followed by the issue summary, so no body row is spent
// on context.
func (m boardModel) viewCommentForm(w, h int) string {
	if m.commentForm == nil {
		return ""
	}
	overlayW, _ := tui.OverlaySize(w, h)
	innerW := overlayW - 2

	title := "Add Comment · " + m.commentKey
	if summary := tui.SanitizeRow(m.commentSummary); summary != "" {
		title += " · " + summary
	}

	body := "\n" + strings.TrimSuffix(m.commentForm.View().Content, "\n")
	if m.commentErr != "" {
		body += "\n" + lipgloss.NewStyle().Foreground(tui.ColorError).Render("  "+m.commentErr)
	}

	// Frame truncates an over-wide title but does not strip control characters, so
	// the dynamic title is sanitised here rather than inside Frame.
	modal := tui.Frame(tui.SanitizeRow(title), body, innerW, 0, tui.ColorAccent, tui.ColorForegroundBright)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal)
}
