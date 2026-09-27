package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/justinmklam/tira/internal/tui"
)

// Column widths for backlog issue rows.
const (
	blKeyW    = 10
	blEpicW   = 12
	blTypeW   = 10
	blSpW     = 5
	blAssignW = 14
)

// Fixed cell widths shared by the header and issue rows.
const (
	blLeadW = 3 // selection marker + status glyph + space
	blGapW  = 2 // gap between the key and the summary
	blPrioW = 1 // priority glyph
)

// blCols is the set of backlog columns visible at a given list-pane width.
type blCols struct {
	summaryW int
	epic     bool
	typ      bool
	prio     bool
	sp       bool
	assignW  int // 14 = initials + name, 3 = initials only, 0 = hidden
	compact  bool
}

// blLayoutFor is the single source of truth for backlog column geometry: both
// blColumnHeader and renderIssueRow derive their cells from it, so a rendered
// row always measures exactly paneW cells. Columns are bought in value order
// (type, priority, story points, assignee, epic) and only while the summary
// keeps its floor, which is why a narrow pane drops columns instead of
// squeezing the summary.
func blLayoutFor(paneW int) blCols {
	// Compact mode keeps a glyph cluster: key, summary, type, initials, prio.
	// Its fixed overhead is lead + key + gap + (type+sep) + (initials+sep) + (prio+sep).
	const compactOverhead = blLeadW + blKeyW + blGapW + 2 + 4 + 2
	if paneW < 52 {
		summaryW := paneW - compactOverhead
		if summaryW < 8 {
			summaryW = 8
		}
		return blCols{summaryW: summaryW, compact: true}
	}

	overhead := blLeadW + blKeyW + blGapW
	layout := blCols{}
	// try adds a column if the summary can still keep its floor. floor(EPIC) is
	// higher because epic names are long and mostly duplicated elsewhere.
	try := func(w, sep, floor int) bool {
		if overhead+sep+w > paneW-floor {
			return false
		}
		overhead += sep + w
		return true
	}

	if try(blTypeW, 1, 24) {
		layout.typ = true
	}
	if try(blPrioW, 1, 24) {
		layout.prio = true
	}
	if try(blSpW, 1, 24) {
		layout.sp = true
	}
	if try(blAssignW, 1, 24) {
		layout.assignW = blAssignW
	} else if try(3, 1, 24) {
		layout.assignW = 3
	}
	if try(blEpicW, 2, 32) {
		layout.epic = true
	}

	layout.summaryW = paneW - overhead
	return layout
}

func blSummaryWidth(totalWidth int) int {
	return blLayoutFor(totalWidth).summaryW
}

// blColsLabel returns a stable description of the visible columns, used by the
// golden layout test.
func blColsLabel(l blCols) string {
	if l.compact {
		return "KEY SUMMARY type initials prio"
	}
	var parts []string
	if l.epic {
		parts = append(parts, "EPIC")
	}
	if l.typ {
		parts = append(parts, "TYPE")
	}
	if l.prio {
		parts = append(parts, "P")
	}
	if l.sp {
		parts = append(parts, "SP")
	}
	if l.assignW > 0 {
		parts = append(parts, fmt.Sprintf("OWN(%d)", l.assignW))
	}
	return strings.Join(parts, " ")
}

func (m blModel) View() tea.View {
	switch m.state {
	case blDetail:
		return tea.NewView(m.viewDetail())
	case blParentPicker:
		return tea.NewView(m.viewParentPicker())
	case blAssignPicker:
		return tea.NewView(m.viewAssignPicker())
	case blStoryPointInput:
		return tea.NewView(m.viewStoryPointInput())
	case blStatusPicker:
		return tea.NewView(m.viewStatusPicker())
	case blEpicFilterPicker:
		return tea.NewView(m.viewEpicFilterPicker())
	case blSprintPicker:
		return tea.NewView(m.viewSprintPicker())
	case blSprintForm:
		return tea.NewView(m.viewSprintForm())
	case blLinkPicker:
		return tea.NewView(viewLinkedItemsPicker(m.linkPicker.picker, m.width, m.height))
	default:
		return tea.NewView(m.viewList())
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

	return renderIssueDetailView(
		m.detailView,
		"  e: edit   c: comment   L: linked items   o: open in browser   esc/q: back   j/k: scroll",
		width, height, overlayW, innerW,
	)
}

// blColumnHeader returns a dim header row aligned with issue row columns. The
// columns come from blLayoutFor, so the header can never drift from the rows.
func blColumnHeader(width int) string {
	l := blLayoutFor(width)
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", blLeadW))
	b.WriteString(tui.FixedWidth("KEY", blKeyW))
	b.WriteString(strings.Repeat(" ", blGapW))
	b.WriteString(tui.FixedWidth("SUMMARY", l.summaryW))
	if l.compact {
		b.WriteString(" " + tui.FixedWidth("T", 1))
		b.WriteString(" " + tui.FixedWidth("O", 3))
		b.WriteString(" " + tui.FixedWidth("P", blPrioW))
	} else {
		if l.epic {
			b.WriteString("  " + tui.FixedWidth("EPIC", blEpicW))
		}
		if l.typ {
			b.WriteString(" " + tui.FixedWidth("TYPE", blTypeW))
		}
		if l.prio {
			b.WriteString(" " + tui.FixedWidth("P", blPrioW))
		}
		if l.sp {
			b.WriteString(" " + tui.FixedWidth("SP", blSpW))
		}
		if l.assignW > 0 {
			label := "ASSIGNEE"
			if l.assignW == 3 {
				label = "OWN"
			}
			b.WriteString(" " + tui.FixedWidth(label, l.assignW))
		}
	}
	return tui.SectionHeader(b.String(), tui.ColorMuted, width)
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

	// Tab strip spans both panes; the transient badges stay coloured.
	topBar := tui.TabStripStyled(0, m.topDetail(), width)

	// Column header for list pane
	colHeader := blColumnHeader(listPaneW)

	// Pane geometry is the single source of truth for the split view's height
	// budget: the list body is the column header plus viewHeight() issue rows and
	// the detail body is the sidebar sliced to viewHeight()+1 rows, so each is
	// paneH-2 rows tall. The top pad, tab strip, pane (paneH = height-3), and
	// footer sum to exactly height.
	vh := m.viewHeight()
	paneH := vh + 3

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
	listBody := colHeader + "\n" + strings.Join(lines, "\n")

	// Sidebar content with scroll
	sidebarLines := strings.Split(m.sidebarContent, "\n")
	totalSidebarLines := len(sidebarLines)
	sidebarEnd := m.sidebarOffset + vh + 1
	if sidebarEnd > totalSidebarLines {
		sidebarEnd = totalSidebarLines
	}
	visibleSidebarLines := sidebarLines[m.sidebarOffset:sidebarEnd]
	for len(visibleSidebarLines) < vh+1 {
		visibleSidebarLines = append(visibleSidebarLines, "")
	}
	detailBody := strings.Join(visibleSidebarLines, "\n")

	// Footer spans both panes
	var footer string
	switch m.state {
	case blFilter:
		footer = lipgloss.NewStyle().Foreground(tui.ColorAccent).Render("/") +
			" " + m.filterInput.View() +
			"  " + tui.MutedStyle.Render("esc: clear  enter: apply")
	case blKeySearch:
		footer = lipgloss.NewStyle().Foreground(tui.ColorAccent).Render("f") +
			" " + m.keySearchInput.View() +
			"  " + tui.MutedStyle.Render("esc: cancel  enter: jump")
	default:
		hints := blHints
		if n := len(m.allSelected()); n > 0 {
			hints = append([]string{fmt.Sprintf("%d selected", n)}, hints...)
		}
		switch {
		case m.state == blLoading:
			spinnerStr := m.loadSpinner.View() + tui.MutedStyle.Render(" Loading…")
			leftWidth := listPaneW - lipgloss.Width(spinnerStr) - 2
			footer = tui.FooterHints(hints, leftWidth) + "  " + spinnerStr
		case m.moving:
			spinnerStr := m.loadSpinner.View() + tui.MutedStyle.Render(" Moving…")
			leftWidth := listPaneW - lipgloss.Width(spinnerStr) - 2
			footer = tui.FooterHints(hints, leftWidth) + "  " + spinnerStr
		default:
			footer = tui.FooterHints(hints, listPaneW)
		}
	}

	return boardTopPad + topBar + "\n" + tui.SplitView(listBody, detailBody, width, paneH) + "\n" + footer
}

// blHints are the backlog's default footer hints, most-used first: they are
// dropped from the tail when the footer is too narrow. "?" opens the full help
// overlay, which is why this list is short.
var blHints = []string{
	"j/k move", "enter details", "e edit", "s status", "m move", "/ filter", "x cut", "? help",
}

// topDetail renders the tab strip's right-hand detail: the transient state
// badges, most important first.
func (m blModel) topDetail() string {
	var b strings.Builder
	if m.yankMessage != "" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.ColorSuccess).Render(tui.SanitizeRow(m.yankMessage)))
	} else if m.visualMode {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.ColorSpecial).Render("VISUAL"))
	} else if m.filterEpic != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(tui.ColorSpecial).Render("epic: " + tui.SanitizeRow(m.filterEpic)))
	} else if m.filter != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(tui.ColorWarning).Render("/ " + tui.SanitizeRow(m.filter)))
	}
	if len(m.cutKeys) > 0 {
		if b.Len() > 0 {
			b.WriteString("   ")
		}
		b.WriteString(lipgloss.NewStyle().Foreground(tui.ColorCaution).Render(fmt.Sprintf("✂ %d cut", len(m.cutKeys))))
	}
	return b.String()
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

	stateColor := tui.ColorMuted
	switch group.Sprint.State {
	case "active":
		stateColor = tui.ColorSuccess
	case "future":
		stateColor = tui.ColorAccent
	}
	accentColor := stateColor
	if row.groupIdx == activeGroupIdx {
		accentColor = tui.ColorWarning
	}

	// The sprint header carries no background fill: only the cursor row does.
	// Every segment still goes through the same style so the row is one unit.
	fill := lipgloss.NewStyle()

	accent := fill.Bold(true).Foreground(accentColor).Render("▌")

	// Build date range badge: "Mar 1 – Mar 14" or fall back to state label.
	var dateBadge string
	if group.Sprint.StartDate != "" || group.Sprint.EndDate != "" {
		dateBadge = formatSprintDate(group.Sprint.StartDate) + " – " + formatSprintDate(group.Sprint.EndDate)
	} else {
		dateBadge = group.Sprint.State
	}

	countStr := fmt.Sprintf("%d issues", len(group.Issues))
	count := fill.Foreground(tui.ColorMuted).Render(countStr)

	// A long sprint name must not wrap inside the list pane: the accent and
	// space take two cells, the state badge is dropped first, then the name is
	// shortened so the rule below always has at least one cell.
	maxLeft := width - tui.DisplayWidth(countStr) - 3
	if maxLeft < 3 {
		maxLeft = 3
	}
	nameText := icon + " " + tui.SanitizeRow(group.Sprint.Name)
	nameAvail := maxLeft - 2
	statePart := ""
	if stateWidth := tui.DisplayWidth(dateBadge); nameAvail >= stateWidth+6 {
		nameAvail -= stateWidth + 2
		statePart = fill.Foreground(stateColor).Render(dateBadge)
	}
	if nameAvail < 1 {
		nameAvail = 1
	}
	namePart := fill.Bold(true).Foreground(tui.ColorForeground).Render(tui.TruncateWidth(nameText, nameAvail))

	left := accent + fill.Render(" ") + namePart
	if statePart != "" {
		left += fill.Render("  ") + statePart
	}
	leftLen := lipgloss.Width(left)
	rightLen := tui.DisplayWidth(countStr)
	fillLen := width - leftLen - rightLen - 2
	if fillLen < 1 {
		fillLen = 1
	}
	rule := fill.Foreground(accentColor).Render(strings.Repeat("─", fillLen))
	return left + fill.Render(" ") + rule + fill.Render(" ") + count
}

func (m blModel) renderIssueRow(row blRow, isSelected bool, width int) string {
	issue := m.groups[row.groupIdx].Issues[row.issueIdx]
	l := blLayoutFor(width)

	epicText := issue.EpicName
	if epicText == "" {
		epicText = issue.EpicKey
	}
	if epicText == "" {
		epicText = "—"
	}

	// Every segment carries its own fill so a selected row never loses the
	// ColorSurface background to an intervening lipgloss reset.
	fill := lipgloss.NewStyle()
	if isSelected {
		fill = tui.SurfaceBg
	}

	// Gutter 1: selection marker. Gutter 2: status glyph. Gutter 3: space.
	isChecked := m.allSelected()[issue.Key]
	isCut := m.cutKeys[issue.Key]
	marker := " "
	markerStyle := fill
	switch {
	case isCut:
		marker, markerStyle = "✂", fill.Foreground(tui.ColorCaution)
	case isChecked:
		marker, markerStyle = "✓", fill.Foreground(tui.ColorWarning)
	}
	statusGlyph := tui.StatusGlyph(issue.Status)
	if statusGlyph == "" {
		statusGlyph = " "
	}
	statusColor := tui.StatusColor(issue.Status)
	if statusColor == nil {
		statusColor = tui.ColorMuted
	}

	key := fill.Bold(true).Foreground(tui.ColorAccent).
		Render(tui.FixedWidth(tui.SanitizeRow(issue.Key), blKeyW))
	summary := fill.Foreground(tui.ColorForegroundBright).
		Render(tui.FixedWidth(tui.SanitizeRow(issue.Summary), l.summaryW))

	epicColor := tui.EpicColor(issue.EpicKey)
	if epicColor == nil {
		epicColor = tui.ColorMuted
	}
	epic := fill.Foreground(epicColor).
		Render(tui.FixedWidth(tui.SanitizeRow(epicText), blEpicW))

	// The badge keeps its own background even on a selected row, so the
	// separators around it must be rendered with the row fill explicitly.
	typeBadge := tui.Badge(
		tui.FixedWidth(tui.SanitizeRow(issue.IssueType), blTypeW-2),
		tui.ColorOnChrome,
		tui.IssueTypeColor(issue.IssueType),
	)

	priorityGlyph := tui.PriorityGlyph(issue.Priority)
	priorityStyle := fill.Foreground(tui.ColorMuted)
	if pc := tui.PriorityColor(issue.Priority); pc != nil {
		priorityStyle = fill.Foreground(pc)
	} else {
		priorityGlyph = " "
	}

	sp := fill.Foreground(tui.ColorMuted).
		Render(tui.FixedWidth(tui.FormatStoryPoints(issue.StoryPoints), blSpW))

	var b strings.Builder
	b.WriteString(markerStyle.Render(marker))
	b.WriteString(fill.Foreground(statusColor).Render(statusGlyph))
	b.WriteString(fill.Render(" "))
	b.WriteString(key)
	b.WriteString(fill.Render("  "))
	b.WriteString(summary)

	if l.compact {
		// Compact cluster: type glyph, initials, priority glyph.
		b.WriteString(fill.Render(" "))
		b.WriteString(fill.Foreground(tui.IssueTypeColor(issue.IssueType)).Render(tui.TypeGlyph(issue.IssueType)))
		b.WriteString(fill.Render(" "))
		b.WriteString(m.renderAssigneeCell(fill, issue.Assignee, 3))
		b.WriteString(fill.Render(" "))
		b.WriteString(priorityStyle.Render(priorityGlyph))
		return b.String()
	}

	if l.epic {
		b.WriteString(fill.Render("  "))
		b.WriteString(epic)
	}
	if l.typ {
		b.WriteString(fill.Render(" "))
		b.WriteString(typeBadge)
	}
	if l.prio {
		b.WriteString(fill.Render(" "))
		b.WriteString(priorityStyle.Render(priorityGlyph))
	}
	if l.sp {
		b.WriteString(fill.Render(" "))
		b.WriteString(sp)
	}
	if l.assignW > 0 {
		b.WriteString(fill.Render(" "))
		b.WriteString(m.renderAssigneeCell(fill, issue.Assignee, l.assignW))
	}
	return b.String()
}

// renderAssigneeCell renders the assignee column at exactly width cells: a
// coloured initials cluster plus the name when there is room for one, or the
// initials alone. An unassigned issue shows an em dash.
func (m blModel) renderAssigneeCell(fill lipgloss.Style, assignee string, width int) string {
	if assignee == "" {
		return fill.Foreground(tui.ColorMuted).Render(tui.FixedWidth("—", width))
	}
	if width < 14 {
		return fill.Bold(true).Foreground(tui.PersonColor(assignee)).
			Render(tui.FixedWidth(tui.Initials(assignee), width))
	}
	initials := fill.Bold(true).Foreground(tui.PersonColor(assignee)).
		Render(tui.FixedWidth(tui.Initials(assignee), 2))
	name := fill.Foreground(tui.ColorForeground).
		Render(" " + tui.FixedWidth(tui.SanitizeRow(assignee), width-3))
	return initials + name
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
	n := len(m.parentTargetKeys)
	noun := "issue"
	if n != 1 {
		noun = "issues"
	}
	title := fmt.Sprintf("Set Parent  (%d %s)", n, noun)

	return tui.RenderPickerModal(
		title,
		func(innerW, listH int) string { return m.parentPicker.View(innerW, listH) },
		tui.PickerFooter,
		m.width,
		m.height,
	)
}

func (m blModel) viewSprintPicker() string {
	n := len(m.sprintTargetKeys)
	noun := "issue"
	if n != 1 {
		noun = "issues"
	}
	title := fmt.Sprintf("Move to Sprint  (%d %s)", n, noun)

	return tui.RenderPickerModal(
		title,
		func(innerW, listH int) string { return m.sprintPicker.View(innerW, listH) },
		tui.PickerFooter,
		m.width,
		m.height,
	)
}

func (m blModel) viewStoryPointInput() string {
	n := len(m.storyPointTargetKeys)
	noun := "issue"
	if n != 1 {
		noun = "issues"
	}
	title := fmt.Sprintf("Set Story Points  (%d %s)", n, noun)

	return tui.RenderPickerModal(
		title,
		func(innerW, _ int) string {
			return "  " + tui.FitInput(m.storyPointInput, innerW-2)
		},
		"  enter: set   esc: cancel",
		m.width,
		m.height,
	)
}

func (m blModel) viewEpicFilterPicker() string {
	title := "Filter by Epic"
	if m.filterEpic != "" {
		title = "Filter by Epic  (current: " + m.filterEpic + ")"
	}

	return tui.RenderPickerModal(
		title,
		func(innerW, listH int) string { return m.epicFilterPicker.View(innerW, listH) },
		tui.PickerFooter,
		m.width,
		m.height,
	)
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
	header := tui.BoldAccent.Padding(0, 1).Width(innerW).
		Render(tui.FixedWidth(title, innerW-2))

	const labelW = 16
	labelStyle := tui.MutedStyle
	activeStyle := lipgloss.NewStyle().Foreground(tui.ColorForeground)

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
	endLine := "  " + tui.MutedStyle.Render(fmt.Sprintf("%-*s", labelW, "End Date")) +
		tui.MutedStyle.Render(endDate)

	var errorLine string
	if m.sprintFormError != "" {
		errorLine = "\n  " + lipgloss.NewStyle().Foreground(tui.ColorError).Render("✗ "+m.sprintFormError)
	}

	var footer string
	if m.sprintFormSubmitting {
		footer = "  " + m.loadSpinner.View() + tui.MutedStyle.Render(" Saving…")
	} else {
		action := "create"
		if isEdit {
			action = "save"
		}
		footer = tui.MutedStyle.Render(fmt.Sprintf("  tab/shift+tab: next field   ctrl+s: %s   esc: cancel", action))
	}

	body := header + "\n" +
		"\n" +
		nameLine + "\n" +
		startLine + "\n" +
		durLine + "\n" +
		endLine +
		errorLine + "\n" +
		"\n" +
		tui.MutedStyle.Render(strings.Repeat("─", innerW)) + "\n" +
		footer

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorAccent).
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
