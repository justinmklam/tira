package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/justinmklam/lazyjira/internal/api"
	"github.com/justinmklam/lazyjira/internal/display"
	"github.com/justinmklam/lazyjira/internal/models"
	"github.com/justinmklam/lazyjira/internal/tui"
)

// --- backlog TUI model ---

type blState int

const (
	blList    blState = iota
	blLoading         // fetching full issue for detail view
	blDetail
	blFilter
)

type blRowKind int

const (
	blRowSprint blRowKind = iota
	blRowIssue
	blRowSpacer // blank line between sprint groups
)

type blRow struct {
	kind     blRowKind
	groupIdx int
	issueIdx int // -1 for sprint header rows
}

type blResult struct {
	editKey string
	refresh bool
}

type blMoveMultiDoneMsg struct {
	movedKeys      []string
	targetGroupIdx int
	err            error
}

type blModel struct {
	state  blState
	client api.Client

	groups    []models.SprintGroup
	rows      []blRow
	cursor    int
	offset    int
	collapsed map[int]bool

	filter      string
	filterInput textinput.Model

	width  int
	height int

	loadSpinner spinner.Model
	detailIssue *models.Issue
	detailView  viewport.Model

	selected     map[string]bool // issue keys marked with spacebar
	cutKeys      map[string]bool // issue keys marked for move with 'x'
	visualMode   bool
	visualAnchor int  // row index where 'v' was pressed
	moving       bool // true while a move API call is in flight

	result   blResult
	quitting bool
}

func blBuildRows(groups []models.SprintGroup, collapsed map[int]bool, filter string) []blRow {
	var rows []blRow
	for i, g := range groups {
		if i > 0 {
			rows = append(rows, blRow{kind: blRowSpacer, groupIdx: i})
		}
		rows = append(rows, blRow{kind: blRowSprint, groupIdx: i, issueIdx: -1})
		if !collapsed[i] {
			for j, issue := range g.Issues {
				if filter == "" || blMatchesFilter(issue, filter) {
					rows = append(rows, blRow{kind: blRowIssue, groupIdx: i, issueIdx: j})
				}
			}
		}
	}
	return rows
}

func blMatchesFilter(issue models.Issue, filter string) bool {
	f := strings.ToLower(filter)
	return strings.Contains(strings.ToLower(issue.Key), f) ||
		strings.Contains(strings.ToLower(issue.Summary), f)
}

func newBacklogModel(client api.Client, groups []models.SprintGroup) blModel {
	collapsed := make(map[int]bool)

	ti := textinput.New()
	ti.Placeholder = "type to filter…"
	ti.CharLimit = 60

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(tui.SpinnerColor)

	m := blModel{
		state:       blList,
		client:      client,
		groups:      groups,
		collapsed:   collapsed,
		filterInput: ti,
		loadSpinner: s,
		selected:    make(map[string]bool),
		cutKeys:     make(map[string]bool),
	}
	m.rows = blBuildRows(groups, collapsed, "")
	return m
}

// refreshData replaces the sprint groups and rebuilds the row list.
func (m *blModel) refreshData(groups []models.SprintGroup) {
	m.groups = groups
	m.rows = blBuildRows(groups, m.collapsed, m.filter)
	m.cursor = tui.Clamp(m.cursor, 0, max(len(m.rows)-1, 0))
}

func (m blModel) viewHeight() int {
	if m.height < 5 {
		return 1
	}
	return m.height - 3 // top bar + column header + footer
}

// visualIssueKeys returns the set of issue keys spanned by the visual selection range.
func (m blModel) visualIssueKeys() map[string]bool {
	if !m.visualMode {
		return nil
	}
	lo, hi := m.visualAnchor, m.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	keys := make(map[string]bool)
	for i := lo; i <= hi; i++ {
		if i < len(m.rows) && m.rows[i].kind == blRowIssue {
			row := m.rows[i]
			issue := m.groups[row.groupIdx].Issues[row.issueIdx]
			keys[issue.Key] = true
		}
	}
	return keys
}

// allSelected returns the effective selection: base selection toggled by any
// active visual range (so reselecting an already-selected item deselects it).
func (m blModel) allSelected() map[string]bool {
	if !m.visualMode && len(m.selected) == 0 {
		return nil
	}
	combined := make(map[string]bool, len(m.selected))
	for k := range m.selected {
		combined[k] = true
	}
	for k := range m.visualIssueKeys() {
		if combined[k] {
			delete(combined, k)
		} else {
			combined[k] = true
		}
	}
	return combined
}

// moveKeys returns the issue keys to operate on: the combined selection if any,
// otherwise just the current issue.
func (m blModel) moveKeys() []string {
	if combined := m.allSelected(); len(combined) > 0 {
		keys := make([]string, 0, len(combined))
		for k := range combined {
			keys = append(keys, k)
		}
		return keys
	}
	if issue := m.currentIssue(); issue != nil {
		return []string{issue.Key}
	}
	return nil
}

func (m blModel) currentIssue() *models.Issue {
	if m.cursor >= len(m.rows) {
		return nil
	}
	row := m.rows[m.cursor]
	if row.kind != blRowIssue {
		return nil
	}
	return &m.groups[row.groupIdx].Issues[row.issueIdx]
}

func blScrollToFit(m blModel) blModel {
	vh := m.viewHeight()
	if vh <= 0 {
		return m
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+vh {
		m.offset = m.cursor - vh + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
	return m
}

// commitVisualSelection merges the visual range into the base selection.
func (m *blModel) commitVisualSelection() {
	for k := range m.visualIssueKeys() {
		if m.selected[k] {
			delete(m.selected, k)
		} else {
			m.selected[k] = true
		}
	}
	m.visualMode = false
}

func (m blModel) Init() tea.Cmd { return nil }

func (m blModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.state == blDetail {
			m.detailView.Width = tui.DetailPaneWidth(m.width)
			m.detailView.Height = msg.Height - 3
		}
		return m, nil

	case issueFetchedMsg:
		if msg.err != nil {
			m.state = blList
			return m, nil
		}
		m.detailIssue = msg.issue
		detailW := tui.DetailPaneWidth(m.width)
		md, _ := display.RenderIssue(msg.issue)
		renderer, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(detailW),
		)
		content := md
		if err == nil {
			if rendered, err2 := renderer.Render(md); err2 == nil {
				content = rendered
			}
		}
		vp := viewport.New(detailW, m.height-3)
		vp.SetContent(content)
		m.detailView = vp
		m.state = blDetail
		return m, nil

	case spinner.TickMsg:
		if m.state == blLoading || m.moving {
			var cmd tea.Cmd
			m.loadSpinner, cmd = m.loadSpinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case blMoveMultiDoneMsg:
		m.moving = false
		if msg.err == nil {
			movedSet := make(map[string]bool, len(msg.movedKeys))
			for _, k := range msg.movedKeys {
				movedSet[k] = true
			}
			var movedIssues []models.Issue
			for i := range m.groups {
				var remaining []models.Issue
				for _, issue := range m.groups[i].Issues {
					if movedSet[issue.Key] && i != msg.targetGroupIdx {
						movedIssues = append(movedIssues, issue)
					} else {
						remaining = append(remaining, issue)
					}
				}
				m.groups[i].Issues = remaining
			}
			m.groups[msg.targetGroupIdx].Issues = append(m.groups[msg.targetGroupIdx].Issues, movedIssues...)
			m.selected = make(map[string]bool)
			m.cutKeys = make(map[string]bool)
			m.visualMode = false
			m.rows = blBuildRows(m.groups, m.collapsed, m.filter)
			m.cursor = tui.Clamp(m.cursor, 0, len(m.rows)-1)
			return blScrollToFit(m), nil
		}
		return m, nil
	}

	switch m.state {
	case blList:
		return m.updateList(msg)
	case blFilter:
		return m.updateFilter(msg)
	case blDetail:
		return m.updateDetail(msg)
	}
	return m, nil
}

func (m blModel) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, nil

	case "j":
		next := tui.Clamp(m.cursor+1, 0, len(m.rows)-1)
		if next < len(m.rows) && m.rows[next].kind == blRowSpacer {
			next = tui.Clamp(next+1, 0, len(m.rows)-1)
		}
		m.cursor = next
		return blScrollToFit(m), nil

	case "k":
		prev := tui.Clamp(m.cursor-1, 0, len(m.rows)-1)
		if prev >= 0 && m.rows[prev].kind == blRowSpacer {
			prev = tui.Clamp(prev-1, 0, len(m.rows)-1)
		}
		m.cursor = prev
		return blScrollToFit(m), nil

	case "J", "}":
		for i := m.cursor + 1; i < len(m.rows); i++ {
			if m.rows[i].kind == blRowSprint {
				m.cursor = i
				break
			}
		}
		return blScrollToFit(m), nil

	case "K", "{":
		for i := m.cursor - 1; i >= 0; i-- {
			if m.rows[i].kind == blRowSprint {
				m.cursor = i
				break
			}
		}
		return blScrollToFit(m), nil

	case "g":
		m.cursor = 0
		m.offset = 0
		return m, nil

	case "G":
		m.cursor = len(m.rows) - 1
		return blScrollToFit(m), nil

	case "ctrl+d":
		m.cursor = tui.Clamp(m.cursor+m.viewHeight()/2, 0, len(m.rows)-1)
		if m.rows[m.cursor].kind == blRowSpacer {
			m.cursor = tui.Clamp(m.cursor+1, 0, len(m.rows)-1)
		}
		return blScrollToFit(m), nil

	case "ctrl+u":
		m.cursor = tui.Clamp(m.cursor-m.viewHeight()/2, 0, len(m.rows)-1)
		if m.rows[m.cursor].kind == blRowSpacer {
			m.cursor = tui.Clamp(m.cursor-1, 0, len(m.rows)-1)
		}
		return blScrollToFit(m), nil

	case "z":
		row := m.rows[m.cursor]
		gIdx := row.groupIdx
		m.collapsed[gIdx] = !m.collapsed[gIdx]
		m.rows = blBuildRows(m.groups, m.collapsed, m.filter)
		m.cursor = tui.Clamp(m.cursor, 0, len(m.rows)-1)
		return blScrollToFit(m), nil

	case "Z":
		anyExpanded := false
		for i := range m.groups {
			if !m.collapsed[i] {
				anyExpanded = true
				break
			}
		}
		for i := range m.groups {
			m.collapsed[i] = anyExpanded
		}
		m.rows = blBuildRows(m.groups, m.collapsed, m.filter)
		m.cursor = tui.Clamp(m.cursor, 0, len(m.rows)-1)
		return blScrollToFit(m), nil

	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.filterInput.SetValue("")
			m.rows = blBuildRows(m.groups, m.collapsed, "")
			m.cursor = tui.Clamp(m.cursor, 0, len(m.rows)-1)
		} else if m.visualMode {
			m.commitVisualSelection()
		} else if len(m.selected) > 0 {
			m.selected = make(map[string]bool)
		}
		return m, nil

	case "/":
		m.state = blFilter
		m.filterInput.SetValue(m.filter)
		return m, m.filterInput.Focus()

	case "enter":
		if m.cursor >= len(m.rows) {
			return m, nil
		}
		row := m.rows[m.cursor]
		if row.kind == blRowSpacer {
			return m, nil
		}
		if row.kind == blRowSprint {
			m.collapsed[row.groupIdx] = !m.collapsed[row.groupIdx]
			m.rows = blBuildRows(m.groups, m.collapsed, m.filter)
			m.cursor = tui.Clamp(m.cursor, 0, len(m.rows)-1)
			return blScrollToFit(m), nil
		}
		issue := m.groups[row.groupIdx].Issues[row.issueIdx]
		m.state = blLoading
		return m, tea.Batch(m.loadSpinner.Tick, fetchIssueCmd(m.client, issue.Key))

	case "e":
		if issue := m.currentIssue(); issue != nil {
			m.result = blResult{editKey: issue.Key}
			return m, nil
		}

	case "R":
		m.result = blResult{refresh: true}
		return m, nil

	case " ":
		if issue := m.currentIssue(); issue != nil {
			if m.selected[issue.Key] {
				delete(m.selected, issue.Key)
			} else {
				m.selected[issue.Key] = true
			}
		}
		return m, nil

	case "v":
		if m.visualMode {
			m.commitVisualSelection()
		} else {
			m.visualMode = true
			m.visualAnchor = m.cursor
		}
		return m, nil

	case "S":
		if issue := m.currentIssue(); issue != nil {
			if m.selected[issue.Key] {
				delete(m.selected, issue.Key)
			} else {
				m.selected[issue.Key] = true
			}
			prev := tui.Clamp(m.cursor-1, 0, len(m.rows)-1)
			if prev >= 0 && m.rows[prev].kind == blRowSpacer {
				prev = tui.Clamp(prev-1, 0, len(m.rows)-1)
			}
			m.cursor = prev
			return blScrollToFit(m), nil
		}
		return m, nil

	case "x":
		keys := m.moveKeys()
		if len(keys) == 0 {
			return m, nil
		}
		m.cutKeys = make(map[string]bool, len(keys))
		for _, k := range keys {
			m.cutKeys[k] = true
		}
		m.selected = make(map[string]bool)
		return m, nil

	case "p":
		if len(m.cutKeys) == 0 || m.cursor >= len(m.rows) {
			return m, nil
		}
		targetGroupIdx := m.rows[m.cursor].groupIdx
		target := m.groups[targetGroupIdx]
		keys := make([]string, 0, len(m.cutKeys))
		for k := range m.cutKeys {
			keys = append(keys, k)
		}
		m.moving = true
		return m, tea.Batch(m.loadSpinner.Tick, blMoveMultiCmd(m.client, keys, target.Sprint.ID, targetGroupIdx))

	case ">":
		keys := m.moveKeys()
		if len(keys) == 0 {
			return m, nil
		}
		row := m.rows[m.cursor]
		nextIdx := row.groupIdx + 1
		if nextIdx >= len(m.groups) {
			return m, nil
		}
		target := m.groups[nextIdx]
		m.moving = true
		return m, tea.Batch(m.loadSpinner.Tick, blMoveMultiCmd(m.client, keys, target.Sprint.ID, nextIdx))

	case "<":
		keys := m.moveKeys()
		if len(keys) == 0 {
			return m, nil
		}
		row := m.rows[m.cursor]
		prevIdx := row.groupIdx - 1
		if prevIdx < 0 {
			return m, nil
		}
		target := m.groups[prevIdx]
		m.moving = true
		return m, tea.Batch(m.loadSpinner.Tick, blMoveMultiCmd(m.client, keys, target.Sprint.ID, prevIdx))

	case "B":
		keys := m.moveKeys()
		if len(keys) == 0 {
			return m, nil
		}
		row := m.rows[m.cursor]
		backlogIdx := -1
		for i, g := range m.groups {
			if g.Sprint.State == "backlog" {
				backlogIdx = i
				break
			}
		}
		if backlogIdx < 0 || backlogIdx == row.groupIdx {
			return m, nil
		}
		m.moving = true
		return m, tea.Batch(m.loadSpinner.Tick, blMoveMultiCmd(m.client, keys, 0, backlogIdx))
	}

	return m, nil
}

func (m blModel) updateFilter(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.filter = ""
			m.filterInput.SetValue("")
			m.filterInput.Blur()
			m.state = blList
			m.rows = blBuildRows(m.groups, m.collapsed, "")
			m.cursor = tui.Clamp(m.cursor, 0, len(m.rows)-1)
			return m, nil
		case "enter":
			m.filter = m.filterInput.Value()
			m.filterInput.Blur()
			m.state = blList
			m.rows = blBuildRows(m.groups, m.collapsed, m.filter)
			m.cursor = tui.Clamp(m.cursor, 0, len(m.rows)-1)
			return m, nil
		case "ctrl+c":
			m.quitting = true
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.filter = m.filterInput.Value()
	m.rows = blBuildRows(m.groups, m.collapsed, m.filter)
	m.cursor = tui.Clamp(m.cursor, 0, len(m.rows)-1)
	return m, cmd
}

func (m blModel) updateDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			m.state = blList
			m.detailIssue = nil
			return m, nil
		case "e":
			if m.detailIssue != nil {
				m.result = blResult{editKey: m.detailIssue.Key}
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

func blMoveMultiCmd(client api.Client, keys []string, targetSprintID, targetGroupIdx int) tea.Cmd {
	return func() tea.Msg {
		var err error
		if targetSprintID == 0 {
			err = client.MoveIssuesToBacklog(keys)
		} else {
			err = client.MoveIssuesToSprint(targetSprintID, keys)
		}
		return blMoveMultiDoneMsg{
			movedKeys:      keys,
			targetGroupIdx: targetGroupIdx,
			err:            err,
		}
	}
}
