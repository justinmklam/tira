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

type blRankDoneMsg struct{ err error }

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
// otherwise just the current issue. Keys are returned in display order.
func (m blModel) moveKeys() []string {
	if combined := m.allSelected(); len(combined) > 0 {
		return m.keysInDisplayOrder(combined)
	}
	if issue := m.currentIssue(); issue != nil {
		return []string{issue.Key}
	}
	return nil
}

// lastIssueKey returns the key of the last issue in the group that is NOT in
// excludeSet (used to find the rank-after anchor for bottom-of-sprint placement).
func lastIssueKey(issues []models.Issue, excludeSet map[string]bool) string {
	for i := len(issues) - 1; i >= 0; i-- {
		if !excludeSet[issues[i].Key] {
			return issues[i].Key
		}
	}
	return ""
}

// keysInDisplayOrder returns the subset of keys that appear in the given set,
// ordered by their position in the current groups/issues list.
func (m blModel) keysInDisplayOrder(keySet map[string]bool) []string {
	keys := make([]string, 0, len(keySet))
	for _, g := range m.groups {
		for _, issue := range g.Issues {
			if keySet[issue.Key] {
				keys = append(keys, issue.Key)
			}
		}
	}
	return keys
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
			vpW, vpH := tui.OverlayViewportSize(m.width, m.height)
			m.detailView.Width = vpW
			m.detailView.Height = vpH
		}
		return m, nil

	case issueFetchedMsg:
		if msg.err != nil {
			m.state = blList
			return m, nil
		}
		m.detailIssue = msg.issue
		vpW, vpH := tui.OverlayViewportSize(m.width, m.height)
		md, _ := display.RenderIssue(msg.issue)
		renderer, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(vpW),
		)
		content := md
		if err == nil {
			if rendered, err2 := renderer.Render(md); err2 == nil {
				content = rendered
			}
		}
		vp := viewport.New(vpW, vpH)
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
			// Remove moved issues from all groups, append to target.
			var movedIssues []models.Issue
			for i := range m.groups {
				var remaining []models.Issue
				for _, issue := range m.groups[i].Issues {
					if movedSet[issue.Key] {
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

	case blRankDoneMsg:
		// Rank API failures are non-fatal; local state is already updated.
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
		m.visualMode = false
		return m, nil

	case "p":
		if len(m.cutKeys) == 0 || m.cursor >= len(m.rows) {
			return m, nil
		}
		targetGroupIdx := m.rows[m.cursor].groupIdx
		target := m.groups[targetGroupIdx]
		keys := m.keysInDisplayOrder(m.cutKeys)
		rankAfter := lastIssueKey(target.Issues, m.cutKeys)
		m.moving = true
		return m, tea.Batch(m.loadSpinner.Tick, blMoveMultiCmd(m.client, keys, target.Sprint.ID, targetGroupIdx, rankAfter))

	case "ctrl+j":
		return m.moveSelectionDown()

	case "ctrl+k":
		return m.moveSelectionUp()

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
		rankAfter := lastIssueKey(target.Issues, make(map[string]bool))
		m.moving = true
		return m, tea.Batch(m.loadSpinner.Tick, blMoveMultiCmd(m.client, keys, target.Sprint.ID, nextIdx, rankAfter))

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
		rankAfter := lastIssueKey(target.Issues, make(map[string]bool))
		m.moving = true
		return m, tea.Batch(m.loadSpinner.Tick, blMoveMultiCmd(m.client, keys, target.Sprint.ID, prevIdx, rankAfter))

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
		return m, tea.Batch(m.loadSpinner.Tick, blMoveMultiCmd(m.client, keys, 0, backlogIdx, ""))
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

// blMoveMultiCmd moves keys to a sprint (or backlog) and, when rankAfterKey is
// non-empty, explicitly ranks them after that issue so they land at the bottom.
func blMoveMultiCmd(client api.Client, keys []string, targetSprintID, targetGroupIdx int, rankAfterKey string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if targetSprintID == 0 {
			err = client.MoveIssuesToBacklog(keys)
		} else {
			err = client.MoveIssuesToSprint(targetSprintID, keys)
			if err == nil && rankAfterKey != "" {
				err = client.RankIssues(keys, rankAfterKey, "")
			}
		}
		return blMoveMultiDoneMsg{movedKeys: keys, targetGroupIdx: targetGroupIdx, err: err}
	}
}

func blRankCmd(client api.Client, keys []string, rankAfterKey, rankBeforeKey string) tea.Cmd {
	return func() tea.Msg {
		return blRankDoneMsg{err: client.RankIssues(keys, rankAfterKey, rankBeforeKey)}
	}
}

// effectiveMoveSet returns the set and ordered keys to move with ctrl+j/ctrl+k.
// If an active selection exists and all selected issues are in groupIdx, returns those.
// Otherwise returns just the cursor issue. Returns nil if no valid issues found.
func (m blModel) effectiveMoveSet(groupIdx int) (map[string]bool, []string) {
	selected := m.allSelected()
	if len(selected) > 0 {
		// All selected issues must be within the same group.
		for k := range selected {
			found := false
			for _, issue := range m.groups[groupIdx].Issues {
				if issue.Key == k {
					found = true
					break
				}
			}
			if !found {
				return nil, nil
			}
		}
		keys := blOrderedKeys(m.groups[groupIdx].Issues, selected)
		return selected, keys
	}
	if m.cursor < len(m.rows) {
		row := m.rows[m.cursor]
		if row.kind == blRowIssue && row.groupIdx == groupIdx {
			key := m.groups[groupIdx].Issues[row.issueIdx].Key
			return map[string]bool{key: true}, []string{key}
		}
	}
	return nil, nil
}

// findIssueRow returns the row index of the issue with the given key in groupIdx.
func (m blModel) findIssueRow(groupIdx int, key string) int {
	for i, row := range m.rows {
		if row.kind == blRowIssue && row.groupIdx == groupIdx &&
			m.groups[groupIdx].Issues[row.issueIdx].Key == key {
			return i
		}
	}
	return tui.Clamp(m.cursor, 0, len(m.rows)-1)
}

// moveSelectionDown moves the effective selection one step down within the sprint,
// or to the top of the next sprint if already at the bottom.
func (m blModel) moveSelectionDown() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.rows) {
		return m, nil
	}
	row := m.rows[m.cursor]
	if row.kind != blRowIssue {
		return m, nil
	}
	groupIdx := row.groupIdx
	cursorKey := m.groups[groupIdx].Issues[row.issueIdx].Key

	if m.visualMode {
		m.commitVisualSelection()
	}
	moveSet, moveKeys := m.effectiveMoveSet(groupIdx)
	if len(moveKeys) == 0 {
		return m, nil
	}

	issues := m.groups[groupIdx].Issues
	maxIdx := blMaxSelectedIdx(issues, moveSet)

	// Find first non-selected issue after the last selected issue.
	blockerIdx := -1
	for i := maxIdx + 1; i < len(issues); i++ {
		if !moveSet[issues[i].Key] {
			blockerIdx = i
			break
		}
	}
	if blockerIdx == -1 {
		return m, nil // already at bottom of sprint
	}

	blockerKey := issues[blockerIdx].Key
	m.groups[groupIdx].Issues = blReorderDown(issues, moveSet, blockerKey)
	m.rows = blBuildRows(m.groups, m.collapsed, m.filter)
	m.cursor = m.findIssueRow(groupIdx, cursorKey)
	return blScrollToFit(m), blRankCmd(m.client, moveKeys, blockerKey, "")
}

// moveSelectionUp moves the effective selection one step up within the sprint,
// or to the bottom of the previous sprint if already at the top.
func (m blModel) moveSelectionUp() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.rows) {
		return m, nil
	}
	row := m.rows[m.cursor]
	if row.kind != blRowIssue {
		return m, nil
	}
	groupIdx := row.groupIdx
	cursorKey := m.groups[groupIdx].Issues[row.issueIdx].Key

	if m.visualMode {
		m.commitVisualSelection()
	}
	moveSet, moveKeys := m.effectiveMoveSet(groupIdx)
	if len(moveKeys) == 0 {
		return m, nil
	}

	issues := m.groups[groupIdx].Issues
	minIdx := blMinSelectedIdx(issues, moveSet)

	// Find last non-selected issue before the first selected issue.
	blockerIdx := -1
	for i := minIdx - 1; i >= 0; i-- {
		if !moveSet[issues[i].Key] {
			blockerIdx = i
			break
		}
	}
	if blockerIdx == -1 {
		return m, nil // already at top of sprint
	}

	blockerKey := issues[blockerIdx].Key
	m.groups[groupIdx].Issues = blReorderUp(issues, moveSet, blockerKey)
	m.rows = blBuildRows(m.groups, m.collapsed, m.filter)
	m.cursor = m.findIssueRow(groupIdx, cursorKey)
	return blScrollToFit(m), blRankCmd(m.client, moveKeys, "", blockerKey)
}

// blOrderedKeys returns the keys in moveSet in the order they appear in issues.
func blOrderedKeys(issues []models.Issue, moveSet map[string]bool) []string {
	keys := make([]string, 0, len(moveSet))
	for _, issue := range issues {
		if moveSet[issue.Key] {
			keys = append(keys, issue.Key)
		}
	}
	return keys
}

// blMaxSelectedIdx returns the highest index in issues that is in moveSet.
func blMaxSelectedIdx(issues []models.Issue, moveSet map[string]bool) int {
	idx := -1
	for i, issue := range issues {
		if moveSet[issue.Key] {
			idx = i
		}
	}
	return idx
}

// blMinSelectedIdx returns the lowest index in issues that is in moveSet.
func blMinSelectedIdx(issues []models.Issue, moveSet map[string]bool) int {
	for i, issue := range issues {
		if moveSet[issue.Key] {
			return i
		}
	}
	return -1
}

// blReorderDown moves all issues in moveSet to after the blocker issue.
func blReorderDown(issues []models.Issue, moveSet map[string]bool, blockerKey string) []models.Issue {
	var moved, rest []models.Issue
	for _, issue := range issues {
		if moveSet[issue.Key] {
			moved = append(moved, issue)
		} else {
			rest = append(rest, issue)
		}
	}
	for i, issue := range rest {
		if issue.Key == blockerKey {
			result := make([]models.Issue, 0, len(issues))
			result = append(result, rest[:i+1]...)
			result = append(result, moved...)
			result = append(result, rest[i+1:]...)
			return result
		}
	}
	return issues
}

// blReorderUp moves all issues in moveSet to before the blocker issue.
func blReorderUp(issues []models.Issue, moveSet map[string]bool, blockerKey string) []models.Issue {
	var moved, rest []models.Issue
	for _, issue := range issues {
		if moveSet[issue.Key] {
			moved = append(moved, issue)
		} else {
			rest = append(rest, issue)
		}
	}
	for i, issue := range rest {
		if issue.Key == blockerKey {
			result := make([]models.Issue, 0, len(issues))
			result = append(result, rest[:i]...)
			result = append(result, moved...)
			result = append(result, rest[i:]...)
			return result
		}
	}
	return issues
}
