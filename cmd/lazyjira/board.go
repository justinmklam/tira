package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/lazyjira/internal/api"
	"github.com/justinmklam/lazyjira/internal/debug"
	"github.com/justinmklam/lazyjira/internal/models"
	"github.com/justinmklam/lazyjira/internal/tui"
	"github.com/spf13/cobra"
)

type boardView int

const (
	viewBacklog boardView = iota
	viewKanban
	viewEditLoading    // fetching issue + valid values
	viewEdit           // huh form active
	viewEditSaving     // API call in flight
	viewCreateLoading  // fetching valid values for new issue
	viewCreate         // create form active
	viewCreateSaving   // create API call in flight
	viewAssigneePicker // assignee fuzzy picker (edit form or direct assignment)
	viewHelp           // help overlay
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
	activeView     boardView
	prevView       boardView // restored after edit completes
	backlog        blModel
	kanban         kanbanModel
	client         api.Client
	boardID        int
	jiraURL        string
	project        string
	classicProject bool

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

	// In-TUI create state.
	createSprintID  int
	createResultKey string // key of newly created issue; used to navigate after refresh

	// In-TUI assignee picker state.
	assigneePicker  tui.PickerModel
	assigneeForEdit bool // true = inject result into editForm; false = used externally

	// Help overlay state.
	helpModel tui.HelpModel
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

func newBoardModel(client api.Client, boardID int, jiraURL, project string, classicProject bool, data boardInitData, startView boardView) boardModel {
	issues, sprintName := activeSprintFromGroups(data.groups)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(tui.SpinnerColor)

	return boardModel{
		activeView:     startView,
		backlog:        newBacklogModel(client, data.groups, project, jiraURL),
		kanban:         newKanbanModel(client, data.boardCols, issues, sprintName, project),
		client:         client,
		boardID:        boardID,
		jiraURL:        strings.TrimRight(jiraURL, "/"),
		project:        project,
		classicProject: classicProject,
		initData:       data,
		editSpinner:    s,
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
		if (m.activeView == viewEdit || m.activeView == viewCreate) && m.editForm != nil {
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
			if m.createResultKey != "" {
				m.backlog.navigateToKey(m.createResultKey)
				m.createResultKey = ""
			}
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
		if m.editForm != nil && m.editForm.wantAssigneePicker {
			m.editForm.wantAssigneePicker = false
			projectKey := m.project
			if projectKey == "" {
				if idx := strings.Index(m.editKey, "-"); idx > 0 {
					projectKey = m.editKey[:idx]
				}
			}
			m.assigneePicker = newAssigneePicker(m.client, projectKey)
			m.assigneeForEdit = true
			m.prevView = m.activeView
			m.activeView = viewAssigneePicker
			return m, m.assigneePicker.Init()
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

	case viewCreateLoading:
		switch msg := msg.(type) {
		case spinner.TickMsg:
			var cmd tea.Cmd
			m.editSpinner, cmd = m.editSpinner.Update(msg)
			return m, cmd
		case createFetchedMsg:
			if msg.err != nil {
				m.activeView = m.prevView
				return m, nil
			}
			m.editValid = msg.valid
			blank := blankIssueFromValid(msg.valid)
			overlayW, overlayH := tui.OverlaySize(m.width, m.height)
			m.editForm = newEditModel(blank, msg.valid, overlayW-4, overlayH-4)
			m.activeView = viewCreate
			return m, m.editForm.Init()
		}
		return m, nil

	case viewCreate:
		if m.editForm == nil {
			return m, nil
		}
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
			m.activeView = m.prevView
			m.editForm = nil
			return m, nil
		}
		updated, cmd := m.editForm.Update(msg)
		m.editForm = updated.(*editModel)
		if m.editForm.completed {
			fields := m.editForm.currentState().toIssueFields(m.editValid)
			m.activeView = viewCreateSaving
			m.editErr = ""
			return m, tea.Batch(m.editSpinner.Tick, saveCreateCmd(m.client, m.project, fields, m.createSprintID))
		}
		if m.editForm.aborted {
			m.activeView = m.prevView
			m.editForm = nil
		}
		if m.editForm != nil && m.editForm.wantAssigneePicker {
			m.editForm.wantAssigneePicker = false
			projectKey := m.project
			if projectKey == "" {
				if idx := strings.Index(m.editKey, "-"); idx > 0 {
					projectKey = m.editKey[:idx]
				}
			}
			m.assigneePicker = newAssigneePicker(m.client, projectKey)
			m.assigneeForEdit = true
			m.prevView = m.activeView
			m.activeView = viewAssigneePicker
			return m, m.assigneePicker.Init()
		}
		return m, cmd

	case viewCreateSaving:
		switch msg := msg.(type) {
		case spinner.TickMsg:
			var cmd tea.Cmd
			m.editSpinner, cmd = m.editSpinner.Update(msg)
			return m, cmd
		case createSaveDoneMsg:
			m.editForm = nil
			m.activeView = m.prevView
			if msg.err != nil {
				m.editErr = fmt.Sprintf("Create failed: %v", msg.err)
				return m, nil
			}
			if msg.issue != nil {
				m.createResultKey = msg.issue.Key
			}
			return m, m.refreshCmd()
		}
		return m, nil

	case viewAssigneePicker:
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
			m.activeView = m.prevView
			m.editForm = nil
			return m, nil
		}
		updated, cmd := m.assigneePicker.Update(msg)
		m.assigneePicker = updated
		if m.assigneePicker.Aborted {
			m.activeView = m.prevView
			return m, nil
		}
		if m.assigneePicker.Completed {
			item := m.assigneePicker.SelectedItem()
			if m.assigneeForEdit && m.editForm != nil {
				if item != nil {
					m.editForm.setAssignee(item.Label, item.Value)
				} else {
					m.editForm.setAssignee("", "")
				}
				m.activeView = m.prevView
				return m, nil
			}
		}
		return m, cmd

	case viewHelp:
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "esc", "?":
				m.activeView = m.prevView
				return m, nil
			case "ctrl+c":
				m.activeView = m.prevView
				return m, nil
			}
		}
		// Update help model for scrolling
		_, overlayH := tui.HelpOverlaySize(m.width, m.height)
		innerH := overlayH - 2 // account for border only
		m.helpModel = m.helpModel.Update(msg, innerH)
		return m, nil
	}

	// Open in browser (only when sub-model is in its base navigation state).
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "o" && m.canSwitchView() {
		var issueKey string
		switch m.activeView {
		case viewBacklog:
			if m.backlog.state == blDetail && m.backlog.detailIssue != nil {
				issueKey = m.backlog.detailIssue.Key
			} else if issue := m.backlog.currentIssue(); issue != nil {
				issueKey = issue.Key
			}
		case viewKanban:
			if m.kanban.state == stateDetail && m.kanban.detailIssue != nil {
				issueKey = m.kanban.detailIssue.Key
			} else if issue := m.kanban.currentIssue(); issue != nil {
				issueKey = issue.Key
			}
		}
		if issueKey != "" {
			return m, openInBrowserCmd(m.issueURL(issueKey))
		}
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
		case "?":
			m.prevView = m.activeView
			m.activeView = viewHelp
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
		if m.backlog.result.create {
			m.backlog.result.create = false
			m.createSprintID = m.backlog.result.createSprintID
			m.prevView = viewBacklog
			m.activeView = viewCreateLoading
			return m, tea.Batch(m.editSpinner.Tick, fetchCreateDataCmd(m.client, m.project))
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
		if m.kanban.result.refresh {
			m.kanban.result.refresh = false
			return m, m.refreshCmd()
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

	case viewCreateLoading:
		msg := m.editSpinner.View() + tui.DimStyle.Render(" Loading…")
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, msg)

	case viewCreate:
		return m.viewEditForm(w, h)

	case viewCreateSaving:
		msg := m.editSpinner.View() + tui.DimStyle.Render(" Creating issue…")
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, msg)

	case viewAssigneePicker:
		return m.viewAssigneePickerOverlay(w, h)

	case viewHelp:
		return m.viewHelpOverlay(w, h)

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

	var titleStr string
	switch m.activeView {
	case viewCreate:
		if m.createSprintID == 0 {
			titleStr = "New Issue  (backlog)"
		} else {
			titleStr = "New Issue"
		}
	default:
		titleStr = m.editKey
		if m.editIssue != nil {
			titleStr = m.editIssue.Key + "  " + m.editIssue.Summary
		}
	}
	header := tui.BoldBlue.Padding(0, 1).Width(innerW).
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

func (m boardModel) issueURL(key string) string {
	base := m.jiraURL
	projectPath := "projects"
	if m.classicProject {
		projectPath = "c/projects"
	}
	switch m.activeView {
	case viewBacklog:
		return fmt.Sprintf("%s/jira/software/%s/%s/boards/%d/backlog?selectedIssue=%s", base, projectPath, m.project, m.boardID, key)
	case viewKanban:
		return fmt.Sprintf("%s/jira/software/%s/%s/boards/%d?selectedIssue=%s", base, projectPath, m.project, m.boardID, key)
	default:
		return fmt.Sprintf("%s/browse/%s", base, key)
	}
}

func openInBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", url)
		default:
			cmd = exec.Command("xdg-open", url)
		}
		_ = cmd.Start()
		return nil
	}
}

func (m boardModel) viewAssigneePickerOverlay(w, h int) string {
	pickerW := w * 2 / 3
	if pickerW < 52 {
		pickerW = 52
	}
	if pickerW > 90 {
		pickerW = 90
	}
	innerW := pickerW - 2

	header := tui.BoldBlue.Padding(0, 1).Width(innerW).
		Render(tui.FixedWidth("Set Assignee", innerW-2))

	listH := h/2 - 6
	if listH < 4 {
		listH = 4
	}

	footer := tui.DimStyle.Render("  ↑/↓ ctrl+p/n: navigate   enter: select   esc: cancel")

	body := header + "\n" +
		m.assigneePicker.View(innerW, listH) + "\n" +
		tui.DimStyle.Render(strings.Repeat("─", innerW)) + "\n" +
		footer

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBlue).
		Width(innerW).
		Render(body)

	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal)
}

func (m boardModel) viewHelpOverlay(w, h int) string {
	overlayW, overlayH := tui.HelpOverlaySize(w, h)
	innerW := overlayW - 2
	innerH := overlayH - 2 // account for border only

	// Get the help content
	helpContent := m.helpModel.View(innerW, innerH)

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.ColorBlue).
		Width(innerW).
		Render(helpContent)

	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal)
}

func runBoardTUI(client api.Client, boardID int, data boardInitData, startView boardView) error {
	// Detect glamour style before handing the TTY to BubbleTea.
	// glamour.WithAutoStyle() queries the terminal for its background color,
	// which conflicts with BubbleTea's terminal reader when called from a
	// background goroutine. Detecting once here (while we still own the TTY)
	// caches the result in termenv's package-level sync.Once, so goroutines
	// can call WithAutoStyle() without blocking.
	_, _ = glamour.NewTermRenderer(glamour.WithAutoStyle())

	m := newBoardModel(client, boardID, cfg.JiraURL, cfg.Project, cfg.ClassicProject, data, startView)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	if err != nil {
		debug.LogError("board.Run", err)
		return fmt.Errorf("board: %w", err)
	}
	return nil
}
