package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
)

// --- kanban TUI model ---

type kanbanState int

const (
	stateBoard   kanbanState = iota
	stateLoading             // fetching full issue for detail view
	stateDetail
	stateAssignPicker
	stateStatusPicker
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
	editKey        string // non-empty when the user pressed e
	commentKey     string // non-empty when the user pressed c
	commentSummary string
	refresh        bool
}

// kanbanBulkDoneMsg carries results from parallel bulk operations.
// Keys and Errors are parallel slices (nil error = success for that key).
// FullRefresh is set for status transitions where GetIssue does not return
// StatusID, so a targeted patch would leave column placement stale.
type kanbanBulkDoneMsg struct {
	Keys        []string
	Errors      []error
	FullRefresh bool
}

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

	// Status picker state
	statusPicker     tui.PickerModel
	statusTargetKeys []string
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
	s.Style = lipgloss.NewStyle().Foreground(tui.ColorSpinner)
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

// patchIssue updates a single issue in place within the kanban columns.
// Preserves agile-only fields not returned by GetIssue.
func (m *kanbanModel) patchIssue(fresh models.Issue) {
	for ci := range m.columns {
		for ii := range m.columns[ci].issues {
			if m.columns[ci].issues[ii].Key != fresh.Key {
				continue
			}
			existing := m.columns[ci].issues[ii]
			fresh.StatusID = existing.StatusID
			fresh.EpicKey = existing.EpicKey
			fresh.EpicName = existing.EpicName
			fresh.ProjectKey = existing.ProjectKey
			if fresh.StatusChangedDate == "" {
				fresh.StatusChangedDate = existing.StatusChangedDate
			}
			m.columns[ci].issues[ii] = fresh
		}
	}
	// Update detail overlay if it is showing this issue.
	if m.detailIssue != nil && m.detailIssue.Key == fresh.Key {
		m.detailIssue = &fresh
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

	case kanbanBulkDoneMsg:
		if msg.FullRefresh {
			// Status transitions need a full refresh: GetIssue does not return
			// StatusID, so a targeted patch would leave column placement stale.
			m.result.refresh = true
			return m, nil
		}
		// For assignee changes, re-fetch only the affected issues.
		var cmds []tea.Cmd
		for i, key := range msg.Keys {
			if i < len(msg.Errors) && msg.Errors[i] == nil {
				cmds = append(cmds, issueRefreshCmd(m.client, key))
			}
		}
		return m, tea.Batch(cmds...)
	}

	switch m.state {
	case stateBoard:
		return m.updateBoard(msg)
	case stateDetail:
		return m.updateDetail(msg)
	case stateAssignPicker:
		return m.updateAssignPicker(msg)
	case stateStatusPicker:
		return m.updateStatusPicker(msg)
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
	case "c":
		if issue := m.currentIssue(); issue != nil {
			m.result = kanbanResult{commentKey: issue.Key, commentSummary: issue.Summary}
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
	case "s":
		if issue := m.currentIssue(); issue != nil {
			m.statusTargetKeys = []string{issue.Key}
			m.statusPicker = kanbanNewStatusPicker(m.client, issue.Key, issue.Status)
			m.state = stateStatusPicker
			return m, m.statusPicker.Init()
		}
	case "R":
		m.result = kanbanResult{refresh: true}
		return m, nil
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
		case "c":
			if m.detailIssue != nil {
				m.result = kanbanResult{commentKey: m.detailIssue.Key, commentSummary: m.detailIssue.Summary}
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

func (m kanbanModel) updateStatusPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		m.quitting = true
		return m, nil
	}
	updated, cmd := m.statusPicker.Update(msg)
	m.statusPicker = updated
	if m.statusPicker.Aborted {
		m.state = stateBoard
		m.statusTargetKeys = nil
		return m, nil
	}
	if m.statusPicker.Completed {
		item := m.statusPicker.SelectedItem()
		var transitionID string
		if item != nil {
			transitionID = item.Value
		}
		keys := m.statusTargetKeys
		m.state = stateBoard
		m.statusTargetKeys = nil
		return m, kanbanTransitionStatusCmd(m.client, keys, transitionID)
	}
	return m, cmd
}

func kanbanAssignCmd(client api.Client, keys []string, accountID string) tea.Cmd {
	return func() tea.Msg {
		errors := client.BulkSetAssignee(keys, accountID)
		return kanbanBulkDoneMsg{Keys: keys, Errors: errors}
	}
}

func kanbanTransitionStatusCmd(client api.Client, keys []string, transitionID string) tea.Cmd {
	return func() tea.Msg {
		errors := client.BulkTransitionStatus(keys, transitionID)
		// FullRefresh: GetIssue does not return StatusID, so a targeted patch
		// would leave the kanban column placement stale after a status change.
		return kanbanBulkDoneMsg{Keys: keys, Errors: errors, FullRefresh: true}
	}
}

// kanbanNewStatusPicker creates a PickerModel whose search function returns
// available status transitions for the given issue.
// If currentStatus is non-empty, the picker will initially select the item
// whose label matches the current status.
func kanbanNewStatusPicker(client api.Client, issueKey, currentStatus string) tui.PickerModel {
	// Pre-fetch statuses to find the transition ID for the current status.
	var initialValue string
	if currentStatus != "" {
		if statuses, err := client.GetStatuses(issueKey); err == nil {
			for _, s := range statuses {
				if s.Name == currentStatus {
					initialValue = s.ID
					break
				}
			}
		}
	}

	search := func(query string) ([]tui.PickerItem, error) {
		statuses, err := client.GetStatuses(issueKey)
		if err != nil {
			return nil, err
		}
		items := make([]tui.PickerItem, len(statuses))
		for i, s := range statuses {
			items[i] = tui.PickerItem{Label: s.Name, SubLabel: "", Value: s.ID}
		}
		// Filter by query if provided.
		if query != "" {
			q := strings.ToLower(query)
			filtered := items[:0]
			for _, item := range items {
				if strings.Contains(strings.ToLower(item.Label), q) {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		return items, nil
	}
	m := tui.NewPickerModel(search)
	m.InitialValue = initialValue
	return m
}
