package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/justinmklam/lazyjira/internal/models"
)

const sentinel = "<!-- lazyjira: do not remove this line or change field names -->"

// RenderTemplate produces a markdown template string for editing an issue.
// valid may be nil for new issues (create flow).
func RenderTemplate(issue *models.Issue, valid *models.ValidValues) string {
	var sb strings.Builder

	sb.WriteString(sentinel + "\n")

	// Type
	if valid != nil && len(valid.IssueTypes) > 0 {
		fmt.Fprintf(&sb, "<!-- Valid types: %s -->\n", strings.Join(valid.IssueTypes, ", "))
	}
	fmt.Fprintf(&sb, "type: %s\n\n", issue.IssueType)

	// Priority
	if valid != nil && len(valid.Priorities) > 0 {
		fmt.Fprintf(&sb, "<!-- Valid priorities: %s -->\n", strings.Join(valid.Priorities, ", "))
	}
	fmt.Fprintf(&sb, "priority: %s\n\n", issue.Priority)

	// Assignee
	fmt.Fprintf(&sb, "assignee: %s\n\n", issue.Assignee)

	// Story points
	sb.WriteString("<!-- Enter a number or leave blank -->\n")
	if issue.StoryPoints > 0 {
		fmt.Fprintf(&sb, "story_points: %.0f\n\n", issue.StoryPoints)
	} else {
		sb.WriteString("story_points:\n\n")
	}

	// Labels
	sb.WriteString("<!-- Comma-separated, e.g. backend, auth -->\n")
	fmt.Fprintf(&sb, "labels: %s\n\n", strings.Join(issue.Labels, ", "))

	// Separator
	sb.WriteString("---\n\n")

	// Title heading
	if issue.Key != "" {
		fmt.Fprintf(&sb, "# %s: %s\n\n", issue.Key, issue.Summary)
	} else {
		sb.WriteString("# Summary goes here\n\n")
	}

	// Description
	sb.WriteString("## Description\n\n")
	if issue.Description != "" {
		sb.WriteString(issue.Description)
		sb.WriteString("\n")
	} else {
		sb.WriteString("<!-- Add description here -->\n")
	}

	// Acceptance Criteria
	sb.WriteString("\n## Acceptance Criteria\n\n")
	if issue.AcceptanceCriteria != "" {
		sb.WriteString(issue.AcceptanceCriteria)
		sb.WriteString("\n")
	} else {
		sb.WriteString("<!-- Add acceptance criteria here -->\n")
	}

	// Linked Work Items (read-only reference, not editable)
	if len(issue.LinkedIssues) > 0 {
		sb.WriteString("\n## Linked Work Items\n\n")
		for _, li := range issue.LinkedIssues {
			status := ""
			if li.Status != "" {
				status = fmt.Sprintf(" (%s)", li.Status)
			}
			summary := ""
			if li.Summary != "" {
				summary = ": " + li.Summary
			}
			fmt.Fprintf(&sb, "<!-- %s %s%s%s -->\n", li.Relationship, li.Key, summary, status)
		}
	}

	return sb.String()
}

// WriteTempFile writes content to a temp .md file and returns its path.
func WriteTempFile(content string) (string, error) {
	f, err := os.CreateTemp("", "lazyjira-*.md")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return "", fmt.Errorf("writing temp file: %w", err)
	}
	return filepath.Abs(f.Name())
}
