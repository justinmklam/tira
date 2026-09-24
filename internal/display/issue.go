package display

import (
	"fmt"
	"strings"
	"time"

	"github.com/justinmklam/tira/internal/models"
)

// RenderIssue returns a pure Markdown string suitable for piping to glow.
func RenderIssue(issue *models.Issue) string {
	var sb strings.Builder

	// Issue title as H1 heading
	fmt.Fprintf(&sb, "# %s  %s\n\n", issue.Key, issue.Summary)

	// Metadata list — build rows first so we can align values.
	type metaRow struct{ key, val string }
	assignee := issue.Assignee
	if assignee == "" {
		assignee = "—"
	}
	var spText string
	if issue.StoryPoints > 0 {
		if issue.StoryPoints == float64(int(issue.StoryPoints)) {
			spText = fmt.Sprintf("%.0f", issue.StoryPoints)
		} else {
			spText = fmt.Sprintf("%.1f", issue.StoryPoints)
		}
	} else {
		spText = "—"
	}
	rows := []metaRow{{"Assignee", assignee}, {"Status", issue.Status}, {"Story Points", spText}}
	rows = append(rows, metaRow{"Type", issue.IssueType})
	priority := issue.Priority
	if priority == "" {
		priority = "—"
	}
	rows = append(rows, metaRow{"Priority", priority})
	sprintName := issue.SprintName
	if sprintName == "" {
		sprintName = "—"
	}
	rows = append(rows, metaRow{"Sprint", sprintName})
	parent := "—"
	if issue.ParentKey != "" {
		parent = issue.ParentKey
		if issue.ParentSummary != "" {
			parent = fmt.Sprintf("%s: %s", issue.ParentKey, issue.ParentSummary)
		}
	}
	rows = append(rows, metaRow{"Parent", parent})
	reporter := issue.Reporter
	if reporter == "" {
		reporter = "—"
	}
	rows = append(rows, metaRow{"Reporter", reporter})
	labels := issue.Labels
	labelsStr := "—"
	if len(labels) > 0 {
		labelsStr = strings.Join(labels, ", ")
	}
	rows = append(rows, metaRow{"Labels", labelsStr})

	maxKeyLen := 0
	for _, r := range rows {
		if len(r.key) > maxKeyLen {
			maxKeyLen = len(r.key)
		}
	}
	for _, r := range rows {
		// Use non-breaking spaces so goldmark doesn't collapse the padding.
		pad := strings.Repeat("\u00a0", maxKeyLen-len(r.key)+1)
		fmt.Fprintf(&sb, "- **%s:** %s%s\n", r.key, pad, r.val)
	}

	// Description
	fmt.Fprintf(&sb, "\n# Description\n\n")
	if issue.Description != "" {
		sb.WriteString(issue.Description)
		sb.WriteString("\n")
	} else {
		fmt.Fprintf(&sb, "*No description*")
	}

	// Acceptance Criteria
	if issue.AcceptanceCriteria != "" {
		fmt.Fprintf(&sb, "\n# Acceptance Criteria\n\n")
		sb.WriteString(issue.AcceptanceCriteria)
		sb.WriteString("\n")
	}

	// Linked Work Items
	sb.WriteString(LinkedItemsSection("Linked Work Items", issue.LinkedIssues))

	// Comments
	fmt.Fprintf(&sb, "\n# Comments\n\n")
	if len(issue.Comments) > 0 {
		for _, c := range issue.Comments {
			fmt.Fprintf(&sb, "**%s** _%s_\n\n", c.Author, formatCommentTime(c.Created))
			sb.WriteString(c.Body)
			sb.WriteString("\n\n---\n\n")
		}
	} else {
		fmt.Fprintf(&sb, "*No comments*")
	}

	return sb.String()
}

// LinkedItemsSection returns a Markdown heading and flat bullet list for
// related work items, or "" when there are none. It is shared by the issue
// renderer and the epic views so every surface formats links the same way.
func LinkedItemsSection(title string, items []models.LinkedIssue) string {
	if len(items) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n# %s\n\n", title)
	for _, li := range items {
		sb.WriteString(FormatLinkedItem(li))
	}
	sb.WriteString("\n")
	return sb.String()
}

// FormatLinkedItem renders one related work item as a Markdown bullet, e.g.
// "- **blocks** PROJ-6: Fix login (Bug · High · In Progress)".
func FormatLinkedItem(li models.LinkedIssue) string {
	line := fmt.Sprintf("- **%s** %s", li.Relationship, li.Key)
	if li.Summary != "" {
		line += ": " + li.Summary
	}
	if detail := linkAttributes(li); detail != "" {
		line += " (" + detail + ")"
	}
	return line + "\n"
}

// linkAttributes summarises the known issue type, priority, status, and subtask
// count for display, skipping any that are empty.
func linkAttributes(li models.LinkedIssue) string {
	attributes := make([]string, 0, 4)
	if li.IssueType != "" {
		attributes = append(attributes, li.IssueType)
	}
	if li.Priority != "" {
		attributes = append(attributes, li.Priority)
	}
	if li.Status != "" {
		attributes = append(attributes, li.Status)
	}
	if li.SubTaskCount > 0 {
		attributes = append(attributes, fmt.Sprintf("%d subtasks", li.SubTaskCount))
	}
	return strings.Join(attributes, " · ")
}

// formatCommentTime parses a Jira timestamp and returns a human-readable string.
// Jira returns timestamps as "2006-01-02T15:04:05.000-0700".
func formatCommentTime(s string) string {
	formats := []string{
		"2006-01-02T15:04:05.999-0700",
		"2006-01-02T15:04:05.999Z",
		"2006-01-02T15:04:05-0700",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t.UTC().Format("January 2, 2006 at 3:04 PM UTC")
		}
	}
	return s
}
