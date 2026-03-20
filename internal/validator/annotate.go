package validator

import (
	"strings"
)

// AnnotateTemplate rewrites content by inserting an ERROR comment directly
// above the offending field line, replacing any existing hint comment.
// This prevents comment stacking across multiple failed saves.
func AnnotateTemplate(content string, errs []ValidationError) string {
	// Build a map of field → error message for quick lookup.
	errMap := make(map[string]string, len(errs))
	for _, e := range errs {
		errMap[e.Field] = e.Message
	}

	lines := strings.Split(content, "\n")
	var out []string

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Detect a "key: value" line (not a comment, not blank, not sentinel).
		if strings.HasPrefix(trimmed, "<!--") || trimmed == "" {
			// If this is a hint comment for a field that has an error, drop it
			// so the ERROR comment we'll add below replaces it cleanly.
			if isHintComment(trimmed) {
				fieldName := hintCommentField(lines, i+1)
				if _, hasErr := errMap[fieldName]; hasErr {
					// Skip this comment line — it will be replaced.
					continue
				}
			}
			out = append(out, line)
			continue
		}

		// Check if this line is a "field: value" line with an error.
		if field, ok := fieldKey(trimmed); ok {
			if msg, hasErr := errMap[field]; hasErr {
				out = append(out, "<!-- ERROR: "+msg+" -->")
			}
		}
		out = append(out, line)
	}

	return strings.Join(out, "\n")
}

// fieldKey returns the key name if line looks like "key: ..." and the key is
// one of the known editable fields.
func fieldKey(line string) (string, bool) {
	known := []string{"type", "priority", "assignee", "story_points", "labels"}
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", false
	}
	key := strings.TrimSpace(line[:idx])
	for _, k := range known {
		if key == k {
			return key, true
		}
	}
	return "", false
}

// isHintComment returns true for comment lines that are hint or error comments
// (i.e. lines we might want to replace).
func isHintComment(line string) bool {
	if !strings.HasPrefix(line, "<!--") {
		return false
	}
	// Sentinel line is never a hint.
	if strings.Contains(line, "tira:") {
		return false
	}
	return true
}

// hintCommentField peeks at the next non-blank line after a comment to find
// out which field it annotates. Returns the field key or "".
func hintCommentField(lines []string, nextIdx int) string {
	for i := nextIdx; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "<!--") {
			// Another comment — keep looking.
			continue
		}
		key, ok := fieldKey(trimmed)
		if ok {
			return key
		}
		return ""
	}
	return ""
}
