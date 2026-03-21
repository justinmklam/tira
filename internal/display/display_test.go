package display

import (
	"strings"
	"testing"

	"github.com/justinmklam/tira/internal/models"
)

func TestRenderIssue_Basic(t *testing.T) {
	issue := &models.Issue{
		Key:       "PROJ-1",
		Summary:   "Test issue",
		Status:    "In Progress",
		IssueType: "Bug",
		Priority:  "High",
	}

	got := RenderIssue(issue)

	checks := [][2]string{
		{"**Status:**", "In Progress"},
		{"**Type:**", "Bug"},
		{"**Priority:**", "High"},
	}
	for _, pair := range checks {
		if !strings.Contains(got, pair[0]) {
			t.Errorf("expected key %q in output", pair[0])
		}
		if !strings.Contains(got, pair[1]) {
			t.Errorf("expected value %q in output", pair[1])
		}
	}
}

func TestRenderIssue_OptionalFields(t *testing.T) {
	issue := &models.Issue{
		Key:         "PROJ-2",
		Summary:     "Full issue",
		Status:      "Done",
		IssueType:   "Story",
		Priority:    "Medium",
		Reporter:    "Alice",
		Assignee:    "Bob",
		StoryPoints: 8,
		SprintName:  "Sprint 5",
		Labels:      []string{"frontend", "urgent"},
	}

	got := RenderIssue(issue)

	checks := [][2]string{
		{"**Reporter:**", "Alice"},
		{"**Assignee:**", "Bob"},
		{"**Story Points:**", "8"},
		{"**Sprint:**", "Sprint 5"},
		{"**Labels:**", "frontend, urgent"},
	}
	for _, pair := range checks {
		if !strings.Contains(got, pair[0]) {
			t.Errorf("expected key %q in output", pair[0])
		}
		if !strings.Contains(got, pair[1]) {
			t.Errorf("expected value %q in output", pair[1])
		}
	}
}

func TestRenderIssue_OmitsEmptyOptional(t *testing.T) {
	issue := &models.Issue{
		Key:       "PROJ-3",
		Summary:   "Minimal",
		Status:    "Open",
		IssueType: "Task",
		Priority:  "Low",
	}

	got := RenderIssue(issue)

	omitted := []string{"Reporter", "Story Points", "Sprint", "Labels"}
	for _, field := range omitted {
		if strings.Contains(got, "**"+field+"**") {
			t.Errorf("expected %q to be omitted", field)
		}
	}
}

func TestRenderIssue_WithDescription(t *testing.T) {
	issue := &models.Issue{
		Key:         "PROJ-4",
		Summary:     "Desc test",
		Status:      "Open",
		IssueType:   "Bug",
		Priority:    "High",
		Description: "This is the description.",
	}

	got := RenderIssue(issue)
	if !strings.Contains(got, "# Description") {
		t.Error("expected Description section")
	}
	if !strings.Contains(got, "This is the description.") {
		t.Error("expected description content")
	}
}

func TestRenderIssue_WithLinkedIssues(t *testing.T) {
	issue := &models.Issue{
		Key:       "PROJ-5",
		Summary:   "Linked",
		Status:    "Open",
		IssueType: "Story",
		Priority:  "Medium",
		LinkedIssues: []models.LinkedIssue{
			{Relationship: "blocks", Key: "PROJ-6", Summary: "Other", Status: "Done"},
		},
	}

	got := RenderIssue(issue)
	if !strings.Contains(got, "# Linked Work Items") {
		t.Error("expected Linked Work Items section")
	}
	if !strings.Contains(got, "**blocks** PROJ-6") {
		t.Error("expected linked issue")
	}
}

func TestRenderIssue_WithParent(t *testing.T) {
	issue := &models.Issue{
		Key:           "PROJ-7",
		Summary:       "Child",
		Status:        "Open",
		IssueType:     "Sub-task",
		Priority:      "Low",
		ParentKey:     "PROJ-6",
		ParentSummary: "Parent Epic",
	}

	got := RenderIssue(issue)
	if !strings.Contains(got, "PROJ-6: Parent Epic") {
		t.Error("expected parent key with summary")
	}
}
