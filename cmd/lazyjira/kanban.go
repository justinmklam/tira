package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/lazyjira/internal/api"
	"github.com/justinmklam/lazyjira/internal/display"
	"github.com/justinmklam/lazyjira/internal/models"
	"github.com/spf13/cobra"
)

var kanbanCmd = &cobra.Command{
	Use:   "kanban",
	Short: "Show the active sprint as an interactive kanban board",
	RunE:  runKanbanCmd,
}

func init() {
	rootCmd.AddCommand(kanbanCmd)
}

func runKanbanCmd(_ *cobra.Command, _ []string) error {
	if cfg.BoardID == 0 {
		return fmt.Errorf("board ID not configured: set default_board_id in ~/.config/lazyjira/config.yaml")
	}

	client, err := api.NewClient(cfg)
	if err != nil {
		return err
	}

	boardCols, issues, err := fetchBoardDataWithSpinner(client, cfg.BoardID)
	if err != nil {
		return err
	}

	sprintName := ""
	if len(issues) > 0 {
		sprintName = issues[0].SprintName
	}

	for {
		result, err := runKanbanTUI(client, boardCols, issues, sprintName)
		if err != nil {
			return err
		}
		if result.editKey == "" {
			break
		}

		issue, err := fetchWithSpinner(fmt.Sprintf("Fetching %s…", result.editKey), func() (*models.Issue, error) {
			return client.GetIssue(result.editKey)
		})
		if err != nil {
			return err
		}

		if err := runEditLoop(client, issue); err != nil {
			return err
		}

		// Re-fetch sprint issues to reflect any updates.
		if refreshed, err := client.GetActiveSprint(cfg.BoardID); err == nil {
			issues = refreshed
		}
	}

	return nil
}

// --- board data fetch spinner ---

type boardDataResult struct {
	boardCols []models.BoardColumn
	issues    []models.Issue
	err       error
}

type boardSpinnerModel struct {
	spinner spinner.Model
	label   string
	result  chan boardDataResult
	done    bool
	data    boardDataResult
}

func (m boardSpinnerModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, func() tea.Msg { return <-m.result })
}

func (m boardSpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case boardDataResult:
		m.done = true
		m.data = msg
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m boardSpinnerModel) View() string {
	if m.done {
		return ""
	}
	return m.spinner.View() + " " + m.label
}

func fetchBoardDataWithSpinner(client api.Client, boardID int) ([]models.BoardColumn, []models.Issue, error) {
	ch := make(chan boardDataResult, 1)
	go func() {
		cols, err := client.GetBoardColumns(boardID)
		if err != nil {
			ch <- boardDataResult{err: err}
			return
		}
		issues, err := client.GetActiveSprint(boardID)
		ch <- boardDataResult{boardCols: cols, issues: issues, err: err}
	}()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))

	p := tea.NewProgram(boardSpinnerModel{
		spinner: s,
		label:   "Fetching active sprint…",
		result:  ch,
	}, tea.WithOutput(os.Stderr))

	fm, err := p.Run()
	if err != nil {
		return nil, nil, err
	}
	bsm := fm.(boardSpinnerModel)
	return bsm.data.boardCols, bsm.data.issues, bsm.data.err
}

// --- kanban TUI ---

type kanbanState int

const (
	stateBoard   kanbanState = iota
	stateLoading             // fetching full issue for detail view
	stateDetail
)

type kanbanColumn struct {
	name   string
	issues []models.Issue
}

type issueFetchedMsg struct {
	issue *models.Issue
	err   error
}

type kanbanResult struct {
	editKey string // non-empty when the user pressed e
}

type kanbanModel struct {
	state      kanbanState
	client     api.Client
	width      int
	height     int
	quitting   bool
	result     kanbanResult

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

func newKanbanModel(client api.Client, boardCols []models.BoardColumn, issues []models.Issue, sprintName string) kanbanModel {
	cols := buildColumns(boardCols, issues)
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	return kanbanModel{
		state:       stateBoard,
		client:      client,
		columns:     cols,
		rowIdxs:     make([]int, len(cols)),
		sprintName:  sprintName,
		loadSpinner: s,
	}
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
			m.detailView.Width = msg.Width
			m.detailView.Height = msg.Height - 3
		}
		return m, nil

	case issueFetchedMsg:
		if msg.err != nil {
			// On error fall back to board.
			m.state = stateBoard
			return m, nil
		}
		m.detailIssue = msg.issue
		md, _ := display.RenderIssue(msg.issue)
		renderer, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(m.width),
		)
		content := md
		if err == nil {
			if rendered, err := renderer.Render(md); err == nil {
				content = rendered
			}
		}
		vp := viewport.New(m.width, m.height-3)
		vp.SetContent(content)
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
	}

	switch m.state {
	case stateBoard:
		return m.updateBoard(msg)
	case stateDetail:
		return m.updateDetail(msg)
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
		return m, tea.Quit
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
			return m, tea.Batch(m.loadSpinner.Tick, fetchIssueCmd(m.client, issue.Key))
		}
	case "e":
		if issue := m.currentIssue(); issue != nil {
			m.result = kanbanResult{editKey: issue.Key}
			return m, tea.Quit
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
				return m, tea.Quit
			}
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}
	// Forward all other messages (including j/k/pgup/pgdn) to viewport.
	var cmd tea.Cmd
	m.detailView, cmd = m.detailView.Update(msg)
	return m, cmd
}

func fetchIssueCmd(client api.Client, key string) tea.Cmd {
	return func() tea.Msg {
		issue, err := client.GetIssue(key)
		return issueFetchedMsg{issue: issue, err: err}
	}
}

func (m kanbanModel) View() string {
	switch m.state {
	case stateLoading:
		return "\n  " + m.loadSpinner.View() + " Loading issue…"
	case stateDetail:
		return m.viewDetail()
	default:
		return m.viewBoard()
	}
}

func (m kanbanModel) viewDetail() string {
	if m.detailIssue == nil {
		return ""
	}
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Padding(0, 1).
		Render(m.detailIssue.Key + "  " + m.detailIssue.Summary)
	footer := "\n" + dim.Render("  e: edit   esc/q: back to board   j/k: scroll")
	return header + "\n" + m.detailView.View() + footer
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
	innerWidth := colWidth - 4 // border (1 each side) + padding (1 each side)

	activeColStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(0, 1).
		Width(innerWidth)
	inactiveColStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(innerWidth)

	colHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	selKeyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("237"))
	selSumStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("237"))

	var renderedCols []string
	for ci, col := range m.columns {
		var lines []string

		title := strings.ToUpper(col.name) + fmt.Sprintf(" (%d)", len(col.issues))
		lines = append(lines, colHeaderStyle.Render(title))
		lines = append(lines, dim.Render(strings.Repeat("─", innerWidth)))

		if len(col.issues) == 0 {
			lines = append(lines, dim.Render("  (empty)"))
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
			if isSelected {
				lines = append(lines,
					selKeyStyle.Render("▶ "+issue.Key),
					selSumStyle.Render("  "+summary),
				)
			} else {
				lines = append(lines,
					"  "+keyStyle.Render(issue.Key),
					"  "+dim.Render(summary),
				)
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
		header = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Padding(0, 1).
			Render("Active Sprint: "+m.sprintName) + "\n"
	}
	footer := "\n" + dim.Render("  hjkl: navigate   enter: view   e: edit   q: quit")

	return header + board + footer
}

func runKanbanTUI(client api.Client, boardCols []models.BoardColumn, issues []models.Issue, sprintName string) (kanbanResult, error) {
	m := newKanbanModel(client, boardCols, issues, sprintName)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return kanbanResult{}, fmt.Errorf("kanban: %w", err)
	}
	return final.(kanbanModel).result, nil
}
