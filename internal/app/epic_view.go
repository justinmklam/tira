package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/justinmklam/tira/internal/display"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
)

const (
	epicKeyWidth      = 13
	epicLocationWidth = 16
	epicStoryPointsW  = 5
	epicCountWidth    = 9
	epicRowOverhead   = 10 // leading indent plus four column gaps
)

func epicSummaryWidth(totalWidth int) int {
	w := totalWidth - epicRowOverhead - epicKeyWidth - epicLocationWidth - epicStoryPointsW - epicCountWidth
	if w < 8 {
		w = 8
	}
	return w
}

func epicColumnHeader(width int) string {
	return tui.SectionHeader(
		"  "+
			tui.FixedWidth("KEY", epicKeyWidth)+"  "+
			tui.FixedWidth("SUMMARY", epicSummaryWidth(width))+"  "+
			tui.FixedWidth("FIRST APPEARS", epicLocationWidth)+"  "+
			tui.FixedWidth("SP", epicStoryPointsW)+"  "+
			tui.FixedWidth("CHILDREN", epicCountWidth),
		tui.ColorMuted,
		width,
	)
}

// epicHints are the epics footer hints, most-used first: they are dropped from
// the tail when the footer is too narrow.
var epicHints = []string{
	"j/k move", "enter details", "L linked", "/ filter", "l labels", "b backlog", "o open Jira", "R refresh", "ctrl+d/u scroll", "q quit",
}

// epicTopDetail renders the tab strip's right-hand detail: the load state and
// filter, most important first.
func (m epicModel) epicTopDetail() string {
	var b strings.Builder
	if m.loadError != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(tui.ColorError).Render("⚠ " + tui.SanitizeRow(m.loadError)))
	}
	if m.filter != "" {
		if b.Len() > 0 {
			b.WriteString("   ")
		}
		b.WriteString(lipgloss.NewStyle().Foreground(tui.ColorWarning).Render("/ " + tui.SanitizeRow(m.filter)))
	}
	if m.loading {
		if b.Len() > 0 {
			b.WriteString("   ")
		}
		b.WriteString(tui.MutedStyle.Render("(loading more…)"))
	}
	return b.String()
}

func (m epicModel) View() tea.View {
	if m.state == epicDetail {
		return tea.NewView(m.viewDetail())
	}
	if m.state == epicLabelLoading || m.state == epicLabelInput || m.state == epicLabelSaving {
		return tea.NewView(m.viewLabelEditor())
	}
	if m.state == epicLinkPicker {
		return tea.NewView(m.viewLinkPicker())
	}
	return tea.NewView(m.viewList())
}

// viewLinkPicker renders the related work items picker overlay. Selecting an
// item opens it in the browser.
func (m epicModel) viewLinkPicker() string {
	return viewLinkedItemsPicker(m.linkPicker.picker, m.width, m.height)
}

func (m epicModel) viewDetail() string {
	if m.detailIssue == nil {
		return ""
	}
	width, height := m.width, m.height
	if width == 0 {
		width = 120
	}
	if height == 0 {
		height = 40
	}
	overlayW, _ := tui.OverlaySize(width, height)
	innerW := overlayW - 2
	footer := tui.MutedStyle.Render("  o: open in browser   L: linked items   esc/q: back   j/k: scroll")
	body := m.detailView.View() + "\n" + footer
	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorAccent).
		Width(innerW).
		Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func (m epicModel) viewList() string {
	if m.quitting {
		return ""
	}
	width := m.width
	if width == 0 {
		width = 120
	}

	listWidth := tui.ListPaneWidth(width)
	// Pane geometry mirrors the backlog: the list body is the column header plus
	// viewHeight() rows, the detail body is the sidebar sliced to viewHeight()+1
	// rows, and paneH (height-3) is each frame's outer height.
	vh := m.viewHeight()
	if vh < 1 {
		vh = 1
	}
	paneH := vh + 3

	header := tui.TabStripStyled(2, m.epicTopDetail(), width)

	colHeader := epicColumnHeader(listWidth)

	var rows []string
	if len(m.items) == 0 {
		rows = append(rows, tui.EmptyState("No represented epics", listWidth))
	} else {
		end := min(m.offset+vh, len(m.items))
		for i := m.offset; i < end; i++ {
			rows = append(rows, m.renderRow(i, listWidth))
		}
	}
	for len(rows) < vh {
		rows = append(rows, "")
	}
	listBody := colHeader + "\n" + strings.Join(rows, "\n")

	sidebarLines := strings.Split(m.sidebarContent, "\n")
	sidebarStart := tui.Clamp(m.sidebarOffset, 0, max(len(sidebarLines)-1, 0))
	// The titled detail frame spends one body row on the title padding, so only
	// viewHeight() sidebar lines fit; keeping the slice to vh lines keeps the last
	// line reachable at max scroll.
	sidebarEnd := min(sidebarStart+vh, len(sidebarLines))
	sidebar := append([]string(nil), sidebarLines[sidebarStart:sidebarEnd]...)
	for len(sidebar) < vh {
		sidebar = append(sidebar, "")
	}
	detailBody := strings.Join(sidebar, "\n")

	var footer string
	switch m.state {
	case epicFilter:
		footer = lipgloss.NewStyle().Foreground(tui.ColorAccent).Render("/") +
			" " + m.filterInput.View() +
			"  " + tui.MutedStyle.Render("esc: clear  enter: apply")
	case epicLoading:
		loadingStr := m.loadSpinner.View() + tui.MutedStyle.Render(" Loading epic…")
		footer = loadingStr + "  " + tui.FooterHints(epicHints, width-lipgloss.Width(loadingStr)-2)
	default:
		footer = tui.FooterHints(epicHints, width)
	}

	return boardTopPad + header + "\n" +
		tui.SplitView(listBody, detailBody, width, paneH) +
		"\n" + footer
}

func (m epicModel) renderRow(idx, width int) string {
	item := m.items[idx]
	summaryW := epicSummaryWidth(width)
	name := item.Name
	if name == "" {
		name = item.Summary
	}
	if name == "" {
		name = item.Key
	}
	location := item.FirstLocation
	if location == "" {
		location = "Backlog"
	}

	key := tui.FixedWidth(tui.SanitizeRow(item.Key), epicKeyWidth)
	summary := tui.FixedWidth(tui.SanitizeRow(name), summaryW)
	firstLocation := tui.FixedWidth(tui.SanitizeRow(location), epicLocationWidth)
	storyPoints := tui.FixedWidth(tui.FormatStoryPoints(item.StoryPoints), epicStoryPointsW)
	children := tui.FixedWidth(fmt.Sprintf("%d", item.ChildCount), epicCountWidth)

	epicColor := tui.EpicColor(item.Key)
	if epicColor == nil {
		epicColor = tui.ColorAccent
	}
	locationColor := tui.SprintColor(item.FirstSprintIndex)

	// The leading two-cell indent becomes a status lane: the glyph is
	// width-neutral (F9), so epicRowOverhead is unchanged.
	statusGlyph := tui.StatusGlyph(item.EpicStatus)
	if statusGlyph == "" {
		statusGlyph = " "
	}
	statusColor := tui.StatusColor(item.EpicStatus)
	if statusColor == nil {
		statusColor = tui.ColorMuted
	}

	if idx == m.cursor {
		bg := tui.SurfaceBg
		return tui.TruncateWidth(bg.Foreground(statusColor).Render(statusGlyph)+
			bg.Render(" ")+
			bg.Bold(true).Foreground(epicColor).Render(key)+
			bg.Foreground(tui.ColorHighlight).Render("  "+summary+"  ")+
			bg.Foreground(locationColor).Render(firstLocation+"  ")+
			bg.Foreground(tui.ColorForeground).Render(storyPoints+"  ")+
			bg.Foreground(tui.ColorForeground).Render(children), width)
	}

	// The epic columns do not shrink below their floors, so at a very narrow
	// pane the trailing columns are clipped rather than wrapped; the header is
	// clamped the same way by SectionHeader.
	return tui.TruncateWidth(lipgloss.NewStyle().Foreground(statusColor).Render(statusGlyph)+" "+
		lipgloss.NewStyle().Bold(true).Foreground(epicColor).Render(key)+
		lipgloss.NewStyle().Foreground(tui.ColorForegroundBright).Render("  "+summary+"  ")+
		lipgloss.NewStyle().Foreground(locationColor).Render(firstLocation+"  ")+
		tui.MutedStyle.Render(storyPoints+"  ")+
		tui.MutedStyle.Render(children), width)
}

func (m epicModel) viewLabelEditor() string {
	width, height := m.width, m.height
	if width == 0 {
		width = 120
	}
	if height == 0 {
		height = 40
	}
	overlayW, _ := tui.OverlaySize(width, height)
	innerW := overlayW - 2
	key := m.labelTargetKey
	if key == "" {
		key = "selected epic"
	}

	title := tui.BoldAccent.Padding(0, 1).Width(innerW).
		Render(tui.FixedWidth("Edit Labels - "+key, innerW-2))
	var lines []string
	lines = append(lines, title)

	switch m.state {
	case epicLabelLoading:
		lines = append(lines, "  "+m.loadSpinner.View()+" "+tui.MutedStyle.Render("Loading labels..."))
	case epicLabelSaving:
		lines = append(lines, "  "+m.loadSpinner.View()+" "+tui.MutedStyle.Render("Saving labels..."))
	default:
		lines = append(lines, "  "+m.labelInput.View())
		lines = append(lines, tui.MutedStyle.Render(strings.Repeat("─", innerW)))
		if m.labelError != "" {
			lines = append(lines, lipgloss.NewStyle().Foreground(tui.ColorError).Render("  Error: "+m.labelError))
		}
		lines = append(lines, tui.MutedStyle.Render("  enter: save   esc: cancel   comma-separated; empty clears all"))
	}

	body := strings.Join(lines, "\n")
	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorAccent).
		Width(innerW).
		Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func renderEpicSidebarContent(issue *models.Issue, item *epicItem, children epicChildren, width int) string {
	if issue == nil {
		return tui.MutedStyle.Render("No epic selected")
	}
	content := renderIssueContent(issue, width-4)

	// Muted summary lines. Their text is never wrapped around the glamour
	// output below, which would flatten the section's own styling.
	var meta strings.Builder
	if item != nil {
		fmt.Fprintf(&meta, "\n\nChildren: %d\nFirst appears: %s", item.ChildCount, item.FirstLocation)
	}
	if children.err != "" {
		fmt.Fprintf(&meta, "\n%s", childrenUnavailableText(children.err))
	}
	if children.loading && len(children.items) == 0 {
		meta.WriteString("\nLoading child work items…")
	}
	if section := renderChildrenSection(children.items, width-4); section != "" {
		meta.WriteString("\n" + section)
	}
	if meta.Len() == 0 {
		return content
	}
	return content + tui.MutedStyle.Render(meta.String())
}

// childrenUnavailableText describes a failed child fetch for inline display.
func childrenUnavailableText(err string) string {
	return "Child work items unavailable: " + err
}

// renderChildrenSection glamour-renders the child work items section at the
// given wrap width.
func renderChildrenSection(children []models.LinkedIssue, wrapWidth int) string {
	section := childrenMarkdown(children)
	if section == "" {
		return ""
	}
	return strings.TrimRight(renderMarkdownWithGlamour(section, wrapWidth), "\n")
}

// renderEpicIssueContent renders the epic detail overlay body: the epic itself
// plus its child work items.
func renderEpicIssueContent(issue *models.Issue, children []models.LinkedIssue, wrapWidth int) string {
	markdown := display.RenderIssue(issue) + childrenMarkdown(children)
	return renderMarkdownWithGlamour(markdown, wrapWidth)
}

// childrenMarkdown renders child work items as a Markdown section, or "" when
// the epic has none.
func childrenMarkdown(children []models.LinkedIssue) string {
	return display.LinkedItemsSection("Child Work Items", children)
}
