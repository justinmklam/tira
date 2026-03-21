package app

import (
	"testing"

	"github.com/justinmklam/tira/internal/models"
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
