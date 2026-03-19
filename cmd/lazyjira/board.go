package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/lazyjira/internal/api"
	"github.com/justinmklam/lazyjira/internal/models"
	"github.com/justinmklam/lazyjira/internal/tui"
	"github.com/spf13/cobra"
)

type boardView int

const (
	viewBacklog     boardView = iota
	viewKanban
	viewEditLoading // fetching issue + valid values
	viewEdit        // huh form active
	viewEditSaving  // API call in flight
)

// boardInitData holds the initial fetch results needed by both views.
type boardInitData struct {
	groups    []models.SprintGroup
	boardCols []models.BoardColumn
}

// boardRefreshDoneMsg is sent when an async refresh completes.
type boardRefreshDoneMsg struct {
	data boardInitData
	err  error
}

type boardModel struct {
	activeView boardView
	prevView   boardView // restored after edit completes
	backlog    blModel
	kanban     kanbanModel
	client     api.Client
	boardID    int

	// Shared data for rebuilding views on refresh/switch.
	initData boardInitData

	width  int
	height int

	// In-TUI edit state.
	editKey     string
	editIssue   *models.Issue
	editValid   *models.ValidValues
	editForm    *editModel
	editErr     string // last save error message
	editSpinner spinner.Model
}

var boardCmd = &cobra.Command{
	Use:   "board",
	Short: "Interactive board with backlog and kanban views (Tab to toggle)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBoardCmd(viewBacklog)
	},
}

var backlogCmd = &cobra.Command{
	Use:   "backlog",
	Short: "Show the project backlog (Tab to switch to kanban)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBoardCmd(viewBacklog)
	},
}

var kanbanCmd = &cobra.Command{
	Use:   "kanban",
	Short: "Show the active sprint as a kanban board (Tab to switch to backlog)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBoardCmd(viewKanban)
	},
}

func init() {
	rootCmd.AddCommand(boardCmd)
	rootCmd.AddCommand(backlogCmd)
	rootCmd.AddCommand(kanbanCmd)
}

func runBoardCmd(startView boardView) error {
	if cfg.BoardID == 0 {
		return fmt.Errorf("board ID not configured: set default_board_id in ~/.config/lazyjira/config.yaml")
	}

	client, err := api.NewClient(cfg)
	if err != nil {
		return err
	}

	data, err := fetchBoardData(client, cfg.BoardID)
	if err != nil {
		return err
	}
	if len(data.groups) == 0 {
		fmt.Fprintln(os.Stderr, "No sprints or backlog issues found.")
		return nil
	}

	return runBoardTUI(client, cfg.BoardID, data, startView)
}

func fetchBoardData(client api.Client, boardID int) (boardInitData, error) {
	return tui.RunWithSpinner("Fetching board data…", func() (boardInitData, error) {
		var (
			groups    []models.SprintGroup
			boardCols []models.BoardColumn
			groupsErr error
			colsErr   error
			wg        sync.WaitGroup
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			groups, groupsErr = client.GetSprintGroups(boardID)
		}()
		go func() {
			defer wg.Done()
			boardCols, colsErr = client.GetBoardColumns(boardID)
		}()
		wg.Wait()

		if groupsErr != nil {
			return boardInitData{}, groupsErr
		}
		if colsErr != nil {
			return boardInitData{}, colsErr
		}
		return boardInitData{groups: groups, boardCols: boardCols}, nil
	})
}

// activeSprintFromGroups finds the active sprint's issues and name from sprint groups.
func activeSprintFromGroups(groups []models.SprintGroup) ([]models.Issue, string) {
	for _, g := range groups {
		if g.Sprint.State == "active" {
			return g.Issues, g.Sprint.Name
		}
	}
	return nil, ""
}

func newBoardModel(client api.Client, boardID int, data boardInitData, startView boardView) boardModel {
	issues, sprintName := activeSprintFromGroups(data.groups)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(tui.SpinnerColor)

	return boardModel{
		activeView:  startView,
		backlog:     newBacklogModel(client, data.groups),
		kanban:      newKanbanModel(client, data.boardCols, issues, sprintName),
		client:      client,
		boardID:     boardID,
		initData:    data,
		editSpinner: s,
	}
}

func (m boardModel) Init() tea.Cmd { return nil }

func (m boardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Window size is always forwarded.
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		m.height = ws.Height
		updated, _ := m.backlog.Update(ws)
		m.backlog = updated.(blModel)
		updated, _ = m.kanban.Update(ws)
		m.kanban = updated.(kanbanModel)
		if m.activeView == viewEdit && m.editForm != nil {
			overlayW, overlayH := tui.OverlaySize(m.width, m.height)
			m.editForm.setSize(overlayW-4, overlayH-4)
		}
		return m, nil
	}

	// Board-level refresh result (may arrive at any time).
	if msg, ok := msg.(boardRefreshDoneMsg); ok {
		if msg.err == nil {
			m.initData = msg.data
			m.backlog.refreshData(msg.data.groups)
			issues, sprintName := activeSprintFromGroups(msg.data.groups)
			m.kanban.refreshData(msg.data.boardCols, issues, sprintName)
		}
		m.backlog.moving = false
		return m, nil
	}

	// --- Edit state machine ---
	switch m.activeView {
	case viewEditLoading:
		switch msg := msg.(type) {
		case spinner.TickMsg:
			var cmd tea.Cmd
			m.editSpinner, cmd = m.editSpinner.Update(msg)
			return m, cmd
		case editFetchedMsg:
			if msg.err != nil {
				// Return to previous view on error.
				m.activeView = m.prevView
				return m, nil
			}
			m.editIssue = msg.issue
			m.editValid = msg.valid
			overlayW, overlayH := tui.OverlaySize(m.width, m.height)
			m.editForm = newEditModel(msg.issue, msg.valid, overlayW-4, overlayH-4)
			m.activeView = viewEdit
			return m, m.editForm.Init()
		}
		return m, nil

	case viewEdit:
		if m.editForm == nil {
			return m, nil
		}
		// Intercept ctrl+c so it cancels the form rather than quitting.
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
			m.activeView = m.prevView
			m.editForm = nil
			return m, nil
		}
		updated, cmd := m.editForm.Update(msg)
		m.editForm = updated.(*editModel)
		if m.editForm.completed {
			fields := m.editForm.currentState().toIssueFields(m.editValid)
			m.activeView = viewEditSaving
			m.editErr = ""
			return m, tea.Batch(m.editSpinner.Tick, saveEditCmd(m.client, m.editKey, fields))
		}
		if m.editForm.aborted {
			m.activeView = m.prevView
			m.editForm = nil
		}
		return m, cmd

	case viewEditSaving:
		switch msg := msg.(type) {
		case spinner.TickMsg:
			var cmd tea.Cmd
			m.editSpinner, cmd = m.editSpinner.Update(msg)
			return m, cmd
		case editSaveDoneMsg:
			m.editForm = nil
			m.editIssue = nil
			m.activeView = m.prevView
			if msg.err != nil {
				m.editErr = fmt.Sprintf("Save failed: %v", msg.err)
				return m, nil
			}
			return m, m.refreshCmd()
		}
		return m, nil
	}

	// View-switching keys: only when the sub-model is in its base state.
	if key, ok := msg.(tea.KeyMsg); ok && m.canSwitchView() {
		switch key.String() {
		case "tab":
			if m.activeView == viewBacklog {
				m.activeView = viewKanban
			} else {
				m.activeView = viewBacklog
			}
			return m, nil
		case "1":
			m.activeView = viewBacklog
			return m, nil
		case "2":
			m.activeView = viewKanban
			return m, nil
		}
	}

	// Delegate to the active sub-model.
	switch m.activeView {
	case viewBacklog:
		updated, cmd := m.backlog.Update(msg)
		m.backlog = updated.(blModel)

		if m.backlog.quitting {
			return m, tea.Quit
		}
		if m.backlog.result.editKey != "" {
			key := m.backlog.result.editKey
			m.backlog.result.editKey = ""
			// If editing from the detail view, return to the list.
			if m.backlog.state == blDetail {
				m.backlog.state = blList
				m.backlog.detailIssue = nil
			}
			m.prevView = viewBacklog
			m.editKey = key
			m.activeView = viewEditLoading
			return m, tea.Batch(m.editSpinner.Tick, fetchEditDataCmd(m.client, key))
		}
		if m.backlog.result.refresh {
			m.backlog.result.refresh = false
			return m, m.refreshCmd()
		}
		return m, cmd

	case viewKanban:
		updated, cmd := m.kanban.Update(msg)
		m.kanban = updated.(kanbanModel)

		if m.kanban.quitting {
			return m, tea.Quit
		}
		if m.kanban.result.editKey != "" {
			key := m.kanban.result.editKey
			m.kanban.result.editKey = ""
			// If editing from the detail view, return to the board.
			if m.kanban.state == stateDetail {
				m.kanban.state = stateBoard
				m.kanban.detailIssue = nil
			}
			m.prevView = viewKanban
			m.editKey = key
			m.activeView = viewEditLoading
			return m, tea.Batch(m.editSpinner.Tick, fetchEditDataCmd(m.client, key))
		}
		return m, cmd
	}

	return m, nil
}

// canSwitchView returns true when the active sub-model is in its base
// navigation state (not filtering, not in detail view, not in visual mode).
func (m boardModel) canSwitchView() bool {
	switch m.activeView {
	case viewBacklog:
		return m.backlog.state == blList && !m.backlog.visualMode
	case viewKanban:
		return m.kanban.state == stateBoard
	}
	return false // edit states: no switching
}

func (m boardModel) refreshCmd() tea.Cmd {
	client := m.client
	boardID := m.boardID
	return func() tea.Msg {
		var (
			groups    []models.SprintGroup
			boardCols []models.BoardColumn
			groupsErr error
			colsErr   error
			wg        sync.WaitGroup
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			groups, groupsErr = client.GetSprintGroups(boardID)
		}()
		go func() {
			defer wg.Done()
			boardCols, colsErr = client.GetBoardColumns(boardID)
		}()
		wg.Wait()

		if groupsErr != nil {
			return boardRefreshDoneMsg{err: groupsErr}
		}
		if colsErr != nil {
			return boardRefreshDoneMsg{err: colsErr}
		}
		return boardRefreshDoneMsg{data: boardInitData{groups: groups, boardCols: boardCols}}
	}
}

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

	case viewKanban:
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

	titleStr := m.editKey
	if m.editIssue != nil {
		titleStr = m.editIssue.Key + "  " + m.editIssue.Summary
	}
	header := tui.BoldBlue.Copy().Padding(0, 1).Width(innerW).
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

func runBoardTUI(client api.Client, boardID int, data boardInitData, startView boardView) error {
	m := newBoardModel(client, boardID, data, startView)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("board: %w", err)
	}
	return nil
}
