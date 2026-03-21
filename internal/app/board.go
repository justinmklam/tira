package app

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/debug"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
)

// BoardView identifies which view is active in the board TUI.
type BoardView int

const (
	ViewBacklog BoardView = iota
	ViewKanban
	viewEditLoading    // fetching issue + valid values
	viewEdit           // huh form active
	viewEditSaving     // API call in flight
	viewCreateLoading  // fetching valid values for new issue
	viewCreate         // create form active
	viewCreateSaving   // create API call in flight
	viewAssigneePicker // assignee fuzzy picker (edit form or direct assignment)
	viewHelp           // help overlay
	viewComment        // comment textarea active
	viewCommentSaving  // comment API call in flight
)

// BoardInitData holds the initial fetch results needed by both views.
type BoardInitData struct {
	Groups    []models.SprintGroup
	BoardCols []models.BoardColumn
}

// boardRefreshDoneMsg is sent when an async refresh completes.
type boardRefreshDoneMsg struct {
	data BoardInitData
	err  error
}

type boardModel struct {
	activeView     BoardView
	prevView       BoardView // restored after edit completes
	backlog        blModel
	kanban         kanbanModel
	client         api.Client
	boardID        int
	jiraURL        string
	project        string
	classicProject bool

	// Shared data for rebuilding views on refresh/switch.
	initData BoardInitData

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

	// In-TUI comment state.
	commentKey     string
	commentSummary string
	commentForm    *commentInputModel
	commentErr     string
}

// fetchBoardDataCore fetches sprint groups and board columns concurrently.
// Returns BoardInitData on success, or an error if either fetch fails.
func fetchBoardDataCore(client api.Client, boardID int) (BoardInitData, error) {
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
		return BoardInitData{}, groupsErr
	}
	if colsErr != nil {
		return BoardInitData{}, colsErr
	}
	return BoardInitData{Groups: groups, BoardCols: boardCols}, nil
}

// FetchBoardData fetches sprint groups and board columns with a spinner.
func FetchBoardData(client api.Client, boardID int) (BoardInitData, error) {
	return tui.RunWithSpinner("Fetching board data…", func() (BoardInitData, error) {
		return fetchBoardDataCore(client, boardID)
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

func newBoardModel(client api.Client, boardID int, jiraURL, project string, classicProject bool, data BoardInitData, startView BoardView) boardModel {
	issues, sprintName := activeSprintFromGroups(data.Groups)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(tui.SpinnerColor)

	return boardModel{
		activeView:     startView,
		backlog:        newBacklogModel(client, data.Groups, project, jiraURL),
		kanban:         newKanbanModel(client, data.BoardCols, issues, sprintName, project),
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
		if m.activeView == viewComment && m.commentForm != nil {
			overlayW, overlayH := tui.OverlaySize(m.width, m.height)
			m.commentForm.setSize(overlayW-4, overlayH-4)
		}
		return m, nil
	}

	// Board-level refresh result (may arrive at any time).
	if msg, ok := msg.(boardRefreshDoneMsg); ok {
		if msg.err == nil {
			m.initData = msg.data
			m.backlog.refreshData(msg.data.Groups)
			issues, sprintName := activeSprintFromGroups(msg.data.Groups)
			m.kanban.refreshData(msg.data.BoardCols, issues, sprintName)
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

	case viewComment:
		if m.commentForm == nil {
			return m, nil
		}
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
			m.activeView = m.prevView
			m.commentForm = nil
			return m, nil
		}
		updated, cmd := m.commentForm.Update(msg)
		m.commentForm = updated.(*commentInputModel)
		if m.commentForm.completed {
			text := strings.TrimSpace(m.commentForm.ta.Value())
			m.activeView = viewCommentSaving
			m.commentErr = ""
			return m, tea.Batch(m.editSpinner.Tick, saveCommentCmd(m.client, m.commentKey, text))
		}
		if m.commentForm.aborted {
			m.activeView = m.prevView
			m.commentForm = nil
		}
		return m, cmd

	case viewCommentSaving:
		switch msg := msg.(type) {
		case spinner.TickMsg:
			var cmd tea.Cmd
			m.editSpinner, cmd = m.editSpinner.Update(msg)
			return m, cmd
		case commentSaveDoneMsg:
			m.commentForm = nil
			m.activeView = m.prevView
			if msg.err != nil {
				m.commentErr = fmt.Sprintf("Comment failed: %v", msg.err)
				return m, nil
			}
			// Refresh the detail view if we're returning to one.
			vpW, _ := tui.OverlayViewportSize(m.width, m.height)
			if m.prevView == ViewBacklog && m.backlog.state == blDetail && m.backlog.detailIssue != nil {
				key := m.backlog.detailIssue.Key
				m.backlog.state = blLoading
				return m, tea.Batch(m.backlog.loadSpinner.Tick, fetchIssueCmd(m.client, key, vpW))
			}
			if m.prevView == ViewKanban && m.kanban.state == stateDetail && m.kanban.detailIssue != nil {
				key := m.kanban.detailIssue.Key
				m.kanban.state = stateLoading
				return m, tea.Batch(m.kanban.loadSpinner.Tick, fetchIssueCmd(m.client, key, vpW))
			}
			return m, nil
		}
		return m, nil
	}

	// Open in browser (only when sub-model is in its base navigation state).
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "o" && m.canSwitchView() {
		var issueKey string
		switch m.activeView {
		case ViewBacklog:
			if m.backlog.state == blDetail && m.backlog.detailIssue != nil {
				issueKey = m.backlog.detailIssue.Key
			} else if issue := m.backlog.currentIssue(); issue != nil {
				issueKey = issue.Key
			}
		case ViewKanban:
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
			if m.activeView == ViewBacklog {
				m.activeView = ViewKanban
			} else {
				m.activeView = ViewBacklog
			}
			return m, nil
		case "1":
			m.activeView = ViewBacklog
			return m, nil
		case "2":
			m.activeView = ViewKanban
			return m, nil
		case "?":
			m.prevView = m.activeView
			m.activeView = viewHelp
			return m, nil
		}
	}

	// Delegate to the active sub-model.
	switch m.activeView {
	case ViewBacklog:
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
			m.prevView = ViewBacklog
			m.editKey = key
			m.activeView = viewEditLoading
			return m, tea.Batch(m.editSpinner.Tick, fetchEditDataCmd(m.client, key))
		}
		if m.backlog.result.commentKey != "" {
			key := m.backlog.result.commentKey
			m.commentSummary = m.backlog.result.commentSummary
			m.backlog.result.commentKey = ""
			m.backlog.result.commentSummary = ""
			m.prevView = ViewBacklog
			m.commentKey = key
			overlayW, overlayH := tui.OverlaySize(m.width, m.height)
			m.commentForm = newCommentInputModel(overlayW-4, overlayH-4)
			m.activeView = viewComment
			return m, m.commentForm.Init()
		}
		if m.backlog.result.refresh {
			m.backlog.result.refresh = false
			return m, m.refreshCmd()
		}
		if m.backlog.result.create {
			m.backlog.result.create = false
			m.createSprintID = m.backlog.result.createSprintID
			m.prevView = ViewBacklog
			m.activeView = viewCreateLoading
			return m, tea.Batch(m.editSpinner.Tick, fetchCreateDataCmd(m.client, m.project))
		}
		return m, cmd

	case ViewKanban:
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
			m.prevView = ViewKanban
			m.editKey = key
			m.activeView = viewEditLoading
			return m, tea.Batch(m.editSpinner.Tick, fetchEditDataCmd(m.client, key))
		}
		if m.kanban.result.commentKey != "" {
			key := m.kanban.result.commentKey
			m.commentSummary = m.kanban.result.commentSummary
			m.kanban.result.commentKey = ""
			m.kanban.result.commentSummary = ""
			m.prevView = ViewKanban
			m.commentKey = key
			overlayW, overlayH := tui.OverlaySize(m.width, m.height)
			m.commentForm = newCommentInputModel(overlayW-4, overlayH-4)
			m.activeView = viewComment
			return m, m.commentForm.Init()
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
	case ViewBacklog:
		return m.backlog.state == blList && !m.backlog.visualMode
	case ViewKanban:
		return m.kanban.state == stateBoard
	}
	return false // edit states: no switching
}

func (m boardModel) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		data, err := fetchBoardDataCore(m.client, m.boardID)
		if err != nil {
			return boardRefreshDoneMsg{err: err}
		}
		return boardRefreshDoneMsg{data: data}
	}
}

type commentSaveDoneMsg struct{ err error }

func saveCommentCmd(client api.Client, key, text string) tea.Cmd {
	return func() tea.Msg {
		err := client.AddComment(key, text)
		return commentSaveDoneMsg{err: err}
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

func (m boardModel) issueURL(key string) string {
	base := m.jiraURL
	projectPath := "projects"
	if m.classicProject {
		projectPath = "c/projects"
	}
	switch m.activeView {
	case ViewBacklog:
		return fmt.Sprintf("%s/jira/software/%s/%s/boards/%d/backlog?selectedIssue=%s", base, projectPath, m.project, m.boardID, key)
	case ViewKanban:
		return fmt.Sprintf("%s/jira/software/%s/%s/boards/%d?selectedIssue=%s", base, projectPath, m.project, m.boardID, key)
	default:
		return fmt.Sprintf("%s/browse/%s", base, key)
	}
}

// RunBoardTUI runs the interactive board TUI.
func RunBoardTUI(client api.Client, boardID int, jiraURL, project string, classicProject bool, data BoardInitData, startView BoardView) error {
	// Detect glamour style before handing the TTY to BubbleTea.
	// glamour.WithAutoStyle() queries the terminal for its background color,
	// which conflicts with BubbleTea's terminal reader when called from a
	// background goroutine. Detecting once here (while we still own the TTY)
	// caches the result in termenv's package-level sync.Once, so goroutines
	// can call WithAutoStyle() without blocking.
	_, _ = glamour.NewTermRenderer(glamour.WithAutoStyle())

	m := newBoardModel(client, boardID, jiraURL, project, classicProject, data, startView)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	if err != nil {
		debug.LogError("board.Run", err)
		return fmt.Errorf("board: %w", err)
	}
	return nil
}
