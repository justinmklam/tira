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
	viewTypePicker     // issue type option picker (edit/create form)
	viewPriorityPicker // priority option picker (edit/create form)
	viewHelp           // help overlay
	viewComment        // comment textarea active
	viewCommentSaving  // comment API call in flight
)

// maxInitialSprints is the number of sprint groups fetched during initial load.
// Remaining sprints and the backlog are lazy-loaded after the TUI renders.
const maxInitialSprints = 3

// BoardInitData holds the initial fetch results needed by both views.
type BoardInitData struct {
	Groups    []models.SprintGroup
	BoardCols []models.BoardColumn
	// RemainingSprints holds sprint metadata not yet fetched during initial load.
	// Empty after lazy loading completes or when all sprints fit in the initial batch.
	RemainingSprints []models.Sprint
}

// boardRefreshDoneMsg is sent when an async full-board refresh completes.
type boardRefreshDoneMsg struct {
	data BoardInitData
	err  error
}

// issueRefreshDoneMsg is sent after a single-issue re-fetch (post-edit).
type issueRefreshDoneMsg struct {
	issue *models.Issue
	err   error
}

// blLazyLoadDoneMsg is sent when the remaining sprint groups and backlog
// finish loading in the background after initial render.
type blLazyLoadDoneMsg struct {
	groups []models.SprintGroup // remaining sprints + backlog
	err    error
}

// issueInsertDoneMsg is sent after fetching a newly created issue so it can be
// inserted into the correct backlog sprint group without a full board reload.
type issueInsertDoneMsg struct {
	issue    *models.Issue
	sprintID int // 0 = backlog
	err      error
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

	// In-TUI type/priority picker state.
	typePicker     tui.OptionPickerModel
	priorityPicker tui.OptionPickerModel

	// Help overlay state.
	helpModel tui.HelpModel

	// In-TUI comment state.
	commentKey     string
	commentSummary string
	commentForm    *commentInputModel
	commentErr     string

	// Initial command (sidebar fetch).
	initCmd tea.Cmd
}

// fetchBoardDataCore fetches the first batch of sprint groups and board columns
// concurrently. Remaining sprints are returned in RemainingSprints for lazy loading.
// If projectFilter is non-empty, only issues from that project are included.
func fetchBoardDataCore(client api.Client, boardID int, projectFilter string) (BoardInitData, error) {
	var (
		allSprints []models.Sprint
		boardCols  []models.BoardColumn
		sprintsErr error
		colsErr    error
		wg         sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		allSprints, sprintsErr = client.GetSprintList(boardID)
	}()
	go func() {
		defer wg.Done()
		boardCols, colsErr = client.GetBoardColumns(boardID)
	}()
	wg.Wait()

	if sprintsErr != nil {
		return BoardInitData{}, sprintsErr
	}
	if colsErr != nil {
		return BoardInitData{}, colsErr
	}

	// Split sprints into initial batch and remainder.
	initialSprints := allSprints
	var remainingSprints []models.Sprint
	if len(allSprints) > maxInitialSprints {
		initialSprints = allSprints[:maxInitialSprints]
		remainingSprints = allSprints[maxInitialSprints:]
	}

	groups, err := client.GetSprintGroupsBatch(boardID, initialSprints)
	if err != nil {
		return BoardInitData{}, err
	}

	groups = filterGroupsByProject(groups, projectFilter)

	return BoardInitData{
		Groups:           groups,
		BoardCols:        boardCols,
		RemainingSprints: remainingSprints,
	}, nil
}

// fetchAllBoardDataCore fetches all sprint groups (no progressive loading).
// Used by manual refresh (R key) where the user expects a full reload.
func fetchAllBoardDataCore(client api.Client, boardID int, projectFilter string) (BoardInitData, error) {
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

	groups = filterGroupsByProject(groups, projectFilter)

	return BoardInitData{Groups: groups, BoardCols: boardCols}, nil
}

// filterGroupsByProject filters issues by project if specified, removing empty groups.
func filterGroupsByProject(groups []models.SprintGroup, projectFilter string) []models.SprintGroup {
	if projectFilter == "" {
		return groups
	}
	filteredGroups := make([]models.SprintGroup, 0, len(groups))
	for _, g := range groups {
		filtered := make([]models.Issue, 0, len(g.Issues))
		for _, issue := range g.Issues {
			if issue.ProjectKey == projectFilter {
				filtered = append(filtered, issue)
			}
		}
		if len(filtered) > 0 {
			g.Issues = filtered
			filteredGroups = append(filteredGroups, g)
		}
	}
	return filteredGroups
}

// FetchBoardData fetches sprint groups and board columns with a spinner.
// If projectFilter is non-empty, only issues from that project are included.
func FetchBoardData(client api.Client, boardID int, projectFilter string) (BoardInitData, error) {
	return tui.RunWithSpinner("Fetching board data…", func() (BoardInitData, error) {
		return fetchBoardDataCore(client, boardID, projectFilter)
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

func newBoardModel(client api.Client, boardID int, jiraURL, project string, classicProject bool, data BoardInitData, startView BoardView) (boardModel, tea.Cmd) {
	issues, sprintName := activeSprintFromGroups(data.Groups)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(tui.SpinnerColor)

	backlog, backlogCmd := newBacklogModel(client, boardID, data.Groups, project, jiraURL)

	cmds := []tea.Cmd{backlogCmd}

	// If there are remaining sprints to load, fire lazy load in background.
	if len(data.RemainingSprints) > 0 {
		cmds = append(cmds, lazyLoadCmd(client, boardID, data.RemainingSprints, project))
	} else {
		// All sprints loaded; still need to lazy-load the backlog.
		cmds = append(cmds, lazyLoadCmd(client, boardID, nil, project))
	}

	// Pre-warm the issue metadata cache so the edit form opens without blocking.
	if project != "" {
		cmds = append(cmds, metadataPreloadCmd(client, project))
	}

	initCmd := tea.Batch(cmds...)

	return boardModel{
		activeView:     startView,
		backlog:        backlog,
		kanban:         newKanbanModel(client, data.BoardCols, issues, sprintName, project),
		client:         client,
		boardID:        boardID,
		jiraURL:        strings.TrimRight(jiraURL, "/"),
		project:        project,
		classicProject: classicProject,
		initData:       data,
		editSpinner:    s,
		initCmd:        initCmd,
	}, initCmd
}

func (m boardModel) Init() tea.Cmd { return m.initCmd }

// metadataPreloadDoneMsg is sent when the background metadata preload finishes.
type metadataPreloadDoneMsg struct{ err error }

// metadataPreloadCmd warms the GetIssueMetadata cache entry so the edit form
// opens without waiting on those network requests.
func metadataPreloadCmd(client api.Client, projectKey string) tea.Cmd {
	return func() tea.Msg {
		_, err := client.GetIssueMetadata(projectKey)
		if err != nil {
			debug.LogError("metadataPreload: GetIssueMetadata", err)
		}
		return metadataPreloadDoneMsg{err: err}
	}
}

// lazyLoadCmd fetches remaining sprint groups and backlog issues in the background.
func lazyLoadCmd(client api.Client, boardID int, remainingSprints []models.Sprint, projectFilter string) tea.Cmd {
	return func() tea.Msg {
		var groups []models.SprintGroup

		// Fetch remaining sprint issues if any.
		if len(remainingSprints) > 0 {
			batch, err := client.GetSprintGroupsBatch(boardID, remainingSprints)
			if err != nil {
				return blLazyLoadDoneMsg{err: err}
			}
			groups = append(groups, batch...)
		}

		// Always fetch backlog.
		backlogIssues, err := client.GetBacklogIssues(boardID)
		if err == nil {
			groups = append(groups, models.SprintGroup{
				Sprint: models.Sprint{Name: "Backlog", State: "backlog"},
				Issues: backlogIssues,
			})
		}

		groups = filterGroupsByProject(groups, projectFilter)

		return blLazyLoadDoneMsg{groups: groups}
	}
}

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

	// Single-issue re-fetch after an edit (faster than a full board refresh).
	if msg, ok := msg.(issueRefreshDoneMsg); ok {
		if msg.err == nil && msg.issue != nil {
			m.backlog.patchIssue(*msg.issue)
			m.kanban.patchIssue(*msg.issue)
		}
		return m, nil
	}

	// New issue inserted: add it to the correct backlog group and navigate to it.
	if msg, ok := msg.(issueInsertDoneMsg); ok {
		if msg.err == nil && msg.issue != nil {
			m.backlog.insertIssue(*msg.issue, msg.sprintID)
			if m.createResultKey != "" {
				m.backlog.navigateToKey(m.createResultKey)
				m.createResultKey = ""
			}
		}
		return m, nil
	}

	// Board-level refresh result (may arrive at any time).
	if msg, ok := msg.(boardRefreshDoneMsg); ok {
		if msg.err == nil {
			m.initData = msg.data
			sidebarCmd := m.backlog.refreshData(msg.data.Groups)
			issues, sprintName := activeSprintFromGroups(msg.data.Groups)
			m.kanban.refreshData(msg.data.BoardCols, issues, sprintName)
			if m.createResultKey != "" {
				m.backlog.navigateToKey(m.createResultKey)
				m.createResultKey = ""
			}
			m.backlog.moving = false
			return m, sidebarCmd
		}
		m.backlog.moving = false
		return m, nil
	}

	// Lazy-loaded sprint groups + backlog arrived.
	if msg, ok := msg.(blLazyLoadDoneMsg); ok {
		if msg.err == nil && len(msg.groups) > 0 {
			m.backlog.appendGroups(msg.groups)
			// Update initData so future refreshes include all groups.
			m.initData.Groups = m.backlog.groups
			m.initData.RemainingSprints = nil
			// Update kanban if new groups contain an active sprint.
			issues, sprintName := activeSprintFromGroups(msg.groups)
			if len(issues) > 0 {
				m.kanban.refreshData(m.initData.BoardCols, issues, sprintName)
			}
		}
		return m, nil
	}

	// Metadata preload completed — result is already cached; nothing else to do.
	if _, ok := msg.(metadataPreloadDoneMsg); ok {
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
			m.activeView = viewAssigneePicker
			return m, m.assigneePicker.Init()
		}
		if m.editForm != nil && m.editForm.wantTypePicker {
			m.editForm.wantTypePicker = false
			m.typePicker = newTypePicker(m.editValid.IssueTypes, m.editForm.inputs[efType].Value())
			m.activeView = viewTypePicker
		}
		if m.editForm != nil && m.editForm.wantPriorityPicker {
			m.editForm.wantPriorityPicker = false
			m.priorityPicker = newPriorityPicker(m.editValid.Priorities, m.editForm.inputs[efPriority].Value())
			m.activeView = viewPriorityPicker
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
			return m, issueRefreshCmd(m.client, m.editKey)
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
			m.activeView = viewAssigneePicker
			return m, m.assigneePicker.Init()
		}
		if m.editForm != nil && m.editForm.wantTypePicker {
			m.editForm.wantTypePicker = false
			m.typePicker = newTypePicker(m.editValid.IssueTypes, m.editForm.inputs[efType].Value())
			m.activeView = viewTypePicker
		}
		if m.editForm != nil && m.editForm.wantPriorityPicker {
			m.editForm.wantPriorityPicker = false
			m.priorityPicker = newPriorityPicker(m.editValid.Priorities, m.editForm.inputs[efPriority].Value())
			m.activeView = viewPriorityPicker
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
				return m, issueInsertCmd(m.client, msg.issue.Key, m.createSprintID)
			}
			return m, m.refreshCmd()
		}
		return m, nil

	case viewAssigneePicker:
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
			if m.assigneeForEdit {
				m.editForm = nil
				m.activeView = m.prevView
			} else {
				m.activeView = m.prevView
			}
			return m, nil
		}
		updated, cmd := m.assigneePicker.Update(msg)
		m.assigneePicker = updated
		if m.assigneePicker.Aborted {
			if m.assigneeForEdit {
				editFormView := viewEdit
				if m.editKey == "" {
					editFormView = viewCreate
				}
				m.activeView = editFormView
			} else {
				m.activeView = m.prevView
			}
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
				editFormView := viewEdit
				if m.editKey == "" {
					editFormView = viewCreate
				}
				m.activeView = editFormView
				return m, nil
			}
		}
		return m, cmd

	case viewTypePicker:
		updated, cmd := m.typePicker.Update(msg)
		m.typePicker = updated
		editFormView := viewEdit
		if m.editKey == "" {
			editFormView = viewCreate
		}
		if m.typePicker.Aborted {
			m.activeView = editFormView
			return m, nil
		}
		if m.typePicker.Completed {
			if val := m.typePicker.SelectedItem(); val != "" && m.editForm != nil {
				m.editForm.inputs[efType].SetValue(val)
			}
			m.activeView = editFormView
			return m, nil
		}
		return m, cmd

	case viewPriorityPicker:
		updated, cmd := m.priorityPicker.Update(msg)
		m.priorityPicker = updated
		editFormView := viewEdit
		if m.editKey == "" {
			editFormView = viewCreate
		}
		if m.priorityPicker.Aborted {
			m.activeView = editFormView
			return m, nil
		}
		if m.priorityPicker.Completed {
			if val := m.priorityPicker.SelectedItem(); val != "" && m.editForm != nil {
				m.editForm.inputs[efPriority].SetValue(val)
			}
			m.activeView = editFormView
			return m, nil
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
		if inv, ok := m.client.(api.CacheInvalidator); ok {
			inv.Invalidate()
		}
		data, err := fetchAllBoardDataCore(m.client, m.boardID, m.project)
		if err != nil {
			return boardRefreshDoneMsg{err: err}
		}
		return boardRefreshDoneMsg{data: data}
	}
}

// issueRefreshCmd re-fetches a single issue after an edit. The cache entry for
// the key is already invalidated by cachedClient.UpdateIssue, so this always
// hits the API.
func issueRefreshCmd(client api.Client, key string) tea.Cmd {
	return func() tea.Msg {
		issue, err := client.GetIssue(key)
		return issueRefreshDoneMsg{issue: issue, err: err}
	}
}

// issueInsertCmd fetches a newly created issue and returns an issueInsertDoneMsg
// so the board can insert it into the correct sprint group without a full reload.
func issueInsertCmd(client api.Client, key string, sprintID int) tea.Cmd {
	return func() tea.Msg {
		issue, err := client.GetIssue(key)
		return issueInsertDoneMsg{issue: issue, sprintID: sprintID, err: err}
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

	m, _ := newBoardModel(client, boardID, jiraURL, project, classicProject, data, startView)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	if err != nil {
		debug.LogError("board.Run", err)
		return fmt.Errorf("board: %w", err)
	}
	return nil
}
