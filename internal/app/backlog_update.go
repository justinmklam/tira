package app

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
)

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
		var cmd tea.Cmd
		m, cmd = m.updateSidebarContent()
		return blScrollToFit(m), cmd

	case "k":
		prev := tui.Clamp(m.cursor-1, 0, len(m.rows)-1)
		if prev >= 0 && m.rows[prev].kind == blRowSpacer {
			prev = tui.Clamp(prev-1, 0, len(m.rows)-1)
		}
		m.cursor = prev
		var cmd tea.Cmd
		m, cmd = m.updateSidebarContent()
		return blScrollToFit(m), cmd

	case "J", "}":
		for i := m.cursor + 1; i < len(m.rows); i++ {
			if m.rows[i].kind == blRowSprint {
				m.cursor = i
				break
			}
		}
		var cmd tea.Cmd
		m, cmd = m.updateSidebarContent()
		return blScrollToFit(m), cmd

	case "K", "{":
		for i := m.cursor - 1; i >= 0; i-- {
			if m.rows[i].kind == blRowSprint {
				m.cursor = i
				break
			}
		}
		var cmd tea.Cmd
		m, cmd = m.updateSidebarContent()
		return blScrollToFit(m), cmd

	case "g":
		m.cursor = 0
		m.offset = 0
		var cmd tea.Cmd
		m, cmd = m.updateSidebarContent()
		return m, cmd

	case "G":
		m.cursor = len(m.rows) - 1
		var cmd tea.Cmd
		m, cmd = m.updateSidebarContent()
		return blScrollToFit(m), cmd

	case "ctrl+d":
		// Scroll sidebar content down by 1/4 page
		m.sidebarOffset += m.viewHeight() / 4
		totalLines := strings.Count(m.sidebarContent, "\n") + 1
		if maxOffset := totalLines - m.viewHeight(); m.sidebarOffset > maxOffset {
			if maxOffset < 0 {
				m.sidebarOffset = 0
			} else {
				m.sidebarOffset = maxOffset
			}
		}
		return m, nil

	case "ctrl+u":
		// Scroll sidebar content up by 1/4 page
		m.sidebarOffset -= m.viewHeight() / 4
		if m.sidebarOffset < 0 {
			m.sidebarOffset = 0
		}
		return m, nil

	case "z":
		row := m.rows[m.cursor]
		gIdx := row.groupIdx
		m.collapsed[gIdx] = !m.collapsed[gIdx]
		m.rows = blBuildRows(m.groups, m.collapsed, m.filter, m.filterEpic)
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
		m.rows = blBuildRows(m.groups, m.collapsed, m.filter, m.filterEpic)
		m.cursor = tui.Clamp(m.cursor, 0, len(m.rows)-1)
		return blScrollToFit(m), nil

	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.filterInput.SetValue("")
			m.rows = blBuildRows(m.groups, m.collapsed, "", m.filterEpic)
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

	case "f":
		m.state = blKeySearch
		m.keySearchInput.SetValue("")
		return m, m.keySearchInput.Focus()

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
			m.rows = blBuildRows(m.groups, m.collapsed, m.filter, m.filterEpic)
			m.cursor = tui.Clamp(m.cursor, 0, len(m.rows)-1)
			return blScrollToFit(m), nil
		}
		issue := m.groups[row.groupIdx].Issues[row.issueIdx]
		m.state = blLoading
		vpW, _ := tui.OverlayViewportSize(m.width, m.height)
		return m, tea.Batch(m.loadSpinner.Tick, fetchIssueCmd(m.client, issue.Key, vpW))

	case "e":
		if issue := m.currentIssue(); issue != nil {
			m.result = blResult{editKey: issue.Key}
			return m, nil
		}

	case "o":
		if issue := m.currentIssue(); issue != nil {
			return m, openInBrowserCmd(m.issueURL(issue.Key))
		}

	case "O":
		keys := m.moveKeys()
		if len(keys) == 0 {
			return m, nil
		}
		for _, key := range keys {
			_ = openInBrowserCmd(m.issueURL(key))()
		}
		return m, nil

	case "y":
		if issue := m.currentIssue(); issue != nil {
			return m, copyToClipboardCmd(m.issueURL(issue.Key))
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
		keys := m.moveKeys()
		if len(keys) == 0 {
			return m, nil
		}
		m.storyPointTargetKeys = keys
		m.storyPointInput.SetValue("")
		m.state = blStoryPointInput
		return m, m.storyPointInput.Focus()

	case "s":
		keys := m.moveKeys()
		if len(keys) == 0 {
			return m, nil
		}
		m.statusTargetKeys = keys
		// Get current status from the first selected issue for initial cursor position.
		var currentStatus string
		if issue := m.currentIssue(); issue != nil {
			currentStatus = issue.Status
		}
		m.statusPicker = blNewStatusPicker(m.client, keys[0], currentStatus)
		m.state = blStatusPicker
		return m, m.statusPicker.Init()

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

	case "a":
		if m.cursor < len(m.rows) {
			row := m.rows[m.cursor]
			g := m.groups[row.groupIdx]
			m.result = blResult{create: true, createSprintID: g.Sprint.ID, createGroupIdx: row.groupIdx}
		}
		return m, nil

	case "C":
		for i, g := range m.groups {
			if g.Sprint.State == "backlog" {
				m.result = blResult{create: true, createSprintID: 0, createGroupIdx: i}
				return m, nil
			}
		}
		// No dedicated backlog group; create in backlog (sprint 0).
		m.result = blResult{create: true, createSprintID: 0, createGroupIdx: 0}
		return m, nil

	case "P":
		keys := m.moveKeys()
		if len(keys) == 0 {
			return m, nil
		}
		projectKey := m.project
		if projectKey == "" && len(keys) > 0 {
			if idx := strings.Index(keys[0], "-"); idx > 0 {
				projectKey = keys[0][:idx]
			}
		}
		m.parentTargetKeys = keys
		m.parentPicker = blNewParentPicker(m.client, projectKey)
		m.state = blParentPicker
		return m, m.parentPicker.Init()

	case "A":
		keys := m.moveKeys()
		if len(keys) == 0 {
			return m, nil
		}
		projectKey := m.project
		if projectKey == "" && len(keys) > 0 {
			if idx := strings.Index(keys[0], "-"); idx > 0 {
				projectKey = keys[0][:idx]
			}
		}
		m.assignTargetKeys = keys
		m.assignPicker = newAssigneePicker(m.client, projectKey)
		m.state = blAssignPicker
		return m, m.assignPicker.Init()

	case "F":
		projectKey := m.project
		if projectKey == "" {
			if issue := m.currentIssue(); issue != nil {
				if idx := strings.Index(issue.Key, "-"); idx > 0 {
					projectKey = issue.Key[:idx]
				}
			}
		}
		m.epicFilterPicker = blNewEpicFilterPicker(m.client, projectKey, m.filterEpic)
		m.state = blEpicFilterPicker
		return m, m.epicFilterPicker.Init()

	case "c":
		if issue := m.currentIssue(); issue != nil {
			m.result = blResult{commentKey: issue.Key, commentSummary: issue.Summary}
			return m, nil
		}

	case "ctrl+n":
		m = m.openSprintCreateForm()
		return m, m.sprintFormName.Focus()

	case "E":
		if m.cursor < len(m.rows) && m.rows[m.cursor].kind == blRowSprint {
			sprint := m.groups[m.rows[m.cursor].groupIdx].Sprint
			if sprint.State != "backlog" && sprint.ID != 0 {
				m = m.openSprintEditForm(sprint)
				return m, m.sprintFormName.Focus()
			}
		}
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
			m.rows = blBuildRows(m.groups, m.collapsed, "", m.filterEpic)
			m.cursor = tui.Clamp(m.cursor, 0, len(m.rows)-1)
			return m, nil
		case "enter":
			m.filter = m.filterInput.Value()
			m.filterInput.Blur()
			m.state = blList
			m.rows = blBuildRows(m.groups, m.collapsed, m.filter, m.filterEpic)
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
	m.rows = blBuildRows(m.groups, m.collapsed, m.filter, m.filterEpic)
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
		case "c":
			if m.detailIssue != nil {
				m.result = blResult{commentKey: m.detailIssue.Key, commentSummary: m.detailIssue.Summary}
				return m, nil
			}
		case "R":
			m.result = blResult{refresh: true}
			return m, nil
		case "ctrl+c":
			m.quitting = true
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.detailView, cmd = m.detailView.Update(msg)
	return m, cmd
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
	m.rows = blBuildRows(m.groups, m.collapsed, m.filter, m.filterEpic)
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
	m.rows = blBuildRows(m.groups, m.collapsed, m.filter, m.filterEpic)
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

func (m blModel) updateParentPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Let the app quit even from inside the picker.
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		m.quitting = true
		return m, nil
	}

	updated, cmd := m.parentPicker.Update(msg)
	m.parentPicker = updated

	if m.parentPicker.Aborted {
		m.state = blList
		m.parentTargetKeys = nil
		return m, nil
	}
	if m.parentPicker.Completed {
		item := m.parentPicker.SelectedItem()
		var parentKey string
		if item != nil {
			parentKey = item.Value
		}
		keys := m.parentTargetKeys
		m.state = blList
		m.parentTargetKeys = nil
		return m, blAssignParentCmd(m.client, keys, parentKey)
	}
	return m, cmd
}

// blNewParentPicker creates a PickerModel whose search function queries the
// Jira API for epics matching the typed query.
func blNewParentPicker(client api.Client, projectKey string) tui.PickerModel {
	search := func(query string) ([]tui.PickerItem, error) {
		epics, err := client.GetEpics(projectKey, query)
		if err != nil {
			return nil, err
		}
		items := make([]tui.PickerItem, len(epics))
		for i, e := range epics {
			items[i] = tui.PickerItem{Label: e.Key, SubLabel: e.Summary, Value: e.Key}
		}
		return items, nil
	}
	m := tui.NewPickerModel(search)
	m.NoneItem = &tui.PickerItem{Label: "(none)", SubLabel: "clear parent"}
	return m
}

// blNewEpicFilterPicker creates a PickerModel for filtering issues by epic.
func blNewEpicFilterPicker(client api.Client, projectKey, currentFilter string) tui.PickerModel {
	search := func(query string) ([]tui.PickerItem, error) {
		epics, err := client.GetEpics(projectKey, query)
		if err != nil {
			return nil, err
		}
		items := make([]tui.PickerItem, len(epics))
		for i, e := range epics {
			items[i] = tui.PickerItem{Label: e.Key, SubLabel: e.Summary, Value: e.Key}
		}
		return items, nil
	}
	m := tui.NewPickerModel(search)
	m.NoneItem = &tui.PickerItem{Label: "(none)", SubLabel: "clear epic filter"}
	return m
}

func (m blModel) updateEpicFilterPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		m.quitting = true
		return m, nil
	}

	updated, cmd := m.epicFilterPicker.Update(msg)
	m.epicFilterPicker = updated

	if m.epicFilterPicker.Aborted {
		m.state = blList
		return m, nil
	}
	if m.epicFilterPicker.Completed {
		item := m.epicFilterPicker.SelectedItem()
		if item != nil {
			m.filterEpic = item.Value
		} else {
			m.filterEpic = ""
		}
		m.state = blList
		m.rows = blBuildRows(m.groups, m.collapsed, m.filter, m.filterEpic)
		m.cursor = tui.Clamp(m.cursor, 0, len(m.rows)-1)
		return blScrollToFit(m), nil
	}
	return m, cmd
}

func (m blModel) updateAssignPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		m.quitting = true
		return m, nil
	}
	updated, cmd := m.assignPicker.Update(msg)
	m.assignPicker = updated
	if m.assignPicker.Aborted {
		m.state = blList
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
		m.state = blList
		m.assignTargetKeys = nil
		return m, blDoAssignCmd(m.client, keys, accountID)
	}
	return m, cmd
}

func blDoAssignCmd(client api.Client, keys []string, accountID string) tea.Cmd {
	return func() tea.Msg {
		errors := client.BulkSetAssignee(keys, accountID)
		return blBulkDoneMsg{Keys: keys, Errors: errors}
	}
}

func blAssignParentCmd(client api.Client, keys []string, parentKey string) tea.Cmd {
	return func() tea.Msg {
		errors := client.BulkSetParent(keys, parentKey)
		return blBulkDoneMsg{Keys: keys, Errors: errors}
	}
}

func (m blModel) updateStoryPointInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		m.quitting = true
		return m, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.state = blList
			m.storyPointTargetKeys = nil
			return m, nil

		case "enter":
			val := m.storyPointInput.Value()
			var sp float64
			if val != "" {
				var err error
				sp, err = parseFloat(val)
				if err != nil {
					m.storyPointInput.Placeholder = "invalid number, try again"
					return m, nil
				}
				if sp < 0 {
					m.storyPointInput.Placeholder = "must be >= 0"
					return m, nil
				}
			}
			keys := m.storyPointTargetKeys
			m.state = blList
			m.storyPointTargetKeys = nil
			return m, blSetStoryPointCmd(m.client, keys, sp)
		}
	}

	var cmd tea.Cmd
	m.storyPointInput, cmd = m.storyPointInput.Update(msg)
	return m, cmd
}

func blSetStoryPointCmd(client api.Client, keys []string, storyPoints float64) tea.Cmd {
	return func() tea.Msg {
		fields := models.IssueFields{StoryPoints: storyPoints}
		errors := client.BulkUpdateIssue(keys, fields)
		return blBulkDoneMsg{Keys: keys, Errors: errors}
	}
}

func (m blModel) updateStatusPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		m.quitting = true
		return m, nil
	}
	updated, cmd := m.statusPicker.Update(msg)
	m.statusPicker = updated
	if m.statusPicker.Aborted {
		m.state = blList
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
		m.state = blList
		m.statusTargetKeys = nil
		return m, blTransitionStatusCmd(m.client, keys, transitionID)
	}
	return m, cmd
}

// blNewStatusPicker creates a PickerModel whose search function returns
// available status transitions for the given issue.
// If currentStatus is non-empty, the picker will initially select the item
// whose label matches the current status.
func blNewStatusPicker(client api.Client, issueKey, currentStatus string) tui.PickerModel {
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

func blTransitionStatusCmd(client api.Client, keys []string, transitionID string) tea.Cmd {
	return func() tea.Msg {
		errors := client.BulkTransitionStatus(keys, transitionID)
		// FullRefresh: GetIssue does not return StatusID, so a targeted patch
		// would leave the kanban column placement stale after a status change.
		return blBulkDoneMsg{Keys: keys, Errors: errors, FullRefresh: true}
	}
}

// openSprintCreateForm initialises the sprint form for creating a new sprint.
func (m blModel) openSprintCreateForm() blModel {
	today := time.Now().Format("2006-01-02")

	nameInput := textinput.New()
	nameInput.Placeholder = "sprint name"
	nameInput.CharLimit = 100
	nameInput.Width = 40
	nameInput.SetValue(nextSprintName(m.groups))

	startInput := textinput.New()
	startInput.Placeholder = "YYYY-MM-DD"
	startInput.CharLimit = 10
	startInput.Width = 12
	startInput.SetValue(today)

	durInput := textinput.New()
	durInput.Placeholder = "weeks"
	durInput.CharLimit = 3
	durInput.Width = 5
	durInput.SetValue("2")

	m.sprintFormName = nameInput
	m.sprintFormStart = startInput
	m.sprintFormDuration = durInput
	m.sprintFormField = 0
	m.sprintFormEditID = 0
	m.sprintFormError = ""
	m.sprintFormSubmitting = false
	m.state = blSprintForm
	return m
}

// openSprintEditForm initialises the sprint form pre-populated with existing sprint data.
func (m blModel) openSprintEditForm(sprint models.Sprint) blModel {
	startDate := sprint.StartDate
	if len(startDate) > 10 {
		startDate = startDate[:10]
	}
	weeks := sprintDurationWeeks(sprint.StartDate, sprint.EndDate)

	nameInput := textinput.New()
	nameInput.Placeholder = "sprint name"
	nameInput.CharLimit = 100
	nameInput.Width = 40
	nameInput.SetValue(sprint.Name)

	startInput := textinput.New()
	startInput.Placeholder = "YYYY-MM-DD"
	startInput.CharLimit = 10
	startInput.Width = 12
	startInput.SetValue(startDate)

	durInput := textinput.New()
	durInput.Placeholder = "weeks"
	durInput.CharLimit = 3
	durInput.Width = 5
	durInput.SetValue(strconv.Itoa(weeks))

	m.sprintFormName = nameInput
	m.sprintFormStart = startInput
	m.sprintFormDuration = durInput
	m.sprintFormField = 0
	m.sprintFormEditID = sprint.ID
	m.sprintFormError = ""
	m.sprintFormSubmitting = false
	m.state = blSprintForm
	return m
}

// focusSprintField blurs all form inputs then focuses the given field index.
func (m blModel) focusSprintField(field int) (tea.Model, tea.Cmd) {
	m.sprintFormField = field
	m.sprintFormName.Blur()
	m.sprintFormStart.Blur()
	m.sprintFormDuration.Blur()
	var cmd tea.Cmd
	switch field {
	case 0:
		cmd = m.sprintFormName.Focus()
	case 1:
		cmd = m.sprintFormStart.Focus()
	case 2:
		cmd = m.sprintFormDuration.Focus()
	}
	return m, cmd
}

// validateSprintForm returns a human-readable error string, or "" if the form is valid.
func (m blModel) validateSprintForm() string {
	if strings.TrimSpace(m.sprintFormName.Value()) == "" {
		return "sprint name is required"
	}
	start := strings.TrimSpace(m.sprintFormStart.Value())
	if _, err := time.Parse("2006-01-02", start); err != nil {
		return "start date must be in YYYY-MM-DD format"
	}
	dur := strings.TrimSpace(m.sprintFormDuration.Value())
	n, err := strconv.Atoi(dur)
	if err != nil || n < 1 {
		return "duration must be a positive number of weeks"
	}
	return ""
}

func (m blModel) updateSprintForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		// While submitting, only allow quitting.
		if m.sprintFormSubmitting {
			if key.String() == "ctrl+c" {
				m.quitting = true
			}
			return m, nil
		}

		switch key.String() {
		case "ctrl+c":
			m.quitting = true
			return m, nil

		case "esc":
			m.state = blList
			return m, nil

		case "tab":
			return m.focusSprintField((m.sprintFormField + 1) % 3)

		case "shift+tab":
			return m.focusSprintField((m.sprintFormField + 2) % 3)

		case "ctrl+s":
			if errMsg := m.validateSprintForm(); errMsg != "" {
				m.sprintFormError = errMsg
				return m, nil
			}
			m.sprintFormError = ""
			m.sprintFormSubmitting = true
			// Blur inputs during submission.
			m.sprintFormName.Blur()
			m.sprintFormStart.Blur()
			m.sprintFormDuration.Blur()

			name := strings.TrimSpace(m.sprintFormName.Value())
			start := strings.TrimSpace(m.sprintFormStart.Value())
			dur, _ := strconv.Atoi(strings.TrimSpace(m.sprintFormDuration.Value()))
			end := computeEndDate(start, dur)

			if m.sprintFormEditID == 0 {
				return m, tea.Batch(m.loadSpinner.Tick, blCreateSprintCmd(m.client, m.boardID, name, start, end))
			}
			return m, tea.Batch(m.loadSpinner.Tick, blUpdateSprintCmd(m.client, m.sprintFormEditID, name, start, end))
		}
	}

	// Forward message to the active text input.
	var cmd tea.Cmd
	switch m.sprintFormField {
	case 0:
		m.sprintFormName, cmd = m.sprintFormName.Update(msg)
	case 1:
		m.sprintFormStart, cmd = m.sprintFormStart.Update(msg)
	case 2:
		m.sprintFormDuration, cmd = m.sprintFormDuration.Update(msg)
	}
	// Clear error when the user types.
	if _, ok := msg.(tea.KeyMsg); ok {
		m.sprintFormError = ""
	}
	return m, cmd
}

func blCreateSprintCmd(client api.Client, boardID int, name, startDate, endDate string) tea.Cmd {
	return func() tea.Msg {
		sprint, err := client.CreateSprint(boardID, name, startDate, endDate)
		return blSprintDoneMsg{created: sprint, err: err}
	}
}

func blUpdateSprintCmd(client api.Client, sprintID int, name, startDate, endDate string) tea.Cmd {
	return func() tea.Msg {
		err := client.UpdateSprint(sprintID, name, startDate, endDate)
		return blSprintDoneMsg{err: err}
	}
}

// updateSidebarContent updates the sidebar content based on the currently selected issue.
// If the cursor is on a sprint header, it uses the last selected issue.
// It triggers an async fetch of the full issue (with description) from the Jira API.
func (m blModel) updateSidebarContent() (blModel, tea.Cmd) {
	issue := m.currentIssue()
	// Track last issue for when cursor is on sprint header
	if issue != nil {
		m.lastIssue = issue
	} else if m.lastIssue != nil {
		// Cursor is on sprint header or spacer, use last issue
		issue = m.lastIssue
	}

	// If we already have the full issue cached, use it
	if issue != nil && m.sidebarFullIssue != nil && m.sidebarIssueKey == issue.Key {
		m.sidebarContent = renderSidebarContent(m.sidebarFullIssue, tui.DetailPaneWidth(m.width))
		m.sidebarOffset = 0
		return m, nil
	}

	// If the issue key changed, trigger a fetch
	if issue != nil && m.sidebarIssueKey != issue.Key {
		m.sidebarIssueKey = issue.Key
		m.sidebarFullIssue = nil
		// Show basic issue content while fetching
		m.sidebarContent = renderSidebarContent(issue, tui.DetailPaneWidth(m.width))
		m.sidebarOffset = 0
		return m, fetchSidebarIssueCmd(m.client, issue.Key)
	}

	// No issue selected
	m.sidebarContent = renderSidebarContent(nil, tui.DetailPaneWidth(m.width))
	m.sidebarOffset = 0
	return m, nil
}

func (m blModel) updateKeySearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.keySearchInput.Blur()
			m.state = blList
			return m, nil
		case "enter":
			query := strings.TrimSpace(m.keySearchInput.Value())
			m.keySearchInput.Blur()
			m.state = blList
			if query != "" {
				for i, row := range m.rows {
					if row.kind == blRowIssue {
						issue := m.groups[row.groupIdx].Issues[row.issueIdx]
						keyNum := issue.Key
						if idx := strings.LastIndex(issue.Key, "-"); idx >= 0 {
							keyNum = issue.Key[idx+1:]
						}
						if strings.HasPrefix(keyNum, query) {
							m.cursor = i
							m = blScrollToFit(m)
							var cmd tea.Cmd
							m, cmd = m.updateSidebarContent()
							return m, cmd
						}
					}
				}
			}
			return m, nil
		case "ctrl+c":
			m.quitting = true
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.keySearchInput, cmd = m.keySearchInput.Update(msg)
	return m, cmd
}
