package app

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
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
