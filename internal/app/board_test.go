package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/justinmklam/tira/internal/models"
)

func keyPress(text string) tea.KeyPressMsg {
	runes := []rune(text)
	var code rune
	if len(runes) > 0 {
		code = runes[0]
	}
	return tea.KeyPressMsg(tea.Key{Code: code, Text: text})
}

func testBoardGroups() []models.SprintGroup {
	return []models.SprintGroup{
		{
			Sprint: models.Sprint{Name: "Sprint 1", State: "active"},
			Issues: []models.Issue{
				{Key: "P-1", Summary: "First", EpicKey: "EPIC-B", EpicName: "Beta"},
				{Key: "P-2", Summary: "Second", EpicKey: "EPIC-A", EpicName: "Alpha"},
			},
		},
		{
			Sprint: models.Sprint{Name: "Backlog", State: "backlog"},
			Issues: []models.Issue{
				{Key: "P-3", Summary: "Third", EpicKey: "EPIC-B", EpicName: "Beta"},
			},
		},
	}
}

func TestBoardViewSwitchToEpicsSyncsBacklogProjection(t *testing.T) {
	groups := testBoardGroups()
	m := boardModel{
		activeView: ViewKanban,
		backlog: blModel{
			state:     blList,
			groups:    groups,
			rows:      blBuildRows(groups, map[int]bool{}, "", ""),
			collapsed: map[int]bool{},
		},
		kanban: kanbanModel{state: stateBoard},
		epics: epicModel{
			state:   epicList,
			loading: true,
			items:   buildEpicItems(groups[:1]),
		},
	}

	updated, _ := m.Update(keyPress("3"))
	got := updated.(boardModel)

	if got.activeView != ViewEpics {
		t.Fatalf("active view = %v, want ViewEpics", got.activeView)
	}
	if len(got.epics.items) != 2 {
		t.Fatalf("epic count = %d, want 2", len(got.epics.items))
	}
	if got.epics.items[0].Key != "EPIC-B" || got.epics.items[1].Key != "EPIC-A" {
		t.Fatalf("unexpected epic order: %#v", got.epics.items)
	}
	if !got.epics.loading {
		t.Fatal("switching views should preserve progressive loading state")
	}
}

func TestBoardTabCyclesAllViews(t *testing.T) {
	m := boardModel{
		activeView: ViewBacklog,
		backlog:    blModel{state: blList},
		kanban:     kanbanModel{state: stateBoard},
		epics:      epicModel{state: epicList},
	}
	want := []BoardView{ViewKanban, ViewEpics, ViewBacklog}

	for i, expected := range want {
		updated, _ := m.Update(keyPress("tab"))
		m = updated.(boardModel)
		if m.activeView != expected {
			t.Fatalf("Tab %d: active view = %v, want %v", i+1, m.activeView, expected)
		}
	}
}

func TestBoardDirectViewKeys(t *testing.T) {
	m := boardModel{
		activeView: ViewBacklog,
		backlog:    blModel{state: blList},
		kanban:     kanbanModel{state: stateBoard},
		epics:      epicModel{state: epicList},
	}
	tests := []struct {
		key  string
		want BoardView
	}{
		{key: "2", want: ViewKanban},
		{key: "3", want: ViewEpics},
		{key: "1", want: ViewBacklog},
	}

	for _, tt := range tests {
		updated, _ := m.Update(keyPress(tt.key))
		m = updated.(boardModel)
		if m.activeView != tt.want {
			t.Fatalf("key %q: active view = %v, want %v", tt.key, m.activeView, tt.want)
		}
	}
}

func TestBoardDoesNotSwitchViewsFromEpicDetail(t *testing.T) {
	m := boardModel{
		activeView: ViewEpics,
		epics: epicModel{
			state:       epicDetail,
			detailIssue: &models.Issue{Key: "EPIC-A"},
		},
	}

	updated, _ := m.Update(keyPress("1"))
	got := updated.(boardModel)
	if got.activeView != ViewEpics {
		t.Fatalf("active view = %v, want ViewEpics while detail is open", got.activeView)
	}
}

func TestBoardAppliesEpicFilterFromEpicsView(t *testing.T) {
	groups := testBoardGroups()
	items := buildEpicItems(groups)
	m := boardModel{
		activeView: ViewEpics,
		backlog: blModel{
			state:     blList,
			groups:    groups,
			rows:      blBuildRows(groups, map[int]bool{}, "", ""),
			collapsed: map[int]bool{},
		},
		epics: epicModel{
			state:  epicList,
			items:  items,
			cursor: 0,
		},
	}

	updated, _ := m.Update(keyPress("b"))
	got := updated.(boardModel)

	if got.activeView != ViewBacklog {
		t.Fatalf("active view = %v, want ViewBacklog", got.activeView)
	}
	if got.backlog.filterEpic != "EPIC-B" {
		t.Fatalf("epic filter = %q, want EPIC-B", got.backlog.filterEpic)
	}
	for _, row := range got.backlog.rows {
		if row.kind == blRowIssue {
			issue := got.backlog.groups[row.groupIdx].Issues[row.issueIdx]
			if issue.EpicKey != "EPIC-B" {
				t.Fatalf("filtered rows contain %s", issue.Key)
			}
		}
	}
}

func TestBoardShowsLazyLoadErrorInEpics(t *testing.T) {
	groups := testBoardGroups()
	m := boardModel{
		activeView: ViewEpics,
		backlog: blModel{
			groups: groups[:1],
		},
		epics: epicModel{
			state:   epicList,
			loading: true,
			items:   buildEpicItems(groups[:1]),
		},
	}

	updated, _ := m.Update(blLazyLoadDoneMsg{err: errTestLazyLoad})
	got := updated.(boardModel)

	if got.epics.loading {
		t.Fatal("epic loading state remained active after lazy-load completion")
	}
	if !strings.Contains(got.epics.loadError, "backlog unavailable") {
		t.Fatalf("load error = %q, want backlog error", got.epics.loadError)
	}
}

func TestBoardRefreshSynchronizesEpics(t *testing.T) {
	initial := testBoardGroups()[:1]
	refreshed := []models.SprintGroup{{
		Sprint: models.Sprint{Name: "Sprint 2", State: "active"},
		Issues: []models.Issue{
			{Key: "P-4", EpicKey: "EPIC-C", EpicName: "Gamma"},
		},
	}}
	m := boardModel{
		activeView: ViewEpics,
		backlog: blModel{
			groups: initial,
		},
		epics: epicModel{
			state:   epicList,
			loading: true,
			items:   buildEpicItems(initial),
		},
	}

	updated, _ := m.Update(boardRefreshDoneMsg{
		data: BoardInitData{
			Groups:    refreshed,
			BoardCols: []models.BoardColumn{{Name: "To Do"}},
		},
	})
	got := updated.(boardModel)

	if len(got.backlog.groups) != 1 || got.backlog.groups[0].Issues[0].Key != "P-4" {
		t.Fatalf("backlog groups were not refreshed: %#v", got.backlog.groups)
	}
	if len(got.epics.items) != 1 || got.epics.items[0].Key != "EPIC-C" {
		t.Fatalf("epic projection was not refreshed: %#v", got.epics.items)
	}
	if got.epics.loading {
		t.Fatal("epic loading state remained active after full refresh")
	}
}

func TestBoardLazyLoadAppendsGroupsToEpics(t *testing.T) {
	initial := testBoardGroups()[:1]
	m := boardModel{
		activeView: ViewEpics,
		backlog: blModel{
			groups: initial,
		},
		epics: epicModel{
			state:   epicList,
			loading: true,
			items:   buildEpicItems(initial),
		},
	}

	updated, _ := m.Update(blLazyLoadDoneMsg{groups: testBoardGroups()[1:]})
	got := updated.(boardModel)

	if got.epics.loading {
		t.Fatal("epic loading state remained active after lazy-load completion")
	}
	if len(got.epics.items) != 2 {
		t.Fatalf("epic count = %d, want 2", len(got.epics.items))
	}
	if len(got.initData.Groups) != 2 {
		t.Fatalf("init group count = %d, want 2", len(got.initData.Groups))
	}
}

var errTestLazyLoad = lazyLoadTestError("backlog unavailable")

type lazyLoadTestError string

func (e lazyLoadTestError) Error() string { return string(e) }
