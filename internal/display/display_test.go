package display

import (
	"strings"
	"testing"

	"github.com/justinmklam/lazyjira/internal/models"
)

func TestRenderIssue_Basic(t *testing.T) {
	issue := &models.Issue{
		Key:       "PROJ-1",
		Summary:   "Test issue",
		Status:    "In Progress",
		IssueType: "Bug",
		Priority:  "High",
	}

	got, err := RenderIssue(issue)
	if err != nil {
		t.Fatalf("RenderIssue error: %v", err)
	}

	checks := []string{
		"# PROJ-1: Test issue",
		"| **Status** | In Progress |",
		"| **Type** | Bug |",
		"| **Priority** | High |",
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output", want)
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

	got, err := RenderIssue(issue)
	if err != nil {
		t.Fatalf("RenderIssue error: %v", err)
	}

	checks := []string{
		"| **Reporter** | Alice |",
		"| **Assignee** | Bob |",
		"| **Story Points** | 8 |",
		"| **Sprint** | Sprint 5 |",
		"| **Labels** | frontend, urgent |",
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output", want)
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

	got, err := RenderIssue(issue)
	if err != nil {
		t.Fatalf("RenderIssue error: %v", err)
	}

	omitted := []string{"Reporter", "Assignee", "Story Points", "Sprint", "Labels"}
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

	got, err := RenderIssue(issue)
	if err != nil {
		t.Fatalf("RenderIssue error: %v", err)
	}
	if !strings.Contains(got, "## Description") {
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

	got, err := RenderIssue(issue)
	if err != nil {
		t.Fatalf("RenderIssue error: %v", err)
	}
	if !strings.Contains(got, "## Linked Work Items") {
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

	got, err := RenderIssue(issue)
	if err != nil {
		t.Fatalf("RenderIssue error: %v", err)
	}
	if !strings.Contains(got, "PROJ-6: Parent Epic") {
		t.Error("expected parent key with summary")
	}
}
