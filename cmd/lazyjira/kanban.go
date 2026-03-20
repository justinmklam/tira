package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/lazyjira/internal/api"
	"github.com/justinmklam/lazyjira/internal/display"
	"github.com/justinmklam/lazyjira/internal/models"
	"github.com/justinmklam/lazyjira/internal/tui"
)

// --- kanban TUI model ---

type kanbanState int

const (
	stateBoard   kanbanState = iota
	stateLoading             // fetching full issue for detail view
	stateDetail
	stateAssignPicker
)

type kanbanColumn struct {
	name   string
	issues []models.Issue
}

type issueFetchedMsg struct {
	issue   *models.Issue
	content string // pre-rendered glamour content
	err     error
}

type kanbanResult struct {
	editKey string // non-empty when the user pressed e
	refresh bool
}

type kanbanAssignDoneMsg struct{ err error }

type kanbanModel struct {
	state    kanbanState
	client   api.Client
	project  string
	width    int
	height   int
	quitting bool
	result   kanbanResult

	// Board state
	columns    []kanbanColumn
	colIdx     int
	rowIdxs    []int
	sprintName string

	// Loading state
	loadSpinner spinner.Model

	// Detail state
	detailIssue *models.Issue
	detailView  viewport.Model

	// Assignee picker state
	assignPicker     tui.PickerModel
	assignTargetKeys []string
}

// buildColumns maps sprint issues into the board's fixed column order.
// Issues whose status ID doesn't match any column fall into the last column.
func buildColumns(boardCols []models.BoardColumn, issues []models.Issue) []kanbanColumn {
	statusIDToCol := map[string]int{}
	for i, col := range boardCols {
		for _, sid := range col.StatusIDs {
			statusIDToCol[sid] = i
		}
	}

	cols := make([]kanbanColumn, len(boardCols))
	for i, bc := range boardCols {
		cols[i] = kanbanColumn{name: bc.Name}
	}

	for _, issue := range issues {
		colIdx, ok := statusIDToCol[issue.StatusID]
		if !ok {
			colIdx = len(cols) - 1
		}
		cols[colIdx].issues = append(cols[colIdx].issues, issue)
	}
	return cols
}

func newKanbanModel(client api.Client, boardCols []models.BoardColumn, issues []models.Issue, sprintName, project string) kanbanModel {
	cols := buildColumns(boardCols, issues)
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(tui.SpinnerColor)
	return kanbanModel{
		state:       stateBoard,
		client:      client,
		project:     project,
		columns:     cols,
		rowIdxs:     make([]int, len(cols)),
		sprintName:  sprintName,
		loadSpinner: s,
	}
}

// refreshData replaces the kanban columns with new data, preserving cursor
// positions (clamped to the new column sizes).
func (m *kanbanModel) refreshData(boardCols []models.BoardColumn, issues []models.Issue, sprintName string) {
	prev := m.rowIdxs
	m.columns = buildColumns(boardCols, issues)
	newRowIdxs := make([]int, len(m.columns))
	for i := range newRowIdxs {
		if i < len(prev) && len(m.columns[i].issues) > 0 {
			newRowIdxs[i] = tui.Clamp(prev[i], 0, len(m.columns[i].issues)-1)
		}
	}
	m.rowIdxs = newRowIdxs
	m.colIdx = tui.Clamp(m.colIdx, 0, max(len(m.columns)-1, 0))
	m.sprintName = sprintName
}

func (m kanbanModel) currentIssue() *models.Issue {
	if len(m.columns) == 0 || len(m.columns[m.colIdx].issues) == 0 {
		return nil
	}
	return &m.columns[m.colIdx].issues[m.rowIdxs[m.colIdx]]
}

func (m kanbanModel) Init() tea.Cmd { return nil }

func (m kanbanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.state == stateDetail {
			vpW, vpH := tui.OverlayViewportSize(m.width, m.height)
			m.detailView.Width = vpW
			m.detailView.Height = vpH
		}
		return m, nil

	case issueFetchedMsg:
		if msg.err != nil {
			m.state = stateBoard
			return m, nil
		}
		m.detailIssue = msg.issue
		vpW, vpH := tui.OverlayViewportSize(m.width, m.height)
		vp := viewport.New(vpW, vpH)
		vp.SetContent(msg.content)
		m.detailView = vp
		m.state = stateDetail
		return m, nil

	case spinner.TickMsg:
		if m.state == stateLoading {
			var cmd tea.Cmd
			m.loadSpinner, cmd = m.loadSpinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case kanbanAssignDoneMsg:
		if msg.err == nil {
			m.result.refresh = true
		}
		return m, nil
	}

	switch m.state {
	case stateBoard:
		return m.updateBoard(msg)
	case stateDetail:
		return m.updateDetail(msg)
	case stateAssignPicker:
		return m.updateAssignPicker(msg)
	}
	return m, nil
}

func (m kanbanModel) updateBoard(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, nil
	case "j":
		col := m.columns[m.colIdx]
		if m.rowIdxs[m.colIdx] < len(col.issues)-1 {
			m.rowIdxs[m.colIdx]++
		}
	case "k":
		if m.rowIdxs[m.colIdx] > 0 {
			m.rowIdxs[m.colIdx]--
		}
	case "h":
		if m.colIdx > 0 {
			m.colIdx--
		}
	case "l":
		if m.colIdx < len(m.columns)-1 {
			m.colIdx++
		}
	case "enter":
		if issue := m.currentIssue(); issue != nil {
			m.state = stateLoading
			vpW, _ := tui.OverlayViewportSize(m.width, m.height)
			return m, tea.Batch(m.loadSpinner.Tick, fetchIssueCmd(m.client, issue.Key, vpW))
		}
	case "e":
		if issue := m.currentIssue(); issue != nil {
			m.result = kanbanResult{editKey: issue.Key}
			return m, nil
		}
	case "A":
		if issue := m.currentIssue(); issue != nil {
			projectKey := m.project
			if projectKey == "" {
				if idx := strings.Index(issue.Key, "-"); idx > 0 {
					projectKey = issue.Key[:idx]
				}
			}
			m.assignTargetKeys = []string{issue.Key}
			m.assignPicker = newAssigneePicker(m.client, projectKey)
			m.state = stateAssignPicker
			return m, m.assignPicker.Init()
		}
	}
	return m, nil
}

func (m kanbanModel) updateDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			m.state = stateBoard
			m.detailIssue = nil
			return m, nil
		case "e":
			if m.detailIssue != nil {
				m.result = kanbanResult{editKey: m.detailIssue.Key}
				return m, nil
			}
		case "ctrl+c":
			m.quitting = true
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.detailView, cmd = m.detailView.Update(msg)
	return m, cmd
}

func (m kanbanModel) updateAssignPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		m.quitting = true
		return m, nil
	}
	updated, cmd := m.assignPicker.Update(msg)
	m.assignPicker = updated
	if m.assignPicker.Aborted {
		m.state = stateBoard
		m.assignTargetKeys = nil
		return m, nil
	}
	if m.assignPicker.Completed {
		item := m.assignPicker.SelectedItem()
		var accountID string
		if item != nil {
			accountID = item.Value
		}
		keys := m.assignTargetKeys
		m.state = stateBoard
		m.assignTargetKeys = nil
		return m, kanbanAssignCmd(m.client, keys, accountID)
	}
	return m, cmd
}

func kanbanAssignCmd(client api.Client, keys []string, accountID string) tea.Cmd {
	return func() tea.Msg {
		for _, k := range keys {
			if err := client.SetAssignee(k, accountID); err != nil {
				return kanbanAssignDoneMsg{err: err}
			}
		}
		return kanbanAssignDoneMsg{}
	}
}

func fetchIssueCmd(client api.Client, key string, vpW int) tea.Cmd {
	return func() tea.Msg {
		issue, err := client.GetIssue(key)
		if err != nil {
			return issueFetchedMsg{err: err}
		}
		md, _ := display.RenderIssue(issue)
		// Use a fixed dark style to avoid terminal detection in the goroutine.
		// The pre-detection in runBoardTUI caches termenv's sync.Once, but
		// using a fixed style here is more reliable and avoids any blocking.
		renderer, rerr := glamour.NewTermRenderer(
			glamour.WithStyles(styles.DarkStyleConfig),
			glamour.WithWordWrap(vpW),
		)
		content := md
		if rerr == nil {
			if rendered, rerr2 := renderer.Render(md); rerr2 == nil {
				content = rendered
			}
		}
		return issueFetchedMsg{issue: issue, content: content}
	}
}

func (m kanbanModel) View() string {
	switch m.state {
	case stateDetail:
		return m.viewDetail()
	case stateAssignPicker:
		return m.viewAssignPicker()
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
	footer := tui.DimStyle.Render("  e: edit   o: open in browser   esc/q: back   j/k: scroll")
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
	hintsStr := "  hjkl: navigate   enter: view   e: edit   o: open   tab: backlog   q: quit"
	var footer string
	if m.state == stateLoading {
		spinnerStr := m.loadSpinner.View() + tui.DimStyle.Render(" Loading…")
		padded := tui.FixedWidth(hintsStr, width-lipgloss.Width(spinnerStr)-2)
		footer = "\n" + tui.DimStyle.Render(padded) + "  " + spinnerStr
	} else {
		footer = "\n" + tui.DimStyle.Render(hintsStr)
	}

	return header + board + footer
}
