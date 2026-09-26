package app

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
)

func TestBlBuildRows_BasicStructure(t *testing.T) {
	groups := []models.SprintGroup{
		{
			Sprint: models.Sprint{ID: 1, Name: "Sprint 1"},
			Issues: []models.Issue{{Key: "PROJ-1", Summary: "First"}, {Key: "PROJ-2", Summary: "Second"}},
		},
		{
			Sprint: models.Sprint{ID: 2, Name: "Sprint 2"},
			Issues: []models.Issue{{Key: "PROJ-3", Summary: "Third"}},
		},
	}

	collapsed := make(map[int]bool)
	rows := blBuildRows(groups, collapsed, "", "")

	// Expected structure:
	// Row 0: Sprint 1 header
	// Row 1: PROJ-1
	// Row 2: PROJ-2
	// Row 3: Spacer
	// Row 4: Sprint 2 header
	// Row 5: PROJ-3

	expectedRows := 6
	if len(rows) != expectedRows {
		t.Fatalf("expected %d rows, got %d", expectedRows, len(rows))
	}

	// Check sprint headers
	if rows[0].kind != blRowSprint || rows[0].groupIdx != 0 {
		t.Errorf("row 0: expected Sprint 1 header, got %+v", rows[0])
	}
	if rows[4].kind != blRowSprint || rows[4].groupIdx != 1 {
		t.Errorf("row 4: expected Sprint 2 header, got %+v", rows[4])
	}

	// Check issue rows
	if rows[1].kind != blRowIssue || rows[1].issueIdx != 0 {
		t.Errorf("row 1: expected first issue, got %+v", rows[1])
	}
	if rows[2].kind != blRowIssue || rows[2].issueIdx != 1 {
		t.Errorf("row 2: expected second issue, got %+v", rows[2])
	}
	if rows[5].kind != blRowIssue || rows[5].issueIdx != 0 {
		t.Errorf("row 5: expected first issue of sprint 2, got %+v", rows[5])
	}

	// Check spacer
	if rows[3].kind != blRowSpacer {
		t.Errorf("row 3: expected spacer, got %+v", rows[3])
	}
}

func TestBlBuildRows_Collapsed(t *testing.T) {
	groups := []models.SprintGroup{
		{
			Sprint: models.Sprint{ID: 1, Name: "Sprint 1"},
			Issues: []models.Issue{{Key: "PROJ-1", Summary: "First"}},
		},
		{
			Sprint: models.Sprint{ID: 2, Name: "Sprint 2"},
			Issues: []models.Issue{{Key: "PROJ-2", Summary: "Second"}},
		},
	}

	// Collapse Sprint 1
	collapsed := map[int]bool{0: true}
	rows := blBuildRows(groups, collapsed, "", "")

	// Expected structure:
	// Row 0: Sprint 1 header
	// Row 1: Spacer
	// Row 2: Sprint 2 header
	// Row 3: PROJ-2

	expectedRows := 4
	if len(rows) != expectedRows {
		t.Fatalf("expected %d rows, got %d", expectedRows, len(rows))
	}

	// Sprint 1 should only have header (no issues)
	if rows[0].kind != blRowSprint {
		t.Errorf("row 0: expected Sprint 1 header, got %+v", rows[0])
	}

	// Check that PROJ-1 is not in rows
	for _, row := range rows {
		if row.kind == blRowIssue && row.groupIdx == 0 {
			t.Error("found issue from collapsed Sprint 1")
		}
	}

	// Sprint 2 should have header and issue
	foundSprint2Issue := false
	for _, row := range rows {
		if row.kind == blRowIssue && row.groupIdx == 1 {
			foundSprint2Issue = true
			break
		}
	}
	if !foundSprint2Issue {
		t.Error("Sprint 2 issue not found")
	}
}

func TestBlBuildRows_EmptyGroups(t *testing.T) {
	groups := []models.SprintGroup{}
	collapsed := make(map[int]bool)
	rows := blBuildRows(groups, collapsed, "", "")

	if len(rows) != 0 {
		t.Errorf("expected 0 rows for empty groups, got %d", len(rows))
	}
}

func TestBlBuildRows_WithFilter(t *testing.T) {
	groups := []models.SprintGroup{
		{
			Sprint: models.Sprint{ID: 1, Name: "Sprint 1"},
			Issues: []models.Issue{{Key: "PROJ-1", Summary: "Authentication feature"}, {Key: "PROJ-2", Summary: "Bug fix"}},
		},
	}

	collapsed := make(map[int]bool)

	// Filter by "auth"
	rows := blBuildRows(groups, collapsed, "auth", "")

	// Only PROJ-1 should match
	expectedRows := 2 // Sprint header + 1 matching issue
	if len(rows) != expectedRows {
		t.Fatalf("expected %d rows, got %d", expectedRows, len(rows))
	}

	found := false
	for _, row := range rows {
		if row.kind == blRowIssue && row.issueIdx == 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("PROJ-1 (matching 'auth') not found")
	}
}

func TestBlMatchesFilter_TextMatch(t *testing.T) {
	issue := models.Issue{
		Key:     "PROJ-123",
		Summary: "Fix authentication bug",
	}

	tests := []struct {
		filter string
		want   bool
	}{
		{"", true},
		{"proj", true},
		{"PROJ-123", true},
		{"auth", true},
		{"AUTHENTICATION", true},
		{"bug", true},
		{"nonexistent", false},
		{"xyz", false},
	}

	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			got := blMatchesFilter(issue, tt.filter, "")
			if got != tt.want {
				t.Errorf("blMatchesFilter(%q) = %v, want %v", tt.filter, got, tt.want)
			}
		})
	}
}

func TestBlMatchesFilter_EpicFilter(t *testing.T) {
	issue := models.Issue{
		Key:      "PROJ-123",
		Summary:  "Fix bug",
		EpicKey:  "PROJ-100",
		EpicName: "Authentication Epic",
	}

	tests := []struct {
		name       string
		filter     string
		filterEpic string
		want       bool
	}{
		{"no filters", "fix", "", true},
		{"matching epic key", "fix", "PROJ-100", true},
		{"matching epic name", "fix", "Authentication Epic", true},
		{"non-matching epic", "fix", "PROJ-999", false},
		{"text matches but epic doesn't", "fix", "Other Epic", false},
		{"empty filter with matching epic", "", "PROJ-100", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := blMatchesFilter(issue, tt.filter, tt.filterEpic)
			if got != tt.want {
				t.Errorf("blMatchesFilter(filter=%q, filterEpic=%q) = %v, want %v", tt.filter, tt.filterEpic, got, tt.want)
			}
		})
	}
}

func TestBlMatchesFilter_EpicFilterOnly(t *testing.T) {
	issue1 := models.Issue{
		Key:      "PROJ-1",
		Summary:  "Issue 1",
		EpicKey:  "PROJ-100",
		EpicName: "Epic A",
	}
	issue2 := models.Issue{
		Key:      "PROJ-2",
		Summary:  "Issue 2",
		EpicKey:  "PROJ-200",
		EpicName: "Epic B",
	}

	// Filter by epic key
	if !blMatchesFilter(issue1, "", "PROJ-100") {
		t.Error("issue1 should match epic filter by key")
	}
	if blMatchesFilter(issue2, "", "PROJ-100") {
		t.Error("issue2 should not match epic filter by key")
	}

	// Filter by epic name
	if !blMatchesFilter(issue1, "", "Epic A") {
		t.Error("issue1 should match epic filter by name")
	}
	if blMatchesFilter(issue2, "", "Epic A") {
		t.Error("issue2 should not match epic filter by name")
	}
}

func TestBlSetEpicFilterSelectsFirstMatchingIssue(t *testing.T) {
	groups := []models.SprintGroup{
		{
			Sprint: models.Sprint{Name: "Sprint 1"},
			Issues: []models.Issue{
				{Key: "PROJ-1", EpicKey: "EPIC-A"},
				{Key: "PROJ-2", EpicKey: "EPIC-B"},
			},
		},
		{
			Sprint: models.Sprint{Name: "Backlog"},
			Issues: []models.Issue{
				{Key: "PROJ-3", EpicKey: "EPIC-A"},
			},
		},
	}
	m := blModel{
		state:     blList,
		groups:    groups,
		rows:      blBuildRows(groups, map[int]bool{}, "", ""),
		collapsed: map[int]bool{},
		cursor:    2,
	}

	got, _ := m.setEpicFilter("EPIC-A")

	if got.filterEpic != "EPIC-A" {
		t.Fatalf("filterEpic = %q, want EPIC-A", got.filterEpic)
	}
	if got.cursor >= len(got.rows) || got.rows[got.cursor].kind != blRowIssue {
		t.Fatalf("cursor = %d, want an issue row", got.cursor)
	}
	selected := got.groups[got.rows[got.cursor].groupIdx].Issues[got.rows[got.cursor].issueIdx]
	if selected.Key != "PROJ-1" {
		t.Fatalf("selected issue = %s, want PROJ-1", selected.Key)
	}
	for _, row := range got.rows {
		if row.kind == blRowIssue {
			issue := got.groups[row.groupIdx].Issues[row.issueIdx]
			if issue.EpicKey != "EPIC-A" {
				t.Fatalf("filtered rows contain %s", issue.Key)
			}
		}
	}
}

func TestBlAssignParentRequestsFullRefresh(t *testing.T) {
	cmd := blAssignParentCmd(parentRefreshClient{}, []string{"PROJ-1"}, "EPIC-1")
	result := cmd()
	msg, ok := result.(blBulkDoneMsg)
	if !ok {
		t.Fatalf("command returned %T, want blBulkDoneMsg", result)
	}
	if !msg.FullRefresh {
		t.Fatal("parent assignment should request a full board refresh")
	}
}

type parentRefreshClient struct {
	api.Client
}

func (parentRefreshClient) BulkSetParent([]string, string) []error {
	return []error{nil}
}

// blSprintJumpGroups returns three sprints with issues so every sprint header
// has an issue row before it.
func blSprintJumpGroups() []models.SprintGroup {
	return []models.SprintGroup{
		{
			Sprint: models.Sprint{ID: 1, Name: "Sprint 1"},
			Issues: []models.Issue{{Key: "PROJ-1", Summary: "First"}, {Key: "PROJ-2", Summary: "Second"}},
		},
		{
			Sprint: models.Sprint{ID: 2, Name: "Sprint 2"},
			Issues: []models.Issue{{Key: "PROJ-3", Summary: "Third"}},
		},
		{
			Sprint: models.Sprint{ID: 3, Name: "Sprint 3"},
			Issues: []models.Issue{{Key: "PROJ-4", Summary: "Fourth"}},
		},
	}
}

// blTestModel builds a list-state model over the given groups with the cursor
// at startCursor. Rows are rebuilt from the groups, so mutate the model's copy
// only through its own Update/handler paths.
func blTestModel(groups []models.SprintGroup, startCursor int) blModel {
	collapsed := map[int]bool{}
	return blModel{
		state:     blList,
		width:     120,
		height:    40,
		groups:    groups,
		rows:      blBuildRows(groups, collapsed, "", ""),
		collapsed: collapsed,
		cursor:    startCursor,
		selected:  map[string]bool{},
		cutKeys:   map[string]bool{},
	}
}

// blIssueKeyAtCursor returns the issue key under the cursor, or "" when the
// cursor is not on an issue row.
func blIssueKeyAtCursor(m blModel) string {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return ""
	}
	row := m.rows[m.cursor]
	if row.kind != blRowIssue {
		return ""
	}
	return m.groups[row.groupIdx].Issues[row.issueIdx].Key
}

func TestBlSprintJumpKeys(t *testing.T) {
	// Rows: 0=Sprint 1, 1=PROJ-1, 2=PROJ-2, 3=spacer, 4=Sprint 2,
	//       5=PROJ-3, 6=spacer, 7=Sprint 3, 8=PROJ-4
	cases := []struct {
		name       string
		key        tea.KeyPressMsg
		startRow   int
		wantCursor int
	}{
		{"l jumps to next header", keyPress("l"), 1, 4},
		{"right jumps to next header", arrowKey(tea.KeyRight), 1, 4},
		{"J jumps to next header", keyPress("J"), 2, 4},
		{"} jumps to next header", keyPress("}"), 5, 7},
		{"l on last header is a no-op", keyPress("l"), 7, 7},
		{"right on last header is a no-op", arrowKey(tea.KeyRight), 7, 7},
		{"h jumps to previous header", keyPress("h"), 5, 4},
		{"left jumps to previous header", arrowKey(tea.KeyLeft), 8, 7},
		{"K jumps to previous header", keyPress("K"), 5, 4},
		{"{ jumps to previous header", keyPress("{"), 2, 0},
		{"h on first header is a no-op", keyPress("h"), 0, 0},
		{"left on first header is a no-op", arrowKey(tea.KeyLeft), 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := blTestModel(blSprintJumpGroups(), tc.startRow)
			got, _ := m.updateList(tc.key)
			m = got.(blModel)
			if m.cursor != tc.wantCursor {
				t.Fatalf("cursor = %d, want %d", m.cursor, tc.wantCursor)
			}
			if m.rows[m.cursor].kind != blRowSprint {
				t.Fatalf("cursor row kind = %v, want a sprint header", m.rows[m.cursor].kind)
			}
		})
	}
}

func TestBlMoveDoneNoFollow(t *testing.T) {
	t.Run("adjacent sprint move keeps the cursor row", func(t *testing.T) {
		m := blTestModel(blSprintJumpGroups()[:2], 1) // cursor on PROJ-1
		if got := blIssueKeyAtCursor(m); got != "PROJ-1" {
			t.Fatalf("setup: cursor issue = %q, want PROJ-1", got)
		}

		updated, _ := m.Update(blMoveMultiDoneMsg{
			movedKeys:      []string{"PROJ-1"},
			firstMovedKey:  "PROJ-1",
			targetGroupIdx: 1,
			followCursor:   false,
		})
		got := updated.(blModel)

		if got.moving {
			t.Error("moving = true, want false after a completed move")
		}
		if got.cursor != 1 {
			t.Errorf("cursor = %d, want 1 (unchanged row index)", got.cursor)
		}
		if got.rows[got.cursor].kind == blRowSpacer {
			t.Error("cursor landed on a spacer row")
		}
		if key := blIssueKeyAtCursor(got); key != "PROJ-2" {
			t.Errorf("cursor issue = %q, want PROJ-2", key)
		}
		if !blGroupHasIssue(got.groups[1], "PROJ-1") {
			t.Error("target group does not contain PROJ-1")
		}
		if blGroupHasIssue(got.groups[0], "PROJ-1") {
			t.Error("source group still contains PROJ-1")
		}
	})

	t.Run("follow still lands on the moved issue", func(t *testing.T) {
		m := blTestModel(blSprintJumpGroups()[:2], 1)

		updated, _ := m.Update(blMoveMultiDoneMsg{
			movedKeys:      []string{"PROJ-1"},
			firstMovedKey:  "PROJ-1",
			targetGroupIdx: 1,
			followCursor:   true,
		})
		got := updated.(blModel)

		if key := blIssueKeyAtCursor(got); key != "PROJ-1" {
			t.Errorf("cursor issue = %q, want PROJ-1", key)
		}
		if got.rows[got.cursor].groupIdx != 1 {
			t.Errorf("cursor group = %d, want 1", got.rows[got.cursor].groupIdx)
		}
	})

	t.Run("cursor steps past a spacer row", func(t *testing.T) {
		groups := []models.SprintGroup{
			{
				Sprint: models.Sprint{ID: 1, Name: "Sprint 1"},
				Issues: []models.Issue{{Key: "PROJ-1"}, {Key: "PROJ-2"}},
			},
			{Sprint: models.Sprint{ID: 2, Name: "Sprint 2"}},
			{
				Sprint: models.Sprint{ID: 3, Name: "Sprint 3"},
				Issues: []models.Issue{{Key: "PROJ-3"}},
			},
		}
		// Rows: 0=Sprint 1, 1=PROJ-1, 2=PROJ-2, 3=spacer, 4=Sprint 2, ...
		m := blTestModel(groups, 2) // cursor on PROJ-2

		updated, _ := m.Update(blMoveMultiDoneMsg{
			movedKeys:      []string{"PROJ-1"},
			firstMovedKey:  "PROJ-1",
			targetGroupIdx: 1,
			followCursor:   false,
		})
		got := updated.(blModel)

		// PROJ-1 removed from Sprint 1 shifts the rows, so row 2 becomes the
		// spacer between Sprint 1 and Sprint 2.
		if got.cursor != 3 {
			t.Fatalf("cursor = %d, want 3 (stepped past the spacer)", got.cursor)
		}
		if got.rows[got.cursor].kind != blRowSprint {
			t.Errorf("cursor row kind = %v, want a sprint header", got.rows[got.cursor].kind)
		}
	})
}

func blGroupHasIssue(group models.SprintGroup, key string) bool {
	for _, issue := range group.Issues {
		if issue.Key == key {
			return true
		}
	}
	return false
}

// --- sprint target picker ('m') ---

// blSprintPickerTestClient records move/rank calls so the picker tests can
// assert that a confirmed target actually reached the API (or did not).
type blSprintPickerTestClient struct {
	api.Client
	moves    [][]string
	sprints  []int
	ranks    int
	rankArgs []string
}

func (c *blSprintPickerTestClient) MoveIssuesToSprint(sprintID int, keys []string) error {
	c.sprints = append(c.sprints, sprintID)
	c.moves = append(c.moves, append([]string(nil), keys...))
	return nil
}

func (c *blSprintPickerTestClient) MoveIssuesToBacklog(keys []string) error {
	c.sprints = append(c.sprints, 0)
	c.moves = append(c.moves, append([]string(nil), keys...))
	return nil
}

func (c *blSprintPickerTestClient) RankIssues(keys []string, rankAfterKey, rankBeforeKey string) error {
	c.ranks++
	c.rankArgs = append([]string(nil), keys...)
	return nil
}

// blCollectMsgs runs a command and flattens every tea.BatchMsg it produces, so
// a test can inspect the terminal messages a batched command would deliver.
func blCollectMsgs(cmd tea.Cmd) []tea.Msg {
	var out []tea.Msg
	var walk func(c tea.Cmd)
	walk = func(c tea.Cmd) {
		if c == nil {
			return
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			for _, sub := range msg {
				walk(sub)
			}
		default:
			out = append(out, msg)
		}
	}
	walk(cmd)
	return out
}

// blSprintPickerGroups returns two sprints plus a backlog group, with the
// cursor at row 1 (PROJ-1 in group 0) by default in the tests below.
func blSprintPickerGroups() []models.SprintGroup {
	return []models.SprintGroup{
		{
			Sprint: models.Sprint{
				ID: 10, Name: "Sprint 1", State: "active",
				StartDate: "2026-03-01", EndDate: "2026-03-14",
			},
			Issues: []models.Issue{{Key: "PROJ-1", Summary: "First"}, {Key: "PROJ-2", Summary: "Second"}},
		},
		{
			Sprint: models.Sprint{ID: 20, Name: "Sprint 2", State: "future"},
			Issues: []models.Issue{{Key: "PROJ-3", Summary: "Third"}},
		},
		{
			Sprint: models.Sprint{Name: "Backlog", State: "backlog"},
			Issues: []models.Issue{{Key: "PROJ-4", Summary: "Fourth"}},
		},
	}
}

func TestBlSprintPickerOpensWithOneItemPerGroup(t *testing.T) {
	m := blTestModel(blSprintPickerGroups(), 1) // cursor on PROJ-1

	got, cmd := m.Update(keyPress("m"))
	m = got.(blModel)

	if m.state != blSprintPicker {
		t.Fatalf("state = %v, want blSprintPicker", m.state)
	}
	if cmd == nil {
		t.Fatal("opening the picker should focus its input")
	}
	if want := []string{"PROJ-1"}; !reflect.DeepEqual(m.sprintTargetKeys, want) {
		t.Fatalf("sprintTargetKeys = %v, want %v", m.sprintTargetKeys, want)
	}

	wantLabels := []string{"Sprint 1", "Sprint 2", "Backlog"}
	if len(m.sprintPicker.Items) != len(wantLabels) {
		t.Fatalf("picker items = %d, want %d", len(m.sprintPicker.Items), len(wantLabels))
	}
	for i, want := range wantLabels {
		item := m.sprintPicker.Items[i]
		if item.Label != want {
			t.Errorf("item %d label = %q, want %q", i, item.Label, want)
		}
		if item.Value != strconv.Itoa(i) {
			t.Errorf("item %d value = %q, want %q", i, item.Value, strconv.Itoa(i))
		}
	}
	if sub := m.sprintPicker.Items[2].SubLabel; sub != "backlog" {
		t.Errorf("backlog sub-label = %q, want %q", sub, "backlog")
	}
	if sub := m.sprintPicker.Items[0].SubLabel; sub != "active · Mar 1 – Mar 14" {
		t.Errorf("active sprint sub-label = %q, want dates appended", sub)
	}
}

func TestBlSprintPickerNoOpOnSprintHeader(t *testing.T) {
	m := blTestModel(blSprintPickerGroups(), 0) // sprint header, no selection

	got, cmd := m.Update(keyPress("m"))
	m = got.(blModel)

	if m.state != blList {
		t.Fatalf("state = %v, want blList", m.state)
	}
	if cmd != nil {
		t.Error("pressing m with nothing selected should not return a command")
	}
	if len(m.sprintTargetKeys) != 0 {
		t.Errorf("sprintTargetKeys = %v, want empty", m.sprintTargetKeys)
	}
}

func TestBlSprintPickerConfirmMovesToTargetGroup(t *testing.T) {
	client := &blSprintPickerTestClient{}
	m := blTestModel(blSprintPickerGroups(), 1)
	m.client = client

	got, _ := m.Update(keyPress("m"))
	m = got.(blModel)
	// Highlight group 1 (Sprint 2, ID 20).
	got, _ = m.Update(arrowKey(tea.KeyDown))
	m = got.(blModel)

	got, cmd := m.Update(keyPress("enter"))
	m = got.(blModel)

	if m.state != blList {
		t.Fatalf("state = %v, want blList after confirming", m.state)
	}
	if len(m.sprintTargetKeys) != 0 {
		t.Errorf("sprintTargetKeys = %v, want cleared", m.sprintTargetKeys)
	}
	if cmd == nil {
		t.Fatal("confirming a target should return a move command")
	}

	var done *blMoveMultiDoneMsg
	for _, msg := range blCollectMsgs(cmd) {
		if d, ok := msg.(blMoveMultiDoneMsg); ok {
			done = &d
			break
		}
	}
	if done == nil {
		t.Fatal("move command did not yield a blMoveMultiDoneMsg")
	}
	if done.targetGroupIdx != 1 {
		t.Errorf("targetGroupIdx = %d, want 1", done.targetGroupIdx)
	}
	if done.followCursor {
		t.Error("followCursor = true, want false so the cursor keeps its row")
	}
	if done.err != nil {
		t.Errorf("err = %v, want nil", done.err)
	}
	if !reflect.DeepEqual(done.movedKeys, []string{"PROJ-1"}) {
		t.Errorf("movedKeys = %v, want [PROJ-1]", done.movedKeys)
	}
	if len(client.moves) != 1 || client.sprints[0] != 20 || !reflect.DeepEqual(client.moves[0], []string{"PROJ-1"}) {
		t.Errorf("client moves = %v (sprints %v), want one move to sprint 20", client.moves, client.sprints)
	}
	if client.ranks != 1 {
		t.Errorf("rank calls = %d, want 1 (rankAfterKey is non-empty)", client.ranks)
	}
}

func TestBlSprintPickerConfirmSameGroupIsNoOp(t *testing.T) {
	client := &blSprintPickerTestClient{}
	m := blTestModel(blSprintPickerGroups(), 1)
	m.client = client

	got, _ := m.Update(keyPress("m"))
	m = got.(blModel)
	// Cursor stays on group 0, which already holds PROJ-1.
	got, cmd := m.Update(keyPress("enter"))
	m = got.(blModel)

	if m.state != blList {
		t.Fatalf("state = %v, want blList", m.state)
	}
	if cmd != nil {
		t.Error("an all-placed target should return a nil command")
	}
	if len(client.moves) != 0 || client.ranks != 0 {
		t.Errorf("client calls = %d moves / %d ranks, want none", len(client.moves), client.ranks)
	}
}

func TestBlSprintPickerOverlayFitsTerminal(t *testing.T) {
	m := blTestModel(blSprintPickerGroups(), 1)
	got, _ := m.Update(keyPress("m"))
	m = got.(blModel)

	out := m.viewSprintPicker()
	// RenderPickerModal normalizes runs of whitespace via SanitizeRow, so the
	// double space in the title renders as a single one.
	for _, want := range []string{"Move to Sprint (1 issue)", "Sprint 1", "Sprint 2", "Backlog"} {
		if !strings.Contains(out, want) {
			t.Errorf("overlay missing %q:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if w := tui.DisplayWidth(line); w > m.width {
			t.Errorf("overlay line exceeds terminal width: %d > %d: %q", w, m.width, line)
		}
	}
}

func TestBlSprintPickerEscCancels(t *testing.T) {
	client := &blSprintPickerTestClient{}
	m := blTestModel(blSprintPickerGroups(), 1)
	m.client = client

	got, _ := m.Update(keyPress("m"))
	m = got.(blModel)
	got, cmd := m.Update(keyPress("esc"))
	m = got.(blModel)

	if m.state != blList {
		t.Fatalf("state = %v, want blList after cancelling", m.state)
	}
	if len(m.sprintTargetKeys) != 0 {
		t.Errorf("sprintTargetKeys = %v, want cleared", m.sprintTargetKeys)
	}
	if cmd != nil {
		t.Error("cancelling should not return a command")
	}
	if len(client.moves) != 0 {
		t.Errorf("client moves = %v, want none", client.moves)
	}
}
