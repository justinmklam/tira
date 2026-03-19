package main

import (
	"fmt"
	"os"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justinmklam/lazyjira/internal/api"
	"github.com/justinmklam/lazyjira/internal/models"
	"github.com/justinmklam/lazyjira/internal/tui"
	"github.com/spf13/cobra"
)

type boardView int

const (
	viewBacklog boardView = iota
	viewKanban
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

type boardResult struct {
	editKey string
}

type boardModel struct {
	activeView boardView
	backlog    blModel
	kanban     kanbanModel
	client     api.Client
	boardID    int

	// Shared data for rebuilding views on refresh/switch.
	initData boardInitData

	width  int
	height int
	result boardResult
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

	for {
		result, err := runBoardTUI(client, cfg.BoardID, data, startView)
		if err != nil {
			return err
		}
		if result.editKey == "" {
			break
		}

		// Edit flow: must exit TUI for $EDITOR.
		issue, err := tui.RunWithSpinner(fmt.Sprintf("Fetching %s…", result.editKey), func() (*models.Issue, error) {
			return client.GetIssue(result.editKey)
		})
		if err != nil {
			return err
		}
		if err := runEditLoop(client, issue); err != nil {
			return err
		}

		// Re-fetch all data to reflect updates.
		if refreshed, err := fetchBoardData(client, cfg.BoardID); err == nil {
			data = refreshed
		}
	}

	return nil
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

	return boardModel{
		activeView: startView,
		backlog:    newBacklogModel(client, data.groups),
		kanban:     newKanbanModel(client, data.boardCols, issues, sprintName),
		client:     client,
		boardID:    boardID,
		initData:   data,
	}
}

func (m boardModel) Init() tea.Cmd { return nil }

func (m boardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Window size → forward to both sub-models.
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		m.height = ws.Height
		updated, _ := m.backlog.Update(ws)
		m.backlog = updated.(blModel)
		updated, _ = m.kanban.Update(ws)
		m.kanban = updated.(kanbanModel)
		return m, nil
	}

	// Board-level refresh result.
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
			m.result = boardResult{editKey: m.backlog.result.editKey}
			return m, tea.Quit
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
			m.result = boardResult{editKey: m.kanban.result.editKey}
			return m, tea.Quit
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
	return false
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
	switch m.activeView {
	case viewKanban:
		return m.kanban.View()
	default:
		return m.backlog.View()
	}
}

func runBoardTUI(client api.Client, boardID int, data boardInitData, startView boardView) (boardResult, error) {
	m := newBoardModel(client, boardID, data, startView)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return boardResult{}, fmt.Errorf("board: %w", err)
	}
	return final.(boardModel).result, nil
}
