package app

import (
	"errors"
	"testing"

	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/models"
)

func TestLinkedItemsForIssueIncludesLinksSubTasksAndParent(t *testing.T) {
	issue := &models.Issue{
		Key: "PROJ-1",
		LinkedIssues: []models.LinkedIssue{
			{Relationship: "blocks", Key: "PROJ-2", Summary: "Blocked"},
		},
		SubTasks: []models.LinkedIssue{
			{Relationship: "subtask", Key: "PROJ-3", Summary: "Sub"},
		},
		ParentKey:     "EPIC-1",
		ParentSummary: "Parent epic",
	}

	items := linkedItemsForIssue(issue)
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
	if items[0].Relationship != "blocks" || items[0].Key != "PROJ-2" {
		t.Errorf("items[0] = %+v, want the explicit link", items[0])
	}
	if items[1].Relationship != "subtask" || items[1].Key != "PROJ-3" {
		t.Errorf("items[1] = %+v, want the subtask", items[1])
	}
	if items[2].Relationship != "parent" || items[2].Key != "EPIC-1" || items[2].Summary != "Parent epic" {
		t.Errorf("items[2] = %+v, want the parent", items[2])
	}
}

func TestLinkedItemsForIssueNilAndParentless(t *testing.T) {
	if items := linkedItemsForIssue(nil); items != nil {
		t.Errorf("items = %v, want nil", items)
	}
	if items := linkedItemsForIssue(&models.Issue{Key: "PROJ-1"}); len(items) != 0 {
		t.Errorf("items = %v, want none", items)
	}
}

func TestCollectLinkedItemsKeepsFirstEntryPerKey(t *testing.T) {
	links := []models.LinkedIssue{{Relationship: "blocks", Key: "PROJ-2", Summary: "Link"}}
	children := []models.LinkedIssue{
		{Relationship: "child", Key: "PROJ-2", Summary: "Duplicate"},
		{Relationship: "child", Key: "PROJ-4"},
		{Relationship: "child", Key: ""},
	}

	items := collectLinkedItems(links, children)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].Summary != "Link" {
		t.Errorf("items[0] = %+v, want the first entry for a duplicate key", items[0])
	}
	if items[1].Key != "PROJ-4" {
		t.Errorf("items[1] = %+v, want PROJ-4", items[1])
	}
}

func TestLinkedPickerRowsMapItemsToLabelAndSubLabel(t *testing.T) {
	rows := linkedPickerRows([]models.LinkedIssue{
		{
			Relationship: "blocks",
			Key:          "PROJ-2",
			Summary:      "Fix login",
			IssueType:    "Bug",
			Status:       "In Progress",
		},
		{Key: "PROJ-3", Summary: "No attributes"},
	})

	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].Label != "blocks PROJ-2" {
		t.Errorf("label = %q, want %q", rows[0].Label, "blocks PROJ-2")
	}
	if rows[0].SubLabel != "Fix login (Bug · In Progress)" {
		t.Errorf("sub-label = %q", rows[0].SubLabel)
	}
	if rows[0].Value != "PROJ-2" {
		t.Errorf("value = %q, want the issue key", rows[0].Value)
	}
	if rows[1].Label != "PROJ-3" || rows[1].SubLabel != "No attributes" {
		t.Errorf("row = %+v, want the key and summary without attributes", rows[1])
	}
}

func TestLinkedItemSubLabelIncludesSubTaskCount(t *testing.T) {
	subLabel := linkedItemSubLabel(models.LinkedIssue{Summary: "Child", SubTaskCount: 2})
	if subLabel != "Child (2 subtasks)" {
		t.Errorf("sub-label = %q", subLabel)
	}
}

func newBacklogLinkPickerModel(issue *models.Issue) blModel {
	groups := []models.SprintGroup{
		{
			Sprint: models.Sprint{Name: "Sprint 1"},
			Issues: []models.Issue{*issue},
		},
	}
	return blModel{
		state:            blList,
		groups:           groups,
		rows:             blBuildRows(groups, map[int]bool{}, "", ""),
		collapsed:        map[int]bool{},
		cursor:           1,
		jiraURL:          "https://example.atlassian.net",
		sidebarFullIssue: issue,
		sidebarIssueKey:  issue.Key,
		width:            100,
		height:           40,
	}
}

func TestBacklogLinkPickerListsRelatedItems(t *testing.T) {
	issue := &models.Issue{
		Key:     "PROJ-1",
		Summary: "Ticket",
		LinkedIssues: []models.LinkedIssue{
			{Relationship: "blocks", Key: "PROJ-2", Summary: "Blocked", IssueType: "Bug", Status: "To Do"},
		},
		SubTasks: []models.LinkedIssue{
			{Relationship: "subtask", Key: "PROJ-3", Summary: "Sub"},
		},
	}
	m := newBacklogLinkPickerModel(issue)

	updated, cmd := m.Update(keyPress("L"))
	m = updated.(blModel)
	if m.state != blLinkPicker {
		t.Fatalf("state = %v, want blLinkPicker", m.state)
	}
	if cmd == nil {
		t.Error("opening the picker should focus the filter input")
	}
	if m.linkPickerReturn != blList {
		t.Errorf("return state = %v, want blList", m.linkPickerReturn)
	}
	if len(m.linkPicker.items) != 2 {
		t.Fatalf("items = %d, want 2", len(m.linkPicker.items))
	}
	if m.linkPicker.selectedKey() != "PROJ-2" {
		t.Errorf("first selection = %q, want PROJ-2", m.linkPicker.selectedKey())
	}

	updated, _ = m.Update(keyPress("down"))
	m = updated.(blModel)
	if m.linkPicker.selectedKey() != "PROJ-3" {
		t.Fatalf("selection after down = %q, want PROJ-3", m.linkPicker.selectedKey())
	}
	if url := m.issueURL(m.linkPicker.selectedKey()); url != "https://example.atlassian.net/browse/PROJ-3" {
		t.Errorf("issue URL = %q", url)
	}

	updated, cmd = m.Update(keyPress("enter"))
	m = updated.(blModel)
	if m.state != blList {
		t.Fatalf("state after select = %v, want blList", m.state)
	}
	if cmd == nil {
		t.Error("selecting an item should return a browser command")
	}
}

func TestBacklogLinkPickerOpensFromDetailAndReturnsToDetail(t *testing.T) {
	issue := &models.Issue{
		Key:          "PROJ-1",
		Summary:      "Ticket",
		LinkedIssues: []models.LinkedIssue{{Relationship: "blocks", Key: "PROJ-2"}},
	}
	m := newBacklogLinkPickerModel(issue)
	m.state = blDetail
	m.detailIssue = issue

	updated, _ := m.Update(keyPress("L"))
	m = updated.(blModel)
	if m.state != blLinkPicker {
		t.Fatalf("state = %v, want blLinkPicker", m.state)
	}
	if m.linkPickerReturn != blDetail {
		t.Fatalf("return state = %v, want blDetail", m.linkPickerReturn)
	}

	updated, _ = m.Update(keyPress("esc"))
	m = updated.(blModel)
	if m.state != blDetail {
		t.Fatalf("state after cancel = %v, want blDetail", m.state)
	}
}

func TestBacklogLinkPickerWithoutFullIssueIsNotFatal(t *testing.T) {
	issue := &models.Issue{Key: "PROJ-1", Summary: "Ticket"}
	m := newBacklogLinkPickerModel(issue)
	m.sidebarFullIssue = nil

	updated, cmd := m.Update(keyPress("L"))
	m = updated.(blModel)
	if m.state != blLinkPicker {
		t.Fatalf("state = %v, want blLinkPicker", m.state)
	}
	if len(m.linkPicker.items) != 0 {
		t.Fatalf("items = %v, want none before the full issue loads", m.linkPicker.items)
	}
	if cmd == nil {
		t.Error("opening the picker should focus the filter input")
	}

	updated, cmd = m.Update(keyPress("enter"))
	m = updated.(blModel)
	if m.state != blList {
		t.Fatalf("state after empty select = %v, want blList", m.state)
	}
	if cmd != nil {
		t.Error("selecting nothing should not open a browser")
	}
}

func TestBacklogLinkPickerQuitsOnCtrlC(t *testing.T) {
	issue := &models.Issue{Key: "PROJ-1", Summary: "Ticket"}
	m := newBacklogLinkPickerModel(issue)

	updated, _ := m.Update(keyPress("L"))
	m = updated.(blModel)
	updated, _ = m.Update(keyPress("ctrl+c"))
	m = updated.(blModel)
	if !m.quitting {
		t.Fatal("ctrl+c in the picker should quit")
	}
}

func TestBacklogLinkPickerPicksUpItemsWhenSidebarFetchLands(t *testing.T) {
	issue := &models.Issue{Key: "PROJ-1", Summary: "Ticket"}
	m := newBacklogLinkPickerModel(issue)
	m.sidebarFullIssue = nil

	updated, _ := m.Update(keyPress("L"))
	m = updated.(blModel)
	if len(m.linkPicker.items) != 0 {
		t.Fatalf("items = %v, want none before the sidebar fetch", m.linkPicker.items)
	}

	full := &models.Issue{
		Key:          "PROJ-1",
		Summary:      "Ticket",
		LinkedIssues: []models.LinkedIssue{{Relationship: "blocks", Key: "PROJ-2", Summary: "Blocked"}},
	}
	updated, _ = m.Update(sidebarIssueFetchedMsg{issue: full})
	m = updated.(blModel)

	if m.state != blLinkPicker {
		t.Fatalf("state = %v, want blLinkPicker still open", m.state)
	}
	if len(m.linkPicker.items) != 1 {
		t.Fatalf("items = %v, want the newly fetched link", m.linkPicker.items)
	}
	if m.linkPicker.selectedKey() != "PROJ-2" {
		t.Errorf("selection = %q, want PROJ-2", m.linkPicker.selectedKey())
	}
}

func TestBacklogLinkPickerKeepsHighlightWhenItemsRefresh(t *testing.T) {
	picker := linkedItemPicker{}
	picker.open([]models.LinkedIssue{{Key: "PROJ-1"}, {Key: "PROJ-2"}})
	picker.picker.Cursor = 1

	picker.reopen([]models.LinkedIssue{{Key: "PROJ-3"}, {Key: "PROJ-4"}, {Key: "PROJ-5"}})
	if picker.selectedKey() != "PROJ-4" {
		t.Errorf("selection = %q, want the same row (PROJ-4)", picker.selectedKey())
	}

	picker.reopen(nil)
	if picker.selectedKey() != "" {
		t.Errorf("selection = %q, want empty for an empty picker", picker.selectedKey())
	}
}

type kanbanLinkTestClient struct {
	api.Client
	issue *models.Issue
	err   error
	calls int
}

func (c *kanbanLinkTestClient) GetIssue(string) (*models.Issue, error) {
	c.calls++
	return c.issue, c.err
}

func newKanbanLinkTestModel(client api.Client) kanbanModel {
	return kanbanModel{
		state:      stateBoard,
		client:     client,
		jiraURL:    "https://example.atlassian.net",
		columns:    []kanbanColumn{{name: "To Do", issues: []models.Issue{{Key: "PROJ-1", Summary: "Ticket"}}}},
		rowIdxs:    []int{0},
		colScrolls: []int{0},
		width:      100,
		height:     40,
	}
}

// filterLinkedPicker feeds the given text into the picker and returns the
// resulting selected key.
func filterLinkedPicker(t *testing.T, m blModel, query string) blModel {
	t.Helper()
	for _, r := range query {
		updated, _ := m.Update(keyPress(string(r)))
		m = updated.(blModel)
	}
	return m
}

func TestBacklogLinkPickerFiltersAsYouType(t *testing.T) {
	issue := &models.Issue{
		Key:     "PROJ-1",
		Summary: "Ticket",
		LinkedIssues: []models.LinkedIssue{
			{Relationship: "blocks", Key: "PROJ-2", Summary: "Fix login", IssueType: "Bug"},
			{Relationship: "relates to", Key: "PROJ-3", Summary: "Add signup", IssueType: "Story"},
		},
	}
	m := newBacklogLinkPickerModel(issue)

	updated, _ := m.Update(keyPress("L"))
	m = updated.(blModel)
	if len(m.linkPicker.picker.Items) != 2 {
		t.Fatalf("rows = %d, want both items before filtering", len(m.linkPicker.picker.Items))
	}

	// Typing narrows the list; the query matches the summary, not the key.
	m = filterLinkedPicker(t, m, "signup")
	if len(m.linkPicker.picker.Items) != 1 {
		t.Fatalf("rows = %d, want 1 after filtering", len(m.linkPicker.picker.Items))
	}
	if m.linkPicker.selectedKey() != "PROJ-3" {
		t.Fatalf("selection = %q, want PROJ-3", m.linkPicker.selectedKey())
	}

	// Arrow keys still navigate and confirm the filtered selection.
	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(blModel)
	if m.state != blList {
		t.Fatalf("state after select = %v, want blList", m.state)
	}
	if cmd == nil {
		t.Fatal("selecting a filtered item should return a browser command")
	}
}

func TestBacklogLinkPickerArrowKeysNavigateAndClearFilter(t *testing.T) {
	issue := &models.Issue{
		Key:          "PROJ-1",
		Summary:      "Ticket",
		LinkedIssues: []models.LinkedIssue{{Relationship: "blocks", Key: "PROJ-2"}, {Relationship: "relates to", Key: "PROJ-3"}},
	}
	m := newBacklogLinkPickerModel(issue)

	updated, _ := m.Update(keyPress("L"))
	m = updated.(blModel)

	updated, _ = m.Update(keyPress("down"))
	m = updated.(blModel)
	if m.linkPicker.selectedKey() != "PROJ-3" {
		t.Fatalf("selection after down = %q, want PROJ-3", m.linkPicker.selectedKey())
	}

	// A query that matches nothing leaves nothing to select.
	m = filterLinkedPicker(t, m, "zzz")
	if m.linkPicker.selectedKey() != "" {
		t.Fatalf("selection = %q, want empty when the filter excludes everything", m.linkPicker.selectedKey())
	}

	// Backspacing restores the full list.
	for range 3 {
		updated, _ = m.Update(keyPress("backspace"))
		m = updated.(blModel)
	}
	if len(m.linkPicker.picker.Items) != 2 {
		t.Fatalf("rows = %d, want the full list after clearing the query", len(m.linkPicker.picker.Items))
	}
}

func TestKanbanLinkPickerFetchesFullIssueFromBoard(t *testing.T) {
	full := &models.Issue{
		Key:          "PROJ-1",
		Summary:      "Ticket",
		LinkedIssues: []models.LinkedIssue{{Relationship: "blocks", Key: "PROJ-2", Summary: "Blocked"}},
	}
	client := &kanbanLinkTestClient{issue: full}
	m := newKanbanLinkTestModel(client)

	updated, cmd := m.Update(keyPress("L"))
	m = updated.(kanbanModel)
	if cmd == nil {
		t.Fatal("opening the picker from the board should fetch the full issue")
	}
	if m.linkPickerKey != "PROJ-1" {
		t.Fatalf("pending key = %q, want PROJ-1", m.linkPickerKey)
	}
	if m.state != stateBoard {
		t.Fatalf("state = %v, want the board until the fetch lands", m.state)
	}

	// The returned command is a tea.Batch whose fetch message the runtime
	// delivers individually, so the fetch is invoked directly here.
	updated, _ = m.Update(fetchKanbanLinkIssueCmd(client, "PROJ-1")())
	m = updated.(kanbanModel)
	if m.state != stateLinkPicker {
		t.Fatalf("state = %v, want stateLinkPicker", m.state)
	}
	if m.linkPickerKey != "" {
		t.Errorf("pending key = %q, want cleared", m.linkPickerKey)
	}
	if m.linkPickerReturn != stateBoard {
		t.Errorf("return state = %v, want stateBoard", m.linkPickerReturn)
	}
	if len(m.linkPicker.items) != 1 || m.linkPicker.selectedKey() != "PROJ-2" {
		t.Fatalf("items = %+v, want the linked issue", m.linkPicker.items)
	}
}

func TestKanbanLinkPickerFromDetailUsesLoadedIssue(t *testing.T) {
	full := &models.Issue{
		Key:          "PROJ-1",
		Summary:      "Ticket",
		LinkedIssues: []models.LinkedIssue{{Relationship: "blocks", Key: "PROJ-2"}},
		SubTasks:     []models.LinkedIssue{{Relationship: "subtask", Key: "PROJ-3"}},
	}
	client := &kanbanLinkTestClient{issue: full}
	m := newKanbanLinkTestModel(client)
	m.state = stateDetail
	m.detailIssue = full

	updated, cmd := m.Update(keyPress("L"))
	m = updated.(kanbanModel)
	if m.state != stateLinkPicker {
		t.Fatalf("state = %v, want stateLinkPicker", m.state)
	}
	if client.calls != 0 {
		t.Errorf("GetIssue calls = %d, want 0: the detail pane already holds the full issue", client.calls)
	}
	if cmd == nil {
		t.Error("opening the picker should focus the filter input")
	}
	if len(m.linkPicker.items) != 2 {
		t.Fatalf("items = %+v, want the link and the subtask", m.linkPicker.items)
	}

	updated, _ = m.Update(keyPress("esc"))
	m = updated.(kanbanModel)
	if m.state != stateDetail {
		t.Fatalf("state after cancel = %v, want stateDetail", m.state)
	}
}

func TestKanbanLinkPickerIgnoresStaleFetchAndOpensEmptyOnFailure(t *testing.T) {
	m := newKanbanLinkTestModel(&kanbanLinkTestClient{})
	m.state = stateBoard
	m.linkPickerKey = "PROJ-2"

	updated, _ := m.Update(kanbanLinkIssueFetchedMsg{key: "PROJ-1", issue: &models.Issue{Key: "PROJ-1"}})
	m = updated.(kanbanModel)
	if m.state != stateBoard {
		t.Fatalf("state = %v, want the board for a stale fetch", m.state)
	}

	client := &kanbanLinkTestClient{err: errors.New("boom")}
	m = newKanbanLinkTestModel(client)
	updated, _ = m.Update(keyPress("L"))
	m = updated.(kanbanModel)
	updated, _ = m.Update(fetchKanbanLinkIssueCmd(client, "PROJ-1")())
	m = updated.(kanbanModel)
	if m.state != stateLinkPicker {
		t.Fatalf("state = %v, want an empty picker so the key press is visible", m.state)
	}
	if len(m.linkPicker.items) != 0 {
		t.Errorf("items = %+v, want none", m.linkPicker.items)
	}
}

func TestKanbanLinkPickerOpensSelectionInBrowser(t *testing.T) {
	full := &models.Issue{
		Key:          "PROJ-1",
		Summary:      "Ticket",
		LinkedIssues: []models.LinkedIssue{{Relationship: "blocks", Key: "PROJ-2"}},
	}
	m := newKanbanLinkTestModel(&kanbanLinkTestClient{issue: full})
	m.state = stateDetail
	m.detailIssue = full
	m.jiraURL = "https://example.atlassian.net"

	updated, _ := m.Update(keyPress("L"))
	m = updated.(kanbanModel)
	selectedKey := m.linkPicker.selectedKey()
	if selectedKey != "PROJ-2" {
		t.Fatalf("selection = %q, want PROJ-2", selectedKey)
	}

	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(kanbanModel)
	if m.state != stateDetail {
		t.Fatalf("state after select = %v, want stateDetail", m.state)
	}
	if cmd == nil {
		t.Fatal("selecting a link should return a browser command")
	}
	if url := m.issueURL(selectedKey); url != "https://example.atlassian.net/browse/PROJ-2" {
		t.Errorf("issue URL = %q", url)
	}
}

func TestLinkPickerSuppressesBoardActions(t *testing.T) {
	tests := []struct {
		name  string
		board boardModel
	}{
		{
			name:  "backlog",
			board: boardModel{activeView: ViewBacklog, backlog: blModel{state: blLinkPicker}},
		},
		{
			name:  "kanban",
			board: boardModel{activeView: ViewKanban, kanban: kanbanModel{state: stateLinkPicker}},
		},
		{
			name:  "epics",
			board: boardModel{activeView: ViewEpics, epics: epicModel{state: epicLinkPicker}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.board.canOpenInBrowser() {
				t.Error("o should not open an issue while the picker is open")
			}
			if tt.board.canSwitchView() {
				t.Error("view switching should be suppressed while the picker is open")
			}
		})
	}
}
