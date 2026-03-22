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
	if len(issue.LinkedIssues) > 0 {
		fmt.Fprintf(&sb, "\n# Linked Work Items\n\n")
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
