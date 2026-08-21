package app

import (
	"errors"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/models"
)

func TestBuildEpicItemsOrderingDeduplicationAndCounts(t *testing.T) {
	groups := []models.SprintGroup{
		{
			Sprint: models.Sprint{Name: "Sprint 1", State: "active"},
			Issues: []models.Issue{
				{Key: "P-1", EpicKey: "EPIC-B", EpicName: "Beta", StoryPoints: 2},
				{Key: "P-2", EpicKey: "EPIC-A", EpicName: "Alpha", StoryPoints: 3},
			},
		},
		{
			Sprint: models.Sprint{Name: "Backlog", State: "backlog"},
			Issues: []models.Issue{
				{Key: "P-3", EpicKey: "EPIC-B", EpicName: "Beta", StoryPoints: 5},
				{Key: "P-4", EpicKey: "EPIC-A", EpicName: "Alpha", StoryPoints: 1},
				{Key: "P-5"},
			},
		},
	}

	items := buildEpicItems(groups)
	if len(items) != 2 {
		t.Fatalf("expected 2 epics, got %d", len(items))
	}
	if items[0].Key != "EPIC-B" || items[1].Key != "EPIC-A" {
		t.Fatalf("unexpected order: %#v", items)
	}
	if items[0].ChildCount != 2 || items[0].StoryPoints != 7 {
		t.Fatalf("unexpected EPIC-B aggregation: %#v", items[0])
	}
	if items[0].FirstLocation != "Sprint 1" || items[0].FirstIssueKey != "P-1" {
		t.Fatalf("unexpected first location: %#v", items[0])
	}
}

func TestBuildEpicItemsExcludesClosedEpics(t *testing.T) {
	groups := []models.SprintGroup{
		{
			Sprint: models.Sprint{Name: "Active sprint", State: "active"},
			Issues: []models.Issue{
				{Key: "P-1", EpicKey: "EPIC-CLOSED", EpicName: "Closed epic", EpicStatus: "Closed"},
				{Key: "P-2", EpicKey: "EPIC-OPEN", EpicName: "Open epic", EpicStatus: "In Progress"},
			},
		},
	}

	items := buildEpicItems(groups)
	if len(items) != 1 {
		t.Fatalf("expected one non-closed epic, got %d", len(items))
	}
	if items[0].Key != "EPIC-OPEN" {
		t.Fatalf("unexpected epic: %#v", items[0])
	}
}

func TestBuildEpicItemsFallbackNameAndEmptyInput(t *testing.T) {
	items := buildEpicItems([]models.SprintGroup{{
		Sprint: models.Sprint{State: "backlog"},
		Issues: []models.Issue{
			{Key: "P-1", EpicKey: "EPIC-X"},
			{Key: "P-2", EpicKey: "EPIC-X", EpicName: "Named Epic"},
		},
	}})
	if len(items) != 1 {
		t.Fatalf("expected one epic, got %d", len(items))
	}
	if items[0].Name != "Named Epic" {
		t.Fatalf("expected later epic name to replace fallback, got %q", items[0].Name)
	}
	if len(buildEpicItems(nil)) != 0 {
		t.Fatal("expected empty input to produce no epics")
	}
}

func TestBuildEpicItemsAssignsDistinctSprintIndexes(t *testing.T) {
	groups := []models.SprintGroup{
		{Sprint: models.Sprint{Name: "Sprint 1"}, Issues: []models.Issue{{Key: "P-1", EpicKey: "EPIC-A"}}},
		{Sprint: models.Sprint{Name: "Sprint 2"}, Issues: []models.Issue{{Key: "P-2", EpicKey: "EPIC-B"}}},
		{Sprint: models.Sprint{Name: "Sprint 3"}, Issues: []models.Issue{{Key: "P-3", EpicKey: "EPIC-C"}}},
		{Sprint: models.Sprint{Name: "Backlog", State: "backlog"}, Issues: []models.Issue{{Key: "P-4", EpicKey: "EPIC-D"}}},
	}

	items := buildEpicItems(groups)
	for i, item := range items {
		if item.FirstSprintIndex != i {
			t.Errorf("epic %s sprint index = %d, want %d", item.Key, item.FirstSprintIndex, i)
		}
	}
}

func TestEpicRefreshPreservesSelectionAndClampsCursor(t *testing.T) {
	m := epicModel{items: buildEpicItems([]models.SprintGroup{{
		Sprint: models.Sprint{Name: "Sprint 1"},
		Issues: []models.Issue{{Key: "P-1", EpicKey: "EPIC-A", EpicName: "A"}},
	}}), cursor: 0, sidebarIssueKey: "EPIC-A"}
	m.refreshData([]models.SprintGroup{
		{Sprint: models.Sprint{Name: "Sprint 2"}, Issues: []models.Issue{
			{Key: "P-2", EpicKey: "EPIC-B", EpicName: "B"},
			{Key: "P-3", EpicKey: "EPIC-A", EpicName: "A"},
		}},
	}, false, nil)
	if m.selectedKey() != "EPIC-A" {
		t.Fatalf("selection was not preserved: %q", m.selectedKey())
	}

	m.cursor = 99
	m.refreshData(nil, false, nil)
	if m.cursor != 0 || m.selectedKey() != "" {
		t.Fatalf("empty projection was not cursor-safe: cursor=%d key=%q", m.cursor, m.selectedKey())
	}
}

func TestEpicDetailLoadsAndReturnsToList(t *testing.T) {
	m := epicModel{
		state:  epicList,
		client: parentRefreshClient{},
		items: buildEpicItems([]models.SprintGroup{{
			Sprint: models.Sprint{Name: "Sprint 1"},
			Issues: []models.Issue{{Key: "P-1", EpicKey: "EPIC-A", EpicName: "Alpha"}},
		}}),
		width:  100,
		height: 40,
	}

	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(epicModel)
	if cmd == nil {
		t.Fatal("opening epic detail should return a fetch command")
	}
	if m.state != epicLoading {
		t.Fatalf("state after enter = %v, want epicLoading", m.state)
	}

	updated, _ = m.Update(issueFetchedMsg{
		issue:   &models.Issue{Key: "EPIC-A", Summary: "Alpha"},
		content: "Epic details",
	})
	m = updated.(epicModel)
	if m.state != epicDetail {
		t.Fatalf("state after fetch = %v, want epicDetail", m.state)
	}

	updated, _ = m.Update(keyPress("q"))
	m = updated.(epicModel)
	if m.state != epicList || m.detailIssue != nil {
		t.Fatalf("state after close = %v, detail issue = %#v", m.state, m.detailIssue)
	}
}

func TestEpicListUsesAlignedColumnsAndFormatting(t *testing.T) {
	const width = 100
	m := epicModel{
		items: []epicItem{{
			Key:              "EPIC-A",
			Name:             "Alpha epic",
			ChildCount:       3,
			StoryPoints:      8,
			FirstLocation:    "Sprint 1",
			FirstSprintState: "active",
		}},
		cursor: 0,
	}

	header := epicColumnHeader(width)
	for _, label := range []string{"KEY", "SUMMARY", "FIRST APPEARS", "SP", "CHILDREN"} {
		if !strings.Contains(header, label) {
			t.Errorf("column header missing %q: %q", label, header)
		}
	}
	if got := lipgloss.Width(header); got != width {
		t.Errorf("header width = %d, want %d", got, width)
	}

	row := m.renderRow(0, width)
	if got := lipgloss.Width(row); got != width {
		t.Errorf("selected row width = %d, want %d", got, width)
	}
	for _, value := range []string{"EPIC-A", "Alpha epic", "Sprint 1", "8", "3"} {
		if !strings.Contains(row, value) {
			t.Errorf("selected row missing %q: %q", value, row)
		}
	}
}

type epicLabelTestClient struct {
	api.Client
	issue          *models.Issue
	getIssueErr    error
	getIssueCalls  int
	setLabelsErr   error
	setLabelsCalls int
	setLabelsKey   string
	setLabels      []string
}

func (c *epicLabelTestClient) GetIssue(string) (*models.Issue, error) {
	c.getIssueCalls++
	return c.issue, c.getIssueErr
}

func (c *epicLabelTestClient) SetLabels(key string, labels []string) error {
	c.setLabelsCalls++
	c.setLabelsKey = key
	c.setLabels = append([]string(nil), labels...)
	return c.setLabelsErr
}

func newEpicLabelTestModel(client api.Client, issue *models.Issue) epicModel {
	return epicModel{
		state:  epicList,
		client: client,
		items: []epicItem{{
			Key:        issue.Key,
			Name:       issue.Summary,
			ChildCount: 1,
		}},
		sidebarFullIssue: issue,
		sidebarIssueKey:  issue.Key,
		cursor:           0,
		width:            100,
		height:           40,
	}
}

func TestEpicLabelHotkeyFetchesAndPrefillsCurrentLabels(t *testing.T) {
	issue := &models.Issue{Key: "EPIC-A", Summary: "Alpha", Labels: []string{"frontend", "urgent"}}
	client := &epicLabelTestClient{issue: issue}
	m := newEpicLabelTestModel(client, issue)
	m.sidebarFullIssue = nil
	m.sidebarIssueKey = ""

	updated, cmd := m.Update(keyPress("l"))
	m = updated.(epicModel)
	if m.state != epicLabelLoading {
		t.Fatalf("state after label hotkey = %v, want epicLabelLoading", m.state)
	}
	if cmd == nil {
		t.Fatal("label hotkey should fetch the full epic when sidebar data is absent")
	}

	fetched := fetchEpicLabelsCmd(client, issue.Key)()
	updated, focusCmd := m.Update(fetched)
	m = updated.(epicModel)
	if m.state != epicLabelInput {
		t.Fatalf("state after label fetch = %v, want epicLabelInput", m.state)
	}
	if focusCmd == nil {
		t.Fatal("opening label input should focus the text input")
	}
	if got := m.labelInput.Value(); got != "frontend, urgent" {
		t.Fatalf("prefilled labels = %q, want %q", got, "frontend, urgent")
	}
	if client.getIssueCalls != 1 {
		t.Fatalf("GetIssue calls = %d, want 1", client.getIssueCalls)
	}
}

func TestEpicLabelEditSaveUpdatesSidebarAndSupportsClear(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantLabels []string
	}{
		{name: "replace", input: "new, trimmed,,", wantLabels: []string{"new", "trimmed"}},
		{name: "clear", input: " , ", wantLabels: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := &models.Issue{Key: "EPIC-A", Summary: "Alpha", Labels: []string{"old"}}
			client := &epicLabelTestClient{issue: issue}
			m := newEpicLabelTestModel(client, issue)
			m.labelTargetKey = issue.Key
			if cmd := m.openLabelInput(issue); cmd == nil {
				t.Fatal("openLabelInput should return a focus command")
			}
			m.labelInput.SetValue(tt.input)

			updated, cmd := m.Update(keyPress("enter"))
			m = updated.(epicModel)
			if m.state != epicLabelSaving {
				t.Fatalf("state after save = %v, want epicLabelSaving", m.state)
			}
			if cmd == nil {
				t.Fatal("save should return an API command")
			}

			updated, _ = m.Update(setEpicLabelsCmd(client, issue.Key, parseLabels(tt.input))())
			m = updated.(epicModel)
			if m.state != epicList {
				t.Fatalf("state after save completion = %v, want epicList", m.state)
			}
			if client.setLabelsKey != issue.Key {
				t.Fatalf("SetLabels key = %q, want %q", client.setLabelsKey, issue.Key)
			}
			if len(client.setLabels) != len(tt.wantLabels) {
				t.Fatalf("saved labels = %v, want %v", client.setLabels, tt.wantLabels)
			}
			for i, label := range tt.wantLabels {
				if client.setLabels[i] != label {
					t.Errorf("saved labels[%d] = %q, want %q", i, client.setLabels[i], label)
				}
			}
			if len(m.sidebarFullIssue.Labels) != len(tt.wantLabels) {
				t.Fatalf("sidebar labels = %v, want %v", m.sidebarFullIssue.Labels, tt.wantLabels)
			}
		})
	}
}

func TestEpicLabelEditCancelAndSaveError(t *testing.T) {
	issue := &models.Issue{Key: "EPIC-A", Summary: "Alpha", Labels: []string{"old"}}
	client := &epicLabelTestClient{issue: issue, setLabelsErr: errors.New("permission denied")}
	m := newEpicLabelTestModel(client, issue)
	m.labelTargetKey = issue.Key
	m.openLabelInput(issue)
	m.labelInput.SetValue("new")

	updated, cmd := m.Update(keyPress("enter"))
	m = updated.(epicModel)
	if cmd == nil {
		t.Fatal("save should return an API command")
	}
	updated, _ = m.Update(setEpicLabelsCmd(client, issue.Key, parseLabels("new"))())
	m = updated.(epicModel)
	if m.state != epicLabelInput {
		t.Fatalf("state after save error = %v, want epicLabelInput", m.state)
	}
	if m.labelError != "permission denied" {
		t.Fatalf("label error = %q, want %q", m.labelError, "permission denied")
	}
	if got := m.labelInput.Value(); got != "new" {
		t.Fatalf("input after save error = %q, want %q", got, "new")
	}

	updated, _ = m.Update(keyPress("esc"))
	m = updated.(epicModel)
	if m.state != epicList {
		t.Fatalf("state after cancel = %v, want epicList", m.state)
	}
	if m.labelTargetKey != "" {
		t.Fatalf("label target after cancel = %q, want empty", m.labelTargetKey)
	}
	if client.setLabelsCalls != 1 {
		t.Fatalf("SetLabels calls = %d, want 1", client.setLabelsCalls)
	}
}

func TestParseLabelsTrimsAndReturnsEmptySlice(t *testing.T) {
	got := parseLabels(" first, second ,, third ")
	want := []string{"first", "second", "third"}
	if len(got) != len(want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
	for i, label := range want {
		if got[i] != label {
			t.Errorf("labels[%d] = %q, want %q", i, got[i], label)
		}
	}

	empty := parseLabels(" , ")
	if empty == nil {
		t.Fatal("empty label parsing should return a non-nil slice")
	}
	if len(empty) != 0 {
		t.Fatalf("empty labels = %v, want empty", empty)
	}
}
