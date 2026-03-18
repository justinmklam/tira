package validator

import (
	"fmt"
	"strings"

	"github.com/justinmklam/lazyjira/internal/models"
)

// ValidationError describes a single field validation failure.
type ValidationError struct {
	Field   string
	Value   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validate checks fields against the allowed values in valid and returns any
// errors found. Enumerable fields use case-insensitive matching.
func Validate(fields *models.IssueFields, valid *models.ValidValues) []ValidationError {
	var errs []ValidationError

	if fields.IssueType != "" && len(valid.IssueTypes) > 0 {
		if !containsCI(valid.IssueTypes, fields.IssueType) {
			errs = append(errs, ValidationError{
				Field:   "type",
				Value:   fields.IssueType,
				Message: fmt.Sprintf("%q is not a valid type. Valid: %s", fields.IssueType, strings.Join(valid.IssueTypes, ", ")),
			})
		}
	}

	if fields.Priority != "" && len(valid.Priorities) > 0 {
		if !containsCI(valid.Priorities, fields.Priority) {
			errs = append(errs, ValidationError{
				Field:   "priority",
				Value:   fields.Priority,
				Message: fmt.Sprintf("%q is not a valid priority. Valid: %s", fields.Priority, strings.Join(valid.Priorities, ", ")),
			})
		}
	}

	if fields.Assignee != "" && len(valid.Assignees) > 0 {
		names := make([]string, len(valid.Assignees))
		for i, a := range valid.Assignees {
			names[i] = a.DisplayName
		}
		if !containsCI(names, fields.Assignee) {
			errs = append(errs, ValidationError{
				Field:   "assignee",
				Value:   fields.Assignee,
				Message: fmt.Sprintf("%q is not a valid assignee. Valid: %s", fields.Assignee, strings.Join(names, ", ")),
			})
		}
	}

	if fields.StoryPoints < 0 {
		errs = append(errs, ValidationError{
			Field:   "story_points",
			Value:   fmt.Sprintf("%g", fields.StoryPoints),
			Message: "story points must be a non-negative number or blank",
		})
	}

	return errs
}

// ResolveAssigneeID looks up the AccountID for the display name in fields.Assignee.
// Returns the AccountID if found, or empty string if no match.
func ResolveAssigneeID(fields *models.IssueFields, valid *models.ValidValues) string {
	if fields.Assignee == "" {
		return ""
	}
	for _, a := range valid.Assignees {
		if strings.EqualFold(a.DisplayName, fields.Assignee) {
			return a.AccountID
		}
	}
	return ""
}

func containsCI(list []string, val string) bool {
	for _, item := range list {
		if strings.EqualFold(item, val) {
			return true
		}
	}
	return false
}
