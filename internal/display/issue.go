package display

import (
	"fmt"
	"strings"

	"github.com/justinmklam/lazyjira/internal/models"
)

// RenderIssue returns a pure Markdown string suitable for piping to glow.
func RenderIssue(issue *models.Issue) (string, error) {
	var sb strings.Builder

	// Title
	fmt.Fprintf(&sb, "# %s: %s\n\n", issue.Key, issue.Summary)

	// Meta table
	fmt.Fprintf(&sb, "| | |\n|---|---|\n")
	fmt.Fprintf(&sb, "| **Status** | %s |\n", issue.Status)
	fmt.Fprintf(&sb, "| **Type** | %s |\n", issue.IssueType)
	fmt.Fprintf(&sb, "| **Priority** | %s |\n", issue.Priority)
	if issue.Reporter != "" {
		fmt.Fprintf(&sb, "| **Reporter** | %s |\n", issue.Reporter)
	}
	if issue.Assignee != "" {
		fmt.Fprintf(&sb, "| **Assignee** | %s |\n", issue.Assignee)
	}
	if issue.StoryPoints > 0 {
		fmt.Fprintf(&sb, "| **Story Points** | %.0f |\n", issue.StoryPoints)
	}
	if issue.SprintName != "" {
		fmt.Fprintf(&sb, "| **Sprint** | %s |\n", issue.SprintName)
	}
	if issue.ParentKey != "" {
		parent := issue.ParentKey
		if issue.ParentSummary != "" {
			parent = fmt.Sprintf("%s: %s", issue.ParentKey, issue.ParentSummary)
		}
		fmt.Fprintf(&sb, "| **Parent** | %s |\n", parent)
	}
	if len(issue.Labels) > 0 {
		fmt.Fprintf(&sb, "| **Labels** | %s |\n", strings.Join(issue.Labels, ", "))
	}

	// Description
	if issue.Description != "" {
		fmt.Fprintf(&sb, "\n## Description\n\n")
		sb.WriteString(issue.Description)
		sb.WriteString("\n")
	}

	// Acceptance Criteria
	if issue.AcceptanceCriteria != "" {
		fmt.Fprintf(&sb, "\n## Acceptance Criteria\n\n")
		sb.WriteString(issue.AcceptanceCriteria)
		sb.WriteString("\n")
	}

	// Linked Work Items
	if len(issue.LinkedIssues) > 0 {
		fmt.Fprintf(&sb, "\n## Linked Work Items\n\n")
		for _, li := range issue.LinkedIssues {
			status := ""
			if li.Status != "" {
				status = fmt.Sprintf(" (%s)", li.Status)
			}
			summary := ""
			if li.Summary != "" {
				summary = ": " + li.Summary
			}
			fmt.Fprintf(&sb, "- **%s** %s%s%s\n", li.Relationship, li.Key, summary, status)
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
