package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/tira/internal/tui"
)

func (m kanbanModel) View() string {
	switch m.state {
	case stateDetail:
		return m.viewDetail()
	case stateAssignPicker:
		return m.viewAssignPicker()
	case stateStatusPicker:
		return m.viewStatusPicker()
	default:
		return m.viewBoard()
	}
}

func (m kanbanModel) viewAssignPicker() string {
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

	header := tui.BoldBlue.Padding(0, 1).Width(innerW).
		Render(tui.FixedWidth("Set Assignee", innerW-2))

	listH := height/2 - 6
	if listH < 4 {
		listH = 4
	}

	footer := tui.DimStyle.Render("  ↑/↓ ctrl+p/n: navigate   enter: select   esc: cancel")

	body := header + "\n" +
		m.assignPicker.View(innerW, listH) + "\n" +
		tui.DimStyle.Render(strings.Repeat("─", innerW)) + "\n" +
		footer

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBlue).
		Width(innerW).
		Render(body)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func (m kanbanModel) viewStatusPicker() string {
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

	header := tui.BoldBlue.Padding(0, 1).Width(innerW).
		Render(tui.FixedWidth("Transition Status", innerW-2))

	listH := height/2 - 6
	if listH < 4 {
		listH = 4
	}

	footer := tui.DimStyle.Render("  ↑/↓ ctrl+p/n: navigate   enter: select   esc: cancel")

	body := header + "\n" +
		m.statusPicker.View(innerW, listH) + "\n" +
		tui.DimStyle.Render(strings.Repeat("─", innerW)) + "\n" +
		footer

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBlue).
		Width(innerW).
		Render(body)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
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

	header := tui.BoldBlue.Padding(0, 1).Width(innerW).
		Render(tui.FixedWidth(m.detailIssue.Key+"  "+m.detailIssue.Summary, innerW-2))
	footer := tui.DimStyle.Render("  e: edit   c: comment   o: open in browser   esc/q: back   j/k: scroll")
	body := header + "\n" + m.detailView.View() + "\n" + footer

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBlue).
		Width(innerW).
		Render(body)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
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
	innerWidth := colWidth - 4

	activeColStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBlue).
		Padding(0, 1).
		Width(innerWidth)
	inactiveColStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorDimmer).
		Padding(0, 1).
		Width(innerWidth)

	colHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorFgBright)
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorBlue)
	selKeyStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorWhite).Background(tui.ColorBg)
	selSumStyle := lipgloss.NewStyle().Foreground(tui.ColorFg).Background(tui.ColorBg)
	assigneeStyle := lipgloss.NewStyle().Foreground(tui.ColorDim)
	daysStyle := lipgloss.NewStyle().Bold(true)

	var renderedCols []string
	for ci, col := range m.columns {
		var lines []string

		title := strings.ToUpper(col.name) + fmt.Sprintf(" (%d)", len(col.issues))
		lines = append(lines, colHeaderStyle.Render(title))
		lines = append(lines, tui.DimStyle.Render(strings.Repeat("─", innerWidth)))

		if len(col.issues) == 0 {
			lines = append(lines, tui.DimStyle.Render("  (empty)"))
		}

		for ri, issue := range col.issues {
			isSelected := ci == m.colIdx && ri == m.rowIdxs[ci]
			maxSummary := innerWidth - 4
			if maxSummary < 1 {
				maxSummary = 1
			}
			runes := []rune(issue.Summary)
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
				lines = append(lines,
					selKeyStyle.Render("▶ "+issue.Key),
					selSumStyle.Render("  "+summary),
				)
				if assigneeStr != "" || days > 0 {
					var metaParts []string
					if assigneeStr != "" {
						metaParts = append(metaParts, assigneeStyle.Render(assigneeStr))
					}
					if days > 0 {
						metaParts = append(metaParts, daysStyle.Foreground(daysColor).Render(daysStr))
					}
					lines = append(lines, selSumStyle.Render("  "+strings.Join(metaParts, " • ")))
				}
			} else {
				lines = append(lines,
					"  "+keyStyle.Render(issue.Key),
					"  "+tui.DimStyle.Render(summary),
				)
				if assigneeStr != "" || days > 0 {
					var metaParts []string
					if assigneeStr != "" {
						metaParts = append(metaParts, assigneeStyle.Render(assigneeStr))
					}
					if days > 0 {
						metaParts = append(metaParts, daysStyle.Foreground(daysColor).Render(daysStr))
					}
					lines = append(lines, "  "+tui.DimStyle.Render(strings.Join(metaParts, " • ")))
				}
			}
		}

		colStyle := inactiveColStyle
		if ci == m.colIdx {
			colStyle = activeColStyle
		}
		renderedCols = append(renderedCols, colStyle.Render(strings.Join(lines, "\n")))
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, renderedCols...)

	var header string
	if m.sprintName != "" {
		header = tui.BoldBlue.Padding(0, 1).
			Render("Kanban: "+m.sprintName) + "\n"
	}

	hintsStr := "  hjkl: navigate   enter: view   e: edit   c: comment   s: status   o: open   tab: backlog   q: quit"
	var footerStr string
	if m.state == stateLoading {
		spinnerStr := m.loadSpinner.View() + tui.DimStyle.Render(" Loading…")
		padded := tui.FixedWidth(hintsStr, width-lipgloss.Width(spinnerStr)-2)
		footerStr = tui.DimStyle.Render(padded) + "  " + spinnerStr
	} else {
		footerStr = tui.DimStyle.Render(hintsStr)
	}

	// Build board content (header + board)
	var boardContent string
	if header != "" {
		boardContent = header + board
	} else {
		boardContent = board
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
