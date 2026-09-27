package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/justinmklam/tira/internal/tui"
)

func (m kanbanModel) View() tea.View {
	switch m.state {
	case stateDetail:
		return tea.NewView(m.viewDetail())
	case stateAssignPicker:
		return tea.NewView(m.viewAssignPicker())
	case stateStatusPicker:
		return tea.NewView(m.viewStatusPicker())
	case stateLinkPicker:
		return tea.NewView(viewLinkedItemsPicker(m.linkPicker.picker, m.width, m.height))
	default:
		return tea.NewView(m.viewBoard())
	}
}

func (m kanbanModel) viewAssignPicker() string {
	return tui.RenderPickerOverlay(
		func(innerW, listH int) string { return m.assignPicker.View(innerW, listH) },
		"Set Assignee",
		m.width,
		m.height,
	)
}

func (m kanbanModel) viewStatusPicker() string {
	return tui.RenderPickerOverlay(
		func(innerW, listH int) string { return m.statusPicker.View(innerW, listH) },
		"Transition Status",
		m.width,
		m.height,
	)
}

func (m kanbanModel) viewDetail() string {
	if m.detailIssue == nil {
		return ""
	}

	width := m.width
	if width == 0 {
		width = 120
	}
	height := m.height
	if height == 0 {
		height = 40
	}

	overlayW, _ := tui.OverlaySize(width, height)
	innerW := overlayW - 2

	return renderIssueDetailView(
		m.detailView,
		"  e: edit   c: comment   L: linked items   o: open in browser   esc/q: back   j/k: scroll",
		width, height, overlayW, innerW,
	)
}

// kanbanHints are the kanban footer hints, most-used first: they are dropped
// from the tail when the footer is too narrow.
var kanbanHints = []string{
	"hjkl/arrows navigate", "enter view", "e edit", "c comment", "s status",
	"L linked", "o open", "tab backlog", "q quit",
}

func (m kanbanModel) viewBoard() string {
	if m.quitting || len(m.columns) == 0 {
		return ""
	}

	width := m.width
	if width == 0 {
		width = 120
	}
	height := m.height
	if height == 0 {
		height = 40
	}

	numCols := len(m.columns)
	colWidth := width / numCols
	if colWidth < 24 {
		colWidth = 24
	}
	// lipgloss Width is the total block width, border included, so a column of
	// Width(colWidth) has a colWidth-2 body. The kanban column carries no padding
	// of its own: content is inset by one cell manually, which lets the cursor
	// fill span the whole body from border to border.
	contentW := colWidth - 2
	innerW := contentW - 2

	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorAccent)
	assigneeStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)
	daysStyle := lipgloss.NewStyle().Bold(true)

	avail := m.availableIssueLines()

	var renderedCols []string
	for ci, col := range m.columns {
		// Each column carries its own status colour; the active column is
		// distinguished by weight (bold title, full-strength border) rather than
		// by a different hue.
		statusColor := tui.StatusColor(col.name)
		if statusColor == nil {
			statusColor = tui.ColorMuted
		}
		active := ci == m.colIdx
		borderColor := tui.ColorSubtle
		if active {
			borderColor = statusColor
		}
		colStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Width(colWidth)

		var lines []string

		title := strings.ToUpper(col.name) + fmt.Sprintf(" (%d)", len(col.issues))
		if active {
			lines = append(lines, tui.SectionHeader(" "+title, statusColor, contentW))
		} else {
			lines = append(lines, tui.SectionHeaderRegular(" "+title, statusColor, contentW))
		}
		lines = append(lines, tui.MutedStyle.Render(" "+strings.Repeat("─", innerW)+" "))

		if len(col.issues) == 0 {
			lines = append(lines, " "+tui.EmptyState("empty", innerW)+" ")
		}

		scroll := 0
		if ci < len(m.colScrolls) {
			scroll = m.colScrolls[ci]
		}

		linesUsed := 0
		for ri := scroll; ri < len(col.issues); ri++ {
			issue := col.issues[ri]
			ilines := kanbanItemLines(issue)
			if linesUsed+ilines > avail {
				break
			}
			linesUsed += ilines

			isSelected := ci == m.colIdx && ri == m.rowIdxs[ci]
			// One cell of inset on each side, plus the two-cell card indent.
			maxSummary := innerW - 2
			if maxSummary < 1 {
				maxSummary = 1
			}
			runes := []rune(tui.SanitizeRow(issue.Summary))
			summary := string(runes)
			if len(runes) > maxSummary {
				summary = string(runes[:maxSummary-1]) + "…"
			}

			// Calculate days in column and get color
			days := tui.DaysInColumn(issue.StatusChangedDate)
			daysColor := tui.DaysColor(days)
			daysStr := fmt.Sprintf("%dd", days)

			// Format assignee
			assigneeStr := ""
			if issue.Assignee != "" {
				assigneeStr = issue.Assignee
			}

			if isSelected {
				// D1: no per-card box or status gutter. The cursor card is a
				// ColorSurface fill with a ▶ marker, sized to the whole column body
				// so the highlight runs border to border rather than just behind
				// the text.
				cardStyle := lipgloss.NewStyle().
					Background(tui.ColorSurface).
					Foreground(tui.ColorForeground).
					Width(contentW)
				lines = append(lines,
					cardStyle.Render(" ▶ "+issue.Key),
					cardStyle.Render("   "+summary),
				)
				if assigneeStr != "" || days > 0 {
					var metaParts []string
					if assigneeStr != "" {
						metaParts = append(metaParts, assigneeStr)
					}
					if days > 0 {
						metaParts = append(metaParts, daysStr)
					}
					lines = append(lines, cardStyle.Render("   "+strings.Join(metaParts, " • ")))
				}
			} else {
				lines = append(lines,
					"   "+keyStyle.Render(issue.Key),
					"   "+tui.MutedStyle.Render(summary),
				)
				if assigneeStr != "" || days > 0 {
					var metaParts []string
					if assigneeStr != "" {
						metaParts = append(metaParts, assigneeStyle.Render(assigneeStr))
					}
					if days > 0 {
						metaParts = append(metaParts, daysStyle.Foreground(daysColor).Render(daysStr))
					}
					lines = append(lines, "   "+tui.MutedStyle.Render(strings.Join(metaParts, " • ")))
				}
			}
		}

		renderedCols = append(renderedCols, colStyle.Render(strings.Join(lines, "\n")))
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, renderedCols...)

	// The top pad, tab strip, and divider always render, which is why
	// availableIssueLines deducts all three unconditionally.
	boardContent := boardTopPad + tui.TabStrip(1, m.sprintName, width) + "\n" + tui.TabDivider(width) + "\n" + board

	var footerStr string
	if m.state == stateLoading || m.linkPickerKey != "" {
		spinnerStr := m.loadSpinner.View() + tui.MutedStyle.Render(" Loading…")
		footerStr = tui.FooterHints(kanbanHints, width-lipgloss.Width(spinnerStr)-2) + "  " + spinnerStr
	} else {
		footerStr = tui.FooterHints(kanbanHints, width)
	}

	// Create footer line at full width
	footerLine := lipgloss.NewStyle().Width(width).Render(footerStr)

	// Place board at top, footer at bottom using vertical join with spacing
	// Calculate how many blank lines between board and footer
	boardHeight := lipgloss.Height(boardContent)
	spacing := height - boardHeight - 1
	if spacing < 0 {
		spacing = 0
	}

	var result string
	if spacing > 0 {
		blankLines := strings.Repeat("\n", spacing)
		result = boardContent + blankLines + "\n" + footerLine
	} else {
		result = boardContent + "\n" + footerLine
	}

	return result
}
