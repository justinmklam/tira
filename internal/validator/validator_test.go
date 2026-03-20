package validator

import (
	"strings"
	"testing"

	"github.com/justinmklam/tira/internal/models"
)

func TestValidate_AllValid(t *testing.T) {
	fields := &models.IssueFields{
		IssueType:   "Bug",
		Priority:    "High",
		Assignee:    "Jane Doe",
		StoryPoints: 3,
	}
	valid := &models.ValidValues{
		IssueTypes: []string{"Bug", "Story", "Task"},
		Priorities: []string{"High", "Medium", "Low"},
		Assignees:  []models.Assignee{{DisplayName: "Jane Doe", AccountID: "abc123"}},
	}

	errs := Validate(fields, valid)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidate_CaseInsensitive(t *testing.T) {
	fields := &models.IssueFields{
		IssueType: "bug",
		Priority:  "HIGH",
		Assignee:  "jane doe",
	}
	valid := &models.ValidValues{
		IssueTypes: []string{"Bug", "Story"},
		Priorities: []string{"High", "Medium"},
		Assignees:  []models.Assignee{{DisplayName: "Jane Doe", AccountID: "abc"}},
	}

	errs := Validate(fields, valid)
	if len(errs) != 0 {
		t.Errorf("expected no errors for case-insensitive match, got %v", errs)
	}
}

func TestValidate_InvalidType(t *testing.T) {
	fields := &models.IssueFields{IssueType: "Feature"}
	valid := &models.ValidValues{IssueTypes: []string{"Bug", "Story"}}

	errs := Validate(fields, valid)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if errs[0].Field != "type" {
		t.Errorf("expected field 'type', got %q", errs[0].Field)
	}
}

func TestValidate_InvalidPriority(t *testing.T) {
	fields := &models.IssueFields{Priority: "Critical"}
	valid := &models.ValidValues{Priorities: []string{"High", "Medium", "Low"}}

	errs := Validate(fields, valid)
	if len(errs) != 1 || errs[0].Field != "priority" {
		t.Errorf("expected priority error, got %v", errs)
	}
}

func TestValidate_InvalidAssignee(t *testing.T) {
	fields := &models.IssueFields{Assignee: "Unknown Person"}
	valid := &models.ValidValues{
		Assignees: []models.Assignee{{DisplayName: "Jane Doe", AccountID: "abc"}},
	}

	errs := Validate(fields, valid)
	if len(errs) != 1 || errs[0].Field != "assignee" {
		t.Errorf("expected assignee error, got %v", errs)
	}
}

func TestValidate_NegativeStoryPoints(t *testing.T) {
	fields := &models.IssueFields{StoryPoints: -1}
	valid := &models.ValidValues{}

	errs := Validate(fields, valid)
	if len(errs) != 1 || errs[0].Field != "story_points" {
		t.Errorf("expected story_points error, got %v", errs)
	}
}

func TestValidate_ZeroStoryPoints(t *testing.T) {
	fields := &models.IssueFields{StoryPoints: 0}
	valid := &models.ValidValues{}

	errs := Validate(fields, valid)
	if len(errs) != 0 {
		t.Errorf("expected no errors for zero story points, got %v", errs)
	}
}

func TestValidate_EmptyFieldsSkipped(t *testing.T) {
	fields := &models.IssueFields{}
	valid := &models.ValidValues{
		IssueTypes: []string{"Bug"},
		Priorities: []string{"High"},
		Assignees:  []models.Assignee{{DisplayName: "Jane", AccountID: "x"}},
	}

	errs := Validate(fields, valid)
	if len(errs) != 0 {
		t.Errorf("expected no errors for empty fields, got %v", errs)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	fields := &models.IssueFields{
		IssueType:   "Invalid",
		Priority:    "Wrong",
		StoryPoints: -5,
	}
	valid := &models.ValidValues{
		IssueTypes: []string{"Bug"},
		Priorities: []string{"High"},
	}

	errs := Validate(fields, valid)
	if len(errs) != 3 {
		t.Errorf("expected 3 errors, got %d: %v", len(errs), errs)
	}
}

func TestResolveAssigneeID_Found(t *testing.T) {
	fields := &models.IssueFields{Assignee: "Jane Doe"}
	valid := &models.ValidValues{
		Assignees: []models.Assignee{
			{DisplayName: "Jane Doe", AccountID: "abc123"},
			{DisplayName: "John Smith", AccountID: "xyz789"},
		},
	}

	id := ResolveAssigneeID(fields, valid)
	if id != "abc123" {
		t.Errorf("got %q, want %q", id, "abc123")
	}
}

func TestResolveAssigneeID_CaseInsensitive(t *testing.T) {
	fields := &models.IssueFields{Assignee: "jane doe"}
	valid := &models.ValidValues{
		Assignees: []models.Assignee{{DisplayName: "Jane Doe", AccountID: "abc"}},
	}

	id := ResolveAssigneeID(fields, valid)
	if id != "abc" {
		t.Errorf("got %q, want %q", id, "abc")
	}
}

func TestResolveAssigneeID_NotFound(t *testing.T) {
	fields := &models.IssueFields{Assignee: "Unknown"}
	valid := &models.ValidValues{
		Assignees: []models.Assignee{{DisplayName: "Jane Doe", AccountID: "abc"}},
	}

	id := ResolveAssigneeID(fields, valid)
	if id != "" {
		t.Errorf("expected empty string, got %q", id)
	}
}

func TestResolveAssigneeID_Empty(t *testing.T) {
	fields := &models.IssueFields{Assignee: ""}
	valid := &models.ValidValues{
		Assignees: []models.Assignee{{DisplayName: "Jane Doe", AccountID: "abc"}},
	}

	id := ResolveAssigneeID(fields, valid)
	if id != "" {
		t.Errorf("expected empty string, got %q", id)
	}
}

func TestAnnotateTemplate_InsertsError(t *testing.T) {
	content := "type: InvalidType\npriority: High\n"
	errs := []ValidationError{
		{Field: "type", Message: `"InvalidType" is not valid`},
	}

	result := AnnotateTemplate(content, errs)
	if !strings.Contains(result, `<!-- ERROR: "InvalidType" is not valid -->`) {
		t.Error("expected error comment in output")
	}
	// Error should appear before the type line.
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		if strings.Contains(line, "ERROR") {
			if i+1 < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i+1]), "type:") {
				t.Error("error comment should be directly above 'type:' line")
			}
			break
		}
	}
}

func TestAnnotateTemplate_ReplacesHintComment(t *testing.T) {
	content := "<!-- Valid types: Bug, Story -->\ntype: InvalidType\n"
	errs := []ValidationError{
		{Field: "type", Message: "not valid"},
	}

	result := AnnotateTemplate(content, errs)
	if strings.Contains(result, "Valid types") {
		t.Error("expected hint comment to be replaced")
	}
	if !strings.Contains(result, "ERROR") {
		t.Error("expected error comment")
	}
}

func TestAnnotateTemplate_NoErrors(t *testing.T) {
	content := "type: Bug\npriority: High\n"
	result := AnnotateTemplate(content, nil)
	if result != content {
		t.Errorf("expected unchanged content, got %q", result)
	}
}

func TestFieldKey(t *testing.T) {
	tests := []struct {
		line    string
		wantKey string
		wantOk  bool
	}{
		{"type: Bug", "type", true},
		{"priority: High", "priority", true},
		{"assignee: Jane", "assignee", true},
		{"story_points: 5", "story_points", true},
		{"labels: a, b", "labels", true},
		{"unknown: value", "", false},
		{"no colon here", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			key, ok := fieldKey(tt.line)
			if key != tt.wantKey || ok != tt.wantOk {
				t.Errorf("fieldKey(%q) = (%q, %v), want (%q, %v)", tt.line, key, ok, tt.wantKey, tt.wantOk)
			}
		})
	}
}

func TestIsHintComment(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"<!-- Valid types: Bug -->", true},
		{"<!-- ERROR: bad -->", true},
		{"<!-- tira: do not remove -->", false},
		{"type: Bug", false},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			if got := isHintComment(tt.line); got != tt.want {
				t.Errorf("isHintComment(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}
