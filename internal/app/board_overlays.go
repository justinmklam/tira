package app

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/tira/internal/tui"
)

func (m boardModel) View() string {
	w, h := m.width, m.height
	if w == 0 {
		w = 120
	}
	if h == 0 {
		h = 40
	}

	switch m.activeView {
	case viewEditLoading:
		msg := m.editSpinner.View() + tui.DimStyle.Render(" Fetching issue…")
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, msg)

	case viewEdit:
		return m.viewEditForm(w, h)

	case viewEditSaving:
		msg := m.editSpinner.View() + tui.DimStyle.Render(" Saving…")
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, msg)

	case viewCreateLoading:
		msg := m.editSpinner.View() + tui.DimStyle.Render(" Loading…")
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, msg)

	case viewCreate:
		return m.viewEditForm(w, h)

	case viewCreateSaving:
		msg := m.editSpinner.View() + tui.DimStyle.Render(" Creating issue…")
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, msg)

	case viewAssigneePicker:
		return m.viewAssigneePickerOverlay(w, h)

	case viewTypePicker:
		return m.viewTypePickerOverlay(w, h)

	case viewPriorityPicker:
		return m.viewPriorityPickerOverlay(w, h)

	case viewHelp:
		return m.viewHelpOverlay(w, h)

	case viewComment:
		return m.viewCommentForm(w, h)

	case viewCommentSaving:
		msg := m.editSpinner.View() + tui.DimStyle.Render(" Saving comment…")
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, msg)

	case ViewKanban:
		return m.kanban.View()

	default:
		return m.backlog.View()
	}
}

func (m boardModel) viewEditForm(w, h int) string {
	if m.editForm == nil {
		return ""
	}
	overlayW, _ := tui.OverlaySize(w, h)
	innerW := overlayW - 2

	var titleStr string
	switch m.activeView {
	case viewCreate:
		if m.createSprintID == 0 {
			titleStr = "New Issue  (backlog)"
		} else {
			titleStr = "New Issue"
		}
	default:
		titleStr = m.editKey
		if m.editIssue != nil {
			titleStr = m.editIssue.Key + "  " + m.editIssue.Summary
		}
	}
	header := tui.BoldBlue.Padding(0, 1).Width(innerW).
		Render(tui.FixedWidth(titleStr, innerW-2))

	body := header + "\n" + m.editForm.View()
	if m.editErr != "" {
		body += "\n" + lipgloss.NewStyle().Foreground(tui.ColorRed).Render("  "+m.editErr)
	}

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBlue).
		Width(innerW).
		Render(body)

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
		BorderForeground(tui.ColorBlue).
		Width(innerW).
		Render(helpContent)

	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal)
}

func (m boardModel) viewCommentForm(w, h int) string {
	if m.commentForm == nil {
		return ""
	}
	overlayW, _ := tui.OverlaySize(w, h)
	innerW := overlayW - 2

	titleStr := "Add Comment → " + m.commentKey + "  " + m.commentSummary
	header := tui.BoldBlue.Padding(0, 1).Width(innerW).
		Render(tui.FixedWidth(titleStr, innerW-2))

	body := header + "\n" + m.commentForm.View()
	if m.commentErr != "" {
		body += "\n" + lipgloss.NewStyle().Foreground(tui.ColorRed).Render("  "+m.commentErr)
	}

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBlue).
		Width(innerW).
		Render(body)

	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal)
}
