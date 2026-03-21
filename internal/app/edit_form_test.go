package app

import (
	"testing"

	"github.com/justinmklam/tira/internal/models"
)

func TestToIssueFields_AllFields(t *testing.T) {
	state := editFormState{
		summary:            "Test Summary",
		issueType:          "Story",
		priority:           "High",
		assignee:           "John Doe",
		origAssignee:       "Jane Doe",
		origAssigneeID:     "account-123",
		storyPoints:        "5",
		labels:             "label1, label2, label3",
		description:        "Test description",
		acceptanceCriteria: "Test acceptance criteria",
	}

	valid := &models.ValidValues{
		IssueTypes: []string{"Bug", "Story", "Task"},
		Priorities: []string{"Low", "Medium", "High"},
		Assignees:  []models.Assignee{{DisplayName: "John Doe", AccountID: "account-456"}},
	}

	fields := state.toIssueFields(valid)

	if fields.Summary != "Test Summary" {
		t.Errorf("Summary = %q, want %q", fields.Summary, "Test Summary")
	}
	if fields.IssueType != "Story" {
		t.Errorf("IssueType = %q, want %q", fields.IssueType, "Story")
	}
	if fields.Priority != "High" {
		t.Errorf("Priority = %q, want %q", fields.Priority, "High")
	}
	if fields.Assignee != "John Doe" {
		t.Errorf("Assignee = %q, want %q", fields.Assignee, "John Doe")
	}
	if fields.StoryPoints != 5 {
		t.Errorf("StoryPoints = %v, want 5", fields.StoryPoints)
	}
	if len(fields.Labels) != 3 {
		t.Errorf("Labels = %v, want 3 labels", len(fields.Labels))
	}
	if fields.Description != "Test description" {
		t.Errorf("Description = %q, want %q", fields.Description, "Test description")
	}
	if fields.AcceptanceCriteria != "Test acceptance criteria" {
		t.Errorf("AcceptanceCriteria = %q, want %q", fields.AcceptanceCriteria, "Test acceptance criteria")
	}
}

func TestToIssueFields_AssigneeUnchanged_ReusesID(t *testing.T) {
	state := editFormState{
		summary:            "Test",
		issueType:          "Bug",
		priority:           "Medium",
		assignee:           "John Doe",
		origAssignee:       "John Doe",
		origAssigneeID:     "original-account-id",
		storyPoints:        "3",
		labels:             "",
		description:        "",
		acceptanceCriteria: "",
	}

	valid := &models.ValidValues{}
	fields := state.toIssueFields(valid)

	// When assignee is unchanged, should reuse original AccountID
	if fields.AssigneeID != "original-account-id" {
		t.Errorf("AssigneeID = %q, want %q", fields.AssigneeID, "original-account-id")
	}
}

func TestToIssueFields_AssigneeChanged_ResolvesID(t *testing.T) {
	state := editFormState{
		summary:            "Test",
		issueType:          "Bug",
		priority:           "Medium",
		assignee:           "New User",
		origAssignee:       "Old User",
		origAssigneeID:     "old-account-id",
		storyPoints:        "3",
		labels:             "",
		description:        "",
		acceptanceCriteria: "",
	}

	valid := &models.ValidValues{
		Assignees: []models.Assignee{
			{DisplayName: "New User", AccountID: "new-account-id"},
			{DisplayName: "Old User", AccountID: "old-account-id"},
		},
	}

	fields := state.toIssueFields(valid)

	// When assignee changes, should resolve new AccountID
	if fields.AssigneeID != "new-account-id" {
		t.Errorf("AssigneeID = %q, want %q", fields.AssigneeID, "new-account-id")
	}
}

func TestToIssueFields_EmptyStoryPoints(t *testing.T) {
	state := editFormState{
		summary:            "Test",
		issueType:          "Bug",
		priority:           "Medium",
		assignee:           "",
		origAssignee:       "",
		origAssigneeID:     "",
		storyPoints:        "",
		labels:             "",
		description:        "",
		acceptanceCriteria: "",
	}

	valid := &models.ValidValues{}
	fields := state.toIssueFields(valid)

	if fields.StoryPoints != 0 {
		t.Errorf("StoryPoints = %v, want 0 for empty input", fields.StoryPoints)
	}
}

func TestToIssueFields_InvalidStoryPoints(t *testing.T) {
	state := editFormState{
		summary:            "Test",
		issueType:          "Bug",
		priority:           "Medium",
		assignee:           "",
		origAssignee:       "",
		origAssigneeID:     "",
		storyPoints:        "not-a-number",
		labels:             "",
		description:        "",
		acceptanceCriteria: "",
	}

	valid := &models.ValidValues{}
	fields := state.toIssueFields(valid)

	if fields.StoryPoints != 0 {
		t.Errorf("StoryPoints = %v, want 0 for invalid input", fields.StoryPoints)
	}
}

func TestToIssueFields_LabelsCommaSeparated(t *testing.T) {
	state := editFormState{
		summary:            "Test",
		issueType:          "Bug",
		priority:           "Medium",
		assignee:           "",
		origAssignee:       "",
		origAssigneeID:     "",
		storyPoints:        "",
		labels:             "  label1  , label2 ,  label3  ",
		description:        "",
		acceptanceCriteria: "",
	}

	valid := &models.ValidValues{}
	fields := state.toIssueFields(valid)

	expectedLabels := []string{"label1", "label2", "label3"}
	if len(fields.Labels) != len(expectedLabels) {
		t.Fatalf("Labels = %v, want %v", fields.Labels, expectedLabels)
	}
	for i, label := range fields.Labels {
		if label != expectedLabels[i] {
			t.Errorf("Labels[%d] = %q, want %q", i, label, expectedLabels[i])
		}
	}
}

func TestToIssueFields_LabelsEmpty(t *testing.T) {
	state := editFormState{
		summary:            "Test",
		issueType:          "Bug",
		priority:           "Medium",
		assignee:           "",
		origAssignee:       "",
		origAssigneeID:     "",
		storyPoints:        "",
		labels:             "",
		description:        "",
		acceptanceCriteria: "",
	}

	valid := &models.ValidValues{}
	fields := state.toIssueFields(valid)

	if len(fields.Labels) != 0 {
		t.Errorf("Labels = %v, want empty slice", fields.Labels)
	}
}

func TestToIssueFields_TrimWhitespace(t *testing.T) {
	state := editFormState{
		summary:            "  Test Summary  ",
		issueType:          "  Story  ",
		priority:           "  High  ",
		assignee:           "  John Doe  ",
		origAssignee:       "  John Doe  ",
		origAssigneeID:     "account-123",
		storyPoints:        "  5  ",
		labels:             "",
		description:        "  Description  ",
		acceptanceCriteria: "  AC  ",
	}

	valid := &models.ValidValues{}
	fields := state.toIssueFields(valid)

	if fields.Summary != "Test Summary" {
		t.Errorf("Summary not trimmed: %q", fields.Summary)
	}
	if fields.Description != "Description" {
		t.Errorf("Description not trimmed: %q", fields.Description)
	}
	if fields.AcceptanceCriteria != "AC" {
		t.Errorf("AcceptanceCriteria not trimmed: %q", fields.AcceptanceCriteria)
	}
}
