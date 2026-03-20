package editor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/justinmklam/tira/internal/models"
)

// ParseTemplate parses a template string (as written by RenderTemplate) back
// into IssueFields. Returns an error if the sentinel line is missing.
func ParseTemplate(content string) (*models.IssueFields, error) {
	if !strings.Contains(content, sentinel) {
		return nil, fmt.Errorf("sentinel line missing — template may be corrupted")
	}

	// Split into front-matter and body on "---" separator.
	// We want the first "---" that appears on its own line.
	parts := splitOnSeparator(content)
	frontMatter := parts[0]
	body := ""
	if len(parts) > 1 {
		body = parts[1]
	}

	fields := &models.IssueFields{}

	// Parse key: value pairs from front matter (skip comment lines and blank lines).
	for _, line := range strings.Split(frontMatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "<!--") || line == sentinel {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		switch key {
		case "type":
			fields.IssueType = val
		case "priority":
			fields.Priority = val
		case "assignee":
			fields.Assignee = val
		case "story_points":
			if val != "" {
				sp, err := strconv.ParseFloat(val, 64)
				if err != nil {
					return nil, fmt.Errorf("story_points: %q is not a valid number", val)
				}
				fields.StoryPoints = sp
			}
		case "labels":
			if val != "" {
				for _, l := range strings.Split(val, ",") {
					fields.Labels = append(fields.Labels, strings.TrimSpace(l))
				}
			}
		}
	}

	fields.Summary = extractSummary(body)
	fields.Description = extractSection(body, "Description")
	fields.AcceptanceCriteria = extractSection(body, "Acceptance Criteria")

	return fields, nil
}

// splitOnSeparator splits content on the first "---" line (alone on the line).
func splitOnSeparator(content string) []string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			before := strings.Join(lines[:i], "\n")
			after := strings.Join(lines[i+1:], "\n")
			return []string{before, after}
		}
	}
	return []string{content}
}

// extractSummary finds "# KEY: Summary" or "# Summary goes here" and returns
// just the summary text (everything after the first ": " or after "# ").
func extractSummary(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			heading := strings.TrimPrefix(line, "# ")
			if idx := strings.Index(heading, ": "); idx >= 0 {
				return strings.TrimSpace(heading[idx+2:])
			}
			return heading
		}
	}
	return ""
}

// extractSection returns the trimmed content under a `## Title` heading, stopping
// at the next `##` heading or end of file. Placeholder comments are excluded.
func extractSection(body, title string) string {
	lines := strings.Split(body, "\n")
	heading := "## " + title
	inside := false
	var collected []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, strings.ToLower(heading)) || strings.EqualFold(trimmed, heading) {
			inside = true
			continue
		}
		if inside {
			// Stop at the next ## heading.
			if strings.HasPrefix(trimmed, "## ") {
				break
			}
			// Skip placeholder comments.
			if strings.HasPrefix(trimmed, "<!--") {
				continue
			}
			collected = append(collected, line)
		}
	}
	return strings.TrimSpace(strings.Join(collected, "\n"))
}
