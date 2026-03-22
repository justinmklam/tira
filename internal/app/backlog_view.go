package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/tira/internal/tui"
)

// Column widths for backlog issue rows.
const (
	blKeyW    = 8
	blEpicW   = 12
	blTypeW   = 8
	blSpW     = 5
	blAssignW = 14
)

func blSummaryWidth(totalWidth int) int {
	w := totalWidth - 2 - blKeyW - 2 - blEpicW - 1 - blTypeW - 1 - blSpW - 1 - blAssignW - 2
	if w < 8 {
		w = 8
	}
	return w
}

func (m blModel) View() string {
	switch m.state {
	case blDetail:
		return m.viewDetail()
	case blParentPicker:
		return m.viewParentPicker()
	case blAssignPicker:
		return m.viewAssignPicker()
	case blStoryPointInput:
		return m.viewStoryPointInput()
	case blStatusPicker:
		return m.viewStatusPicker()
	case blEpicFilterPicker:
		return m.viewEpicFilterPicker()
	case blSprintForm:
		return m.viewSprintForm()
	default:
		return m.viewList()
	}
}

func (m blModel) viewDetail() string {
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

	return renderIssueDetailView(m.detailIssue, m.detailView, width, height, overlayW, innerW)
}

// blColumnHeader returns a dim header row aligned with issue row columns.
func blColumnHeader(width int) string {
	summaryW := blSummaryWidth(width)
	return tui.DimStyle.Render(
		"  " +
			tui.FixedWidth("KEY", blKeyW) + "  " +
			tui.FixedWidth("SUMMARY", summaryW) + "  " +
			tui.FixedWidth("EPIC", blEpicW) + " " +
			tui.FixedWidth("TYPE", blTypeW) + " " +
			tui.FixedWidth("SP", blSpW) + " " +
			tui.FixedWidth("ASSIGNEE", blAssignW),
	)
}

func (m blModel) viewList() string {
	if m.quitting {
		return ""
	}

	width := m.width
	if width == 0 {
		width = 120
	}

	// Calculate pane widths: 65% for list, 35% for sidebar
	listPaneW := tui.ListPaneWidth(width)

	// Top bar spans both panes
	topBar := tui.BoldBlue.Padding(0, 1).Render("Backlog")
	if m.yankMessage != "" {
		topBar += " " + lipgloss.NewStyle().Bold(true).Foreground(tui.ColorGreen).Render(m.yankMessage)
	} else if m.visualMode {
		topBar += " " + lipgloss.NewStyle().Bold(true).Foreground(tui.ColorMagenta).Render("VISUAL")
	} else if m.filterEpic != "" {
		topBar += " " + lipgloss.NewStyle().Foreground(tui.ColorMagenta).Render("epic: "+m.filterEpic)
	} else if m.filter != "" {
		topBar += " " + lipgloss.NewStyle().Foreground(tui.ColorYellow).Render("/ "+m.filter)
	}
	if len(m.cutKeys) > 0 {
		topBar += " " + lipgloss.NewStyle().Foreground(tui.ColorOrange).Render(fmt.Sprintf("✂ %d cut", len(m.cutKeys)))
	}

	// Column header for list pane
	colHeader := blColumnHeader(listPaneW)

	// Visible rows for list pane
	vh := m.viewHeight()
	end := m.offset + vh
	if end > len(m.rows) {
		end = len(m.rows)
	}
	lines := make([]string, 0, vh)
	for i := m.offset; i < end; i++ {
		lines = append(lines, m.renderRow(i, listPaneW))
	}
	for len(lines) < vh {
		lines = append(lines, "")
	}
	listContent := strings.Join(lines, "\n")

	// Header line: column header on left, divider separating sidebar
	div := lipgloss.NewStyle().Foreground(tui.ColorDimmer).Render("│")
	headerLine := lipgloss.NewStyle().Width(listPaneW).Render(colHeader) + div

	// Sidebar content with scroll
	sidebarLines := strings.Split(m.sidebarContent, "\n")
	totalSidebarLines := len(sidebarLines)
	maxSidebarOffset := totalSidebarLines - vh
	if maxSidebarOffset < 0 {
		maxSidebarOffset = 0
	}
	if m.sidebarOffset < 0 {
		m.sidebarOffset = 0
	} else if m.sidebarOffset > maxSidebarOffset {
		m.sidebarOffset = maxSidebarOffset
	}
	sidebarEnd := m.sidebarOffset + vh
	if sidebarEnd > totalSidebarLines {
		sidebarEnd = totalSidebarLines
	}
	visibleSidebarLines := sidebarLines[m.sidebarOffset:sidebarEnd]
	for len(visibleSidebarLines) < vh {
		visibleSidebarLines = append(visibleSidebarLines, "")
	}
	sidebarContent := strings.Join(visibleSidebarLines, "\n")

	// Footer spans both panes
	var footer string
	if m.state == blFilter {
		footer = lipgloss.NewStyle().Foreground(tui.ColorBlue).Render("/") +
			" " + m.filterInput.View() +
			"  " + tui.DimStyle.Render("esc: clear  enter: apply")
	} else {
		hints := []string{
			"e: edit", "c: comment", "o: open", "y: copy", "s: status", "S: story pts",
			"x: cut", "p: paste", ">/<: adj sprint", "B: backlog",
			"/: filter", "F: epic", "ctrl+n: new sprint", "E: edit sprint", "R: refresh",
			"ctrl+d/u: scroll details",
		}
		left := "  " + strings.Join(hints, "   ")
		if n := len(m.allSelected()); n > 0 {
			left = fmt.Sprintf("  %d selected   ", n) + strings.Join(hints, "   ")
		}
		switch {
		case m.state == blLoading:
			spinnerStr := m.loadSpinner.View() + tui.DimStyle.Render(" Loading…")
			leftWidth := listPaneW - lipgloss.Width(spinnerStr) - 2
			footer = tui.DimStyle.Render(tui.FixedWidth(left, leftWidth)) + "  " + spinnerStr
		case m.moving:
			spinnerStr := m.loadSpinner.View() + tui.DimStyle.Render(" Moving…")
			leftWidth := listPaneW - lipgloss.Width(spinnerStr) - 2
			footer = tui.DimStyle.Render(tui.FixedWidth(left, leftWidth)) + "  " + spinnerStr
		default:
			footer = tui.DimStyle.Render(left)
		}
	}

	return topBar + "\n" + headerLine + "\n" + tui.SplitPanes(listContent, sidebarContent, listPaneW, vh) + "\n" + footer
}

func (m blModel) renderRow(idx, width int) string {
	row := m.rows[idx]
	isSelected := idx == m.cursor

	activeGroupIdx := -1
	if m.cursor < len(m.rows) {
		activeGroupIdx = m.rows[m.cursor].groupIdx
	}

	if row.kind == blRowSpacer {
		return ""
	}

	if row.kind == blRowSprint {
		return m.renderSprintRow(row, isSelected, activeGroupIdx, width)
	}

	return m.renderIssueRow(row, isSelected, width)
}

func (m blModel) renderSprintRow(row blRow, isSelected bool, activeGroupIdx, width int) string {
	group := m.groups[row.groupIdx]
	icon := "▼"
	if m.collapsed[row.groupIdx] {
		icon = "▶"
	}

	stateColor := tui.ColorDimmer
	switch group.Sprint.State {
	case "active":
		stateColor = tui.ColorGreen
	case "future":
		stateColor = tui.ColorBlue
	}
	accentColor := stateColor
	if row.groupIdx == activeGroupIdx {
		accentColor = tui.ColorYellow
	}
	accentStyle := lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	accent := accentStyle.Render("▌")

	// Build date range badge: "Mar 1 – Mar 14" or fall back to state label.
	var dateBadge string
	if group.Sprint.StartDate != "" || group.Sprint.EndDate != "" {
		dateBadge = formatSprintDate(group.Sprint.StartDate) + " – " + formatSprintDate(group.Sprint.EndDate)
	} else {
		dateBadge = group.Sprint.State
	}
	stateBadge := lipgloss.NewStyle().Foreground(stateColor).Render(dateBadge)

	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorFg)
	namePart := nameStyle.Render(icon + " " + group.Sprint.Name)

	countStr := fmt.Sprintf("%d issues", len(group.Issues))
	count := tui.DimStyle.Render(countStr)

	left := accent + " " + namePart + "  " + stateBadge
	leftLen := lipgloss.Width(left)
	rightLen := len(countStr)
	fillLen := width - leftLen - rightLen - 2
	if fillLen < 1 {
		fillLen = 1
	}
	fill := lipgloss.NewStyle().Foreground(accentColor).Render(strings.Repeat("─", fillLen))
	line := left + " " + fill + " " + count

	if isSelected {
		highlight := lipgloss.NewStyle().
			Background(tui.ColorBg).
			Foreground(tui.ColorFgBright).
			Bold(true)
		return highlight.Width(width).Render(line)
	}
	return line
}

func (m blModel) renderIssueRow(row blRow, isSelected bool, width int) string {
	issue := m.groups[row.groupIdx].Issues[row.issueIdx]
	summaryW := blSummaryWidth(width)

	key := tui.FixedWidth(issue.Key, blKeyW)
	summary := tui.FixedWidth(issue.Summary, summaryW)

	epicText := issue.EpicName
	if epicText == "" {
		epicText = issue.EpicKey
	}
	if epicText == "" {
		epicText = "—"
	}
	epic := tui.FixedWidth(epicText, blEpicW)

	issueType := tui.FixedWidth(issue.IssueType, blTypeW)

	var spText string
	if issue.StoryPoints > 0 {
		if issue.StoryPoints == float64(int(issue.StoryPoints)) {
			spText = fmt.Sprintf("%d", int(issue.StoryPoints))
		} else {
			spText = fmt.Sprintf("%.1f", issue.StoryPoints)
		}
	} else {
		spText = "—"
	}
	sp := tui.FixedWidth(spText, blSpW)

	assignee := issue.Assignee
	if assignee == "" {
		assignee = "—"
	}
	assignee = tui.FixedWidth(assignee, blAssignW)

	epicColor := tui.EpicColor(issue.EpicKey)
	typeColor := tui.IssueTypeColor(issue.IssueType)

	isChecked := m.allSelected()[issue.Key]
	isCut := m.cutKeys[issue.Key]

	if isSelected {
		bg := tui.SelectedBg
		var cursorStr string
		switch {
		case isCut:
			cursorStr = bg.Bold(true).Foreground(tui.ColorOrange).Render("✂ ")
		case isChecked:
			cursorStr = bg.Foreground(tui.ColorYellow).Render("● ")
		default:
			cursorStr = bg.Render("  ")
		}
		keyColor := tui.ColorWhite
		if isChecked {
			keyColor = tui.ColorYellow
		} else if isCut {
			keyColor = tui.ColorOrange
		}
		keyPart := bg.Bold(true).Foreground(keyColor).Render(key)
		summaryPart := bg.Foreground(tui.ColorWhite).Render("  " + summary + "  ")
		epicStyle := bg.Foreground(tui.ColorDim)
		if epicColor != "" {
			epicStyle = bg.Foreground(epicColor)
		}
		epicPart := epicStyle.Render(epic + " ")
		typePart := bg.Bold(true).Foreground(typeColor).Render(issueType + " ")
		spPart := bg.Foreground(tui.ColorFg).Render(sp + " ")
		assigneePart := bg.Foreground(tui.ColorFg).Render(assignee)
		return cursorStr + keyPart + summaryPart + epicPart + typePart + spPart + assigneePart
	}

	var cursorStr string
	var keyPart string
	switch {
	case isCut:
		cursorStr = lipgloss.NewStyle().Foreground(tui.ColorOrange).Render("✂ ")
		keyPart = lipgloss.NewStyle().Bold(true).Foreground(tui.ColorOrange).Render(key)
	case isChecked:
		cursorStr = lipgloss.NewStyle().Foreground(tui.ColorYellow).Render("● ")
		keyPart = lipgloss.NewStyle().Bold(true).Foreground(tui.ColorYellow).Render(key)
	default:
		cursorStr = "  "
		keyPart = lipgloss.NewStyle().Bold(true).Foreground(tui.ColorBlue).Render(key)
	}
	summaryPart := lipgloss.NewStyle().Foreground(tui.ColorFgBright).Render("  " + summary + "  ")
	epicStyle := lipgloss.NewStyle().Foreground(tui.ColorDim)
	if epicColor != "" {
		epicStyle = lipgloss.NewStyle().Foreground(epicColor)
	}
	epicPart := epicStyle.Render(epic + " ")
	typePart := lipgloss.NewStyle().Bold(true).Foreground(typeColor).Render(issueType + " ")
	spPart := lipgloss.NewStyle().Foreground(tui.ColorDim).Render(sp + " ")
	assigneePart := tui.DimStyle.Render(assignee)
	return cursorStr + keyPart + summaryPart + epicPart + typePart + spPart + assigneePart
}

func (m blModel) viewAssignPicker() string {
	n := len(m.assignTargetKeys)
	noun := "issue"
	if n != 1 {
		noun = "issues"
	}
	title := fmt.Sprintf("Set Assignee  (%d %s)", n, noun)

	return tui.RenderPickerOverlay(
		func(innerW, listH int) string { return m.assignPicker.View(innerW, listH) },
		title,
		m.width,
		m.height,
	)
}

func (m blModel) viewParentPicker() string {
	width := m.width
	if width == 0 {
		width = 120
	}
	height := m.height
	if height == 0 {
		height = 40
	}

	pickerW := width * 2 / 3
	if pickerW < 52 {
		pickerW = 52
	}
	if pickerW > 90 {
		pickerW = 90
	}
	innerW := pickerW - 2 // inside border

	n := len(m.parentTargetKeys)
	noun := "issue"
	if n != 1 {
		noun = "issues"
	}
	title := fmt.Sprintf("Set Parent  (%d %s)", n, noun)
	header := tui.BoldBlue.Padding(0, 1).Width(innerW).
		Render(tui.FixedWidth(title, innerW-2))

	// List rows fit in roughly half the terminal height.
	listH := height/2 - 6
	if listH < 4 {
		listH = 4
	}

	footer := tui.DimStyle.Render("  ↑/↓ ctrl+p/n: navigate   enter: select   esc: cancel")

	body := header + "\n" +
		m.parentPicker.View(innerW, listH) + "\n" +
		tui.DimStyle.Render(strings.Repeat("─", innerW)) + "\n" +
		footer

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBlue).
		Width(innerW).
		Render(body)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func (m blModel) viewStoryPointInput() string {
	width := m.width
	if width == 0 {
		width = 120
	}
	height := m.height
	if height == 0 {
		height = 40
	}

	pickerW := width * 2 / 3
	if pickerW < 52 {
		pickerW = 52
	}
	if pickerW > 90 {
		pickerW = 90
	}
	innerW := pickerW - 2

	n := len(m.storyPointTargetKeys)
	noun := "issue"
	if n != 1 {
		noun = "issues"
	}
	title := fmt.Sprintf("Set Story Points  (%d %s)", n, noun)
	header := tui.BoldBlue.Padding(0, 1).Width(innerW).
		Render(tui.FixedWidth(title, innerW-2))

	inputLine := "  " + m.storyPointInput.View()

	footer := tui.DimStyle.Render("  enter: set   esc: cancel")

	body := header + "\n" +
		inputLine + "\n" +
		tui.DimStyle.Render(strings.Repeat("─", innerW)) + "\n" +
		footer

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBlue).
		Width(innerW).
		Render(body)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func (m blModel) viewEpicFilterPicker() string {
	width := m.width
	if width == 0 {
		width = 120
	}
	height := m.height
	if height == 0 {
		height = 40
	}

	pickerW := width * 2 / 3
	if pickerW < 52 {
		pickerW = 52
	}
	if pickerW > 90 {
		pickerW = 90
	}
	innerW := pickerW - 2

	title := "Filter by Epic"
	if m.filterEpic != "" {
		title = "Filter by Epic  (current: " + m.filterEpic + ")"
	}
	header := tui.BoldBlue.Padding(0, 1).Width(innerW).
		Render(tui.FixedWidth(title, innerW-2))

	listH := height/2 - 6
	if listH < 4 {
		listH = 4
	}

	footer := tui.DimStyle.Render("  ↑/↓ ctrl+p/n: navigate   enter: select   esc: cancel")

	body := header + "\n" +
		m.epicFilterPicker.View(innerW, listH) + "\n" +
		tui.DimStyle.Render(strings.Repeat("─", innerW)) + "\n" +
		footer

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBlue).
		Width(innerW).
		Render(body)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

// formatSprintDate converts "YYYY-MM-DD" to "Jan 2" for compact display.
// Returns the original string if parsing fails.
func formatSprintDate(s string) string {
	if len(s) < 10 {
		return s
	}
	t, err := time.Parse("2006-01-02", s[:10])
	if err != nil {
		return s
	}
	return t.Format("Jan 2")
}

func (m blModel) viewSprintForm() string {
	width := m.width
	if width == 0 {
		width = 120
	}
	height := m.height
	if height == 0 {
		height = 40
	}

	pickerW := width * 2 / 3
	if pickerW < 56 {
		pickerW = 56
	}
	if pickerW > 84 {
		pickerW = 84
	}
	innerW := pickerW - 2

	isEdit := m.sprintFormEditID != 0
	title := "Create Sprint"
	if isEdit {
		title = "Edit Sprint"
	}
	header := tui.BoldBlue.Padding(0, 1).Width(innerW).
		Render(tui.FixedWidth(title, innerW-2))

	const labelW = 16
	labelStyle := tui.DimStyle
	activeStyle := lipgloss.NewStyle().Foreground(tui.ColorFg)

	label := func(text string, active bool) string {
		s := labelStyle
		if active {
			s = activeStyle
		}
		return s.Render(fmt.Sprintf("%-*s", labelW, text))
	}

	nameLine := "  " + label("Name", m.sprintFormField == 0 && !m.sprintFormSubmitting) +
		m.sprintFormName.View()
	startLine := "  " + label("Start Date", m.sprintFormField == 1 && !m.sprintFormSubmitting) +
		m.sprintFormStart.View()
	durLine := "  " + label("Duration (wk)", m.sprintFormField == 2 && !m.sprintFormSubmitting) +
		m.sprintFormDuration.View()

	// Compute and display end date in real time.
	startVal := strings.TrimSpace(m.sprintFormStart.Value())
	durVal := strings.TrimSpace(m.sprintFormDuration.Value())
	endDate := "—"
	if dur, err := strconv.Atoi(durVal); err == nil {
		if e := computeEndDate(startVal, dur); e != "" {
			endDate = e
		}
	}
	endLine := "  " + tui.DimStyle.Render(fmt.Sprintf("%-*s", labelW, "End Date")) +
		tui.DimStyle.Render(endDate)

	var errorLine string
	if m.sprintFormError != "" {
		errorLine = "\n  " + lipgloss.NewStyle().Foreground(tui.ColorRed).Render("✗ "+m.sprintFormError)
	}

	var footer string
	if m.sprintFormSubmitting {
		footer = "  " + m.loadSpinner.View() + tui.DimStyle.Render(" Saving…")
	} else {
		action := "create"
		if isEdit {
			action = "save"
		}
		footer = tui.DimStyle.Render(fmt.Sprintf("  tab/shift+tab: next field   ctrl+s: %s   esc: cancel", action))
	}

	body := header + "\n" +
		"\n" +
		nameLine + "\n" +
		startLine + "\n" +
		durLine + "\n" +
		endLine +
		errorLine + "\n" +
		"\n" +
		tui.DimStyle.Render(strings.Repeat("─", innerW)) + "\n" +
		footer

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBlue).
		Width(innerW).
		Render(body)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func (m blModel) viewStatusPicker() string {
	n := len(m.statusTargetKeys)
	noun := "issue"
	if n != 1 {
		noun = "issues"
	}
	title := fmt.Sprintf("Transition Status  (%d %s)", n, noun)

	return tui.RenderPickerOverlay(
		func(innerW, listH int) string { return m.statusPicker.View(innerW, listH) },
		title,
		m.width,
		m.height,
	)
}
