package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	return tui.MutedStyle.Render(
		"  " +
			tui.FixedWidth("KEY", epicKeyWidth) + "  " +
			tui.FixedWidth("SUMMARY", epicSummaryWidth(width)) + "  " +
			tui.FixedWidth("FIRST APPEARS", epicLocationWidth) + "  " +
			tui.FixedWidth("SP", epicStoryPointsW) + "  " +
			tui.FixedWidth("CHILDREN", epicCountWidth),
	)
}

func (m epicModel) View() tea.View {
	if m.state == epicDetail {
		return tea.NewView(m.viewDetail())
	}
	if m.state == epicLabelLoading || m.state == epicLabelInput || m.state == epicLabelSaving {
		return tea.NewView(m.viewLabelEditor())
	}
	return tea.NewView(m.viewList())
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
	footer := tui.MutedStyle.Render("  o: open in browser   esc/q: back   j/k: scroll")
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
	width, height := m.width, m.height
	if width == 0 {
		width = 120
	}
	if height == 0 {
		height = 40
	}

	listWidth := tui.ListPaneWidth(width)
	vh := m.viewHeight()
	if vh < 1 {
		vh = 1
	}

	header := tui.BoldAccent.Padding(0, 1).Render("Epics")
	if m.loading {
		header += " " + tui.MutedStyle.Render("(loading more…)")
	}
	if m.loadError != "" {
		header += " " + lipgloss.NewStyle().Foreground(tui.ColorError).Render("⚠ "+m.loadError)
	}

	colHeader := epicColumnHeader(listWidth)
	div := lipgloss.NewStyle().Foreground(tui.ColorSubtle).Render("│")
	headerLine := lipgloss.NewStyle().Width(listWidth).Render(colHeader) + div

	var rows []string
	if len(m.items) == 0 {
		rows = append(rows, tui.MutedStyle.Render("  No represented epics"))
	} else {
		end := min(m.offset+vh, len(m.items))
		for i := m.offset; i < end; i++ {
			rows = append(rows, m.renderRow(i, listWidth))
		}
	}
	for len(rows) < vh {
		rows = append(rows, "")
	}

	sidebarLines := strings.Split(m.sidebarContent, "\n")
	sidebarStart := tui.Clamp(m.sidebarOffset, 0, max(len(sidebarLines)-1, 0))
	sidebarEnd := min(sidebarStart+vh, len(sidebarLines))
	sidebar := append([]string(nil), sidebarLines[sidebarStart:sidebarEnd]...)
	for len(sidebar) < vh {
		sidebar = append(sidebar, "")
	}

	footer := "  j/k ↑/↓: move   enter: details   l: edit labels   b: filter backlog   o: open Jira   R: refresh   ctrl+d/u: scroll   q: quit"
	if m.state == epicLoading {
		footer = "  " + m.loadSpinner.View() + tui.MutedStyle.Render(" Loading epic…") + "   " + footer
	}

	return header + "\n" +
		headerLine + "\n" +
		tui.SplitPanes(strings.Join(rows, "\n"), strings.Join(sidebar, "\n"), listWidth, vh) +
		"\n" + tui.MutedStyle.Render(footer)
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

	key := tui.FixedWidth(item.Key, epicKeyWidth)
	summary := tui.FixedWidth(name, summaryW)
	firstLocation := tui.FixedWidth(location, epicLocationWidth)
	storyPoints := tui.FixedWidth(tui.FormatStoryPoints(item.StoryPoints), epicStoryPointsW)
	children := tui.FixedWidth(fmt.Sprintf("%d", item.ChildCount), epicCountWidth)

	epicColor := tui.EpicColor(item.Key)
	if epicColor == nil {
		epicColor = tui.ColorAccent
	}
	locationColor := tui.SprintColor(item.FirstSprintIndex)

	if idx == m.cursor {
		bg := tui.SurfaceBg
		return bg.Render("  ") +
			bg.Bold(true).Foreground(epicColor).Render(key) +
			bg.Foreground(tui.ColorHighlight).Render("  "+summary+"  ") +
			bg.Foreground(locationColor).Render(firstLocation+"  ") +
			bg.Foreground(tui.ColorForeground).Render(storyPoints+"  ") +
			bg.Foreground(tui.ColorForeground).Render(children)
	}

	return "  " +
		lipgloss.NewStyle().Bold(true).Foreground(epicColor).Render(key) +
		lipgloss.NewStyle().Foreground(tui.ColorForegroundBright).Render("  "+summary+"  ") +
		lipgloss.NewStyle().Foreground(locationColor).Render(firstLocation+"  ") +
		tui.MutedStyle.Render(storyPoints+"  ") +
		tui.MutedStyle.Render(children)
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

func renderEpicSidebarContent(issue *models.Issue, item *epicItem, width int) string {
	if issue == nil {
		return tui.MutedStyle.Render("No epic selected")
	}
	content := renderSidebarContent(issue, width)
	if item == nil {
		return content
	}
	meta := fmt.Sprintf("\n\nChildren: %d\nFirst appears: %s", item.ChildCount, item.FirstLocation)
	return content + tui.MutedStyle.Render(meta)
}
