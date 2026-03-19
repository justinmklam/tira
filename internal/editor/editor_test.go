package editor

import (
	"testing"

	"github.com/justinmklam/lazyjira/internal/models"
)

func TestRenderThenParse_RoundTrip(t *testing.T) {
	issue := &models.Issue{
		Key:                "PROJ-42",
		Summary:            "Fix login bug",
		IssueType:          "Bug",
		Priority:           "High",
		Assignee:           "Jane Doe",
		StoryPoints:        5,
		Labels:             []string{"backend", "auth"},
		Description:        "Users cannot log in after password reset.",
		AcceptanceCriteria: "- Login works after reset\n- Error message is clear",
	}
	valid := &models.ValidValues{
		IssueTypes: []string{"Bug", "Story", "Task"},
		Priorities: []string{"High", "Medium", "Low"},
	}

	tmpl := RenderTemplate(issue, valid)
	fields, err := ParseTemplate(tmpl)
	if err != nil {
		t.Fatalf("ParseTemplate error: %v", err)
	}

	assertEqual(t, "Summary", issue.Summary, fields.Summary)
	assertEqual(t, "IssueType", issue.IssueType, fields.IssueType)
	assertEqual(t, "Priority", issue.Priority, fields.Priority)
	assertEqual(t, "Assignee", issue.Assignee, fields.Assignee)
	if fields.StoryPoints != issue.StoryPoints {
		t.Errorf("StoryPoints: got %v, want %v", fields.StoryPoints, issue.StoryPoints)
	}
	if len(fields.Labels) != len(issue.Labels) {
		t.Fatalf("Labels length: got %d, want %d", len(fields.Labels), len(issue.Labels))
	}
	for i, l := range fields.Labels {
		assertEqual(t, "Labels[i]", issue.Labels[i], l)
	}
	assertEqual(t, "Description", issue.Description, fields.Description)
	assertEqual(t, "AcceptanceCriteria", issue.AcceptanceCriteria, fields.AcceptanceCriteria)
}

func TestRenderThenParse_EmptyFields(t *testing.T) {
	issue := &models.Issue{
		Key:       "PROJ-1",
		Summary:   "Minimal issue",
		IssueType: "Task",
		Priority:  "Medium",
	}

	tmpl := RenderTemplate(issue, nil)
	fields, err := ParseTemplate(tmpl)
	if err != nil {
		t.Fatalf("ParseTemplate error: %v", err)
	}

	assertEqual(t, "Summary", "Minimal issue", fields.Summary)
	assertEqual(t, "IssueType", "Task", fields.IssueType)
	assertEqual(t, "Description", "", fields.Description)
	assertEqual(t, "AcceptanceCriteria", "", fields.AcceptanceCriteria)
	if fields.StoryPoints != 0 {
		t.Errorf("StoryPoints: got %v, want 0", fields.StoryPoints)
	}
	if len(fields.Labels) != 0 {
		t.Errorf("Labels: got %v, want empty", fields.Labels)
	}
}

func TestRenderThenParse_NewIssue(t *testing.T) {
	issue := &models.Issue{
		Summary:   "New feature",
		IssueType: "Story",
		Priority:  "Low",
	}

	tmpl := RenderTemplate(issue, nil)
	fields, err := ParseTemplate(tmpl)
	if err != nil {
		t.Fatalf("ParseTemplate error: %v", err)
	}

	// New issues have no key, so summary heading is "# Summary goes here"
	// which extracts as "Summary goes here" (there's no ": " separator).
	assertEqual(t, "Summary", "Summary goes here", fields.Summary)
	assertEqual(t, "IssueType", "Story", fields.IssueType)
}

func TestParseTemplate_MissingSentinel(t *testing.T) {
	_, err := ParseTemplate("type: Bug\n---\n# PROJ-1: Hello")
	if err == nil {
		t.Error("expected error for missing sentinel, got nil")
	}
}

func TestParseTemplate_StoryPointsInvalid(t *testing.T) {
	content := sentinel + "\ntype: Bug\nstory_points: abc\n---\n# PROJ-1: Hello"
	_, err := ParseTemplate(content)
	if err == nil {
		t.Error("expected error for invalid story_points, got nil")
	}
}

func TestSplitOnSeparator(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantParts int
	}{
		{"with separator", "before\n---\nafter", 2},
		{"no separator", "no separator here", 1},
		{"separator with spaces", "before\n  ---  \nafter", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := splitOnSeparator(tt.input)
			if len(parts) != tt.wantParts {
				t.Errorf("got %d parts, want %d", len(parts), tt.wantParts)
			}
		})
	}
}

func TestExtractSummary(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		want  string
	}{
		{"with key prefix", "# PROJ-1: My Summary", "My Summary"},
		{"without key prefix", "# Summary goes here", "Summary goes here"},
		{"no heading", "just text", ""},
		{"empty body", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSummary(tt.body)
			assertEqual(t, "summary", tt.want, got)
		})
	}
}

func TestExtractSection(t *testing.T) {
	body := "## Description\n\nSome description text\n\n## Acceptance Criteria\n\n- Item 1\n- Item 2"

	desc := extractSection(body, "Description")
	assertEqual(t, "Description", "Some description text", desc)

	ac := extractSection(body, "Acceptance Criteria")
	assertEqual(t, "AcceptanceCriteria", "- Item 1\n- Item 2", ac)

	missing := extractSection(body, "Nonexistent")
	assertEqual(t, "Nonexistent", "", missing)
}

func TestExtractSection_SkipsComments(t *testing.T) {
	body := "## Description\n\n<!-- Add description here -->\n"
	got := extractSection(body, "Description")
	assertEqual(t, "Description", "", got)
}

func TestRenderTemplate_WithLinkedIssues(t *testing.T) {
	issue := &models.Issue{
		Key:       "PROJ-10",
		Summary:   "Parent task",
		IssueType: "Story",
		Priority:  "Medium",
		LinkedIssues: []models.LinkedIssue{
			{Relationship: "blocks", Key: "PROJ-11", Summary: "Child task", Status: "In Progress"},
		},
	}

	tmpl := RenderTemplate(issue, nil)
	if !containsStr(tmpl, "## Linked Work Items") {
		t.Error("expected Linked Work Items section")
	}
	if !containsStr(tmpl, "blocks PROJ-11") {
		t.Error("expected linked issue reference")
	}
}

func TestRenderTemplate_WithParent(t *testing.T) {
	issue := &models.Issue{
		Key:           "PROJ-20",
		Summary:       "Sub-task",
		IssueType:     "Sub-task",
		Priority:      "Low",
		ParentKey:     "PROJ-19",
		ParentSummary: "Epic task",
	}

	tmpl := RenderTemplate(issue, nil)
	if !containsStr(tmpl, "parent: PROJ-19: Epic task") {
		t.Error("expected parent reference in template")
	}
}

func assertEqual(t *testing.T, field, want, got string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}

func containsStr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && contains(s, substr)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
