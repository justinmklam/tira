package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
)

func renderKanbanCard(issue models.Issue, selected bool, width int) []string {
	fill := lipgloss.NewStyle()
	if selected {
		fill = tui.SurfaceBg
	}

	typeColor := tui.IssueTypeColor(issue.IssueType)
	bar := fill.Foreground(typeColor).Render("▌")
	prefix := bar + fill.Render(" ")
	contentW := width - lipgloss.Width(prefix)
	if contentW < 1 {
		contentW = 1
	}

	renderText := func(text string, maxWidth int) string {
		return tui.TruncateWidth(tui.SanitizeRow(text), maxWidth)
	}

	summary := renderText(issue.Summary, contentW)
	epic := issue.EpicName
	if epic == "" {
		epic = "—"
	}
	epic = renderText(epic, contentW)

	points := tui.FormatStoryPoints(issue.StoryPoints) + " SP"
	assigneeText := "—"
	daysText := ""
	separator := " · "
	metadataKey := tui.SanitizeRow(issue.Key)
	days := tui.DaysInColumn(issue.StatusChangedDate)
	metadataPrefix := metadataKey + separator + points
	if days > 0 {
		daysText = fmt.Sprintf("%dd", days)
		metadataPrefix += separator + daysText
	}
	assigneeWidth := contentW - lipgloss.Width(metadataPrefix) - lipgloss.Width(separator)
	if assigneeWidth < 1 {
		assigneeWidth = 1
	}
	if issue.Assignee != "" {
		assigneeText = renderText(issue.Assignee, assigneeWidth)
	}

	epicColor := tui.EpicColor(issue.EpicKey)
	if epicColor == nil {
		epicColor = tui.ColorMuted
	}

	// The card is deliberately assembled from individually filled segments so
	// the selected background survives the lipgloss resets between colours.
	summaryLine := prefix + fill.Foreground(tui.ColorForegroundBright).Render(summary)
	epicLine := prefix + fill.Foreground(epicColor).Render(epic)
	metadataLine := prefix
	metadataSeparator := fill.Foreground(tui.ColorMuted).Render(separator)
	metadataParts := []string{
		fill.Foreground(tui.ColorMuted).Render(metadataKey),
		fill.Foreground(tui.ColorMuted).Render(points),
	}
	if daysText != "" {
		metadataParts = append(metadataParts, fill.Foreground(tui.DaysColor(days)).Render(daysText))
	}
	metadataParts = append(metadataParts, fill.Foreground(tui.ColorMuted).Render(assigneeText))
	metadataLine += strings.Join(metadataParts, metadataSeparator)

	return []string{summaryLine, epicLine, metadataLine}
}

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
			for _, line := range renderKanbanCard(issue, isSelected, contentW) {
				cardStyle := lipgloss.NewStyle().Width(contentW)
				if isSelected {
					cardStyle = cardStyle.Background(tui.ColorSurface).Foreground(tui.ColorForeground)
				}
				lines = append(lines, cardStyle.Render(line))
			}
		}

		renderedCols = append(renderedCols, colStyle.Render(strings.Join(lines, "\n")))
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, renderedCols...)

	// The top pad and tab strip always render, and the columns' own top border
	// now provides the rule beneath the tabs, which is why availableIssueLines
	// deducts them unconditionally.
	boardContent := boardTopPad + tui.TabStrip(1, m.sprintName, width) + "\n" + board

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
