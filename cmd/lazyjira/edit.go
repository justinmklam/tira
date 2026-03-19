package main

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justinmklam/lazyjira/internal/api"
	"github.com/justinmklam/lazyjira/internal/models"
	"github.com/justinmklam/lazyjira/internal/validator"
)

// editFetchedMsg is sent when the issue + valid values have been fetched.
type editFetchedMsg struct {
	issue *models.Issue
	valid *models.ValidValues
	err   error
}

// editSaveDoneMsg is sent when the API update call completes.
type editSaveDoneMsg struct {
	err error
}

// editFormState holds field values extracted from editModel for saving.
type editFormState struct {
	summary            string
	issueType          string
	priority           string
	assignee           string // display name (editable)
	origAssignee       string // display name at form open
	origAssigneeID     string // AccountID at form open — reused when name unchanged
	storyPoints        string
	labels             string
	description        string
	acceptanceCriteria string
}

func (s editFormState) toIssueFields(valid *models.ValidValues) models.IssueFields {
	var sp float64
	if v, err := strconv.ParseFloat(strings.TrimSpace(s.storyPoints), 64); err == nil {
		sp = v
	}
	var labels []string
	for _, l := range strings.Split(s.labels, ",") {
		if l = strings.TrimSpace(l); l != "" {
			labels = append(labels, l)
		}
	}
	fields := models.IssueFields{
		Summary:            strings.TrimSpace(s.summary),
		IssueType:          s.issueType,
		Priority:           s.priority,
		Assignee:           strings.TrimSpace(s.assignee),
		StoryPoints:        sp,
		Labels:             labels,
		Description:        strings.TrimSpace(s.description),
		AcceptanceCriteria: strings.TrimSpace(s.acceptanceCriteria),
	}
	// Reuse original AccountID when the display name is unchanged; otherwise
	// fall back to a lookup (returns empty when no assignee list was loaded).
	if strings.EqualFold(fields.Assignee, s.origAssignee) {
		fields.AssigneeID = s.origAssigneeID
	} else {
		fields.AssigneeID = validator.ResolveAssigneeID(&fields, valid)
	}
	return fields
}

// fetchEditDataCmd fetches the full issue and issue metadata concurrently.
func fetchEditDataCmd(client api.Client, key string) tea.Cmd {
	projectKey := key
	if idx := strings.Index(key, "-"); idx > 0 {
		projectKey = key[:idx]
	}
	return func() tea.Msg {
		issue, err := client.GetIssue(key)
		if err != nil {
			return editFetchedMsg{err: err}
		}
		valid, err := client.GetIssueMetadata(projectKey)
		if err != nil {
			valid = &models.ValidValues{} // fall back: no fuzzy options
		}
		return editFetchedMsg{issue: issue, valid: valid}
	}
}

// saveEditCmd calls UpdateIssue and returns the result.
func saveEditCmd(client api.Client, key string, fields models.IssueFields) tea.Cmd {
	return func() tea.Msg {
		return editSaveDoneMsg{err: client.UpdateIssue(key, fields)}
	}
}

// createFetchedMsg is sent when valid values have been fetched for issue creation.
type createFetchedMsg struct {
	valid *models.ValidValues
	err   error
}

// createSaveDoneMsg is sent when the create API call completes.
type createSaveDoneMsg struct {
	issue *models.Issue
	err   error
}

// fetchCreateDataCmd fetches issue metadata (types, priorities) for the create form.
func fetchCreateDataCmd(client api.Client, projectKey string) tea.Cmd {
	return func() tea.Msg {
		valid, err := client.GetIssueMetadata(projectKey)
		if err != nil {
			return createFetchedMsg{valid: &models.ValidValues{}}
		}
		return createFetchedMsg{valid: valid}
	}
}

// saveCreateCmd creates the issue and optionally moves it to a sprint.
func saveCreateCmd(client api.Client, projectKey string, fields models.IssueFields, sprintID int) tea.Cmd {
	return func() tea.Msg {
		issue, err := client.CreateIssue(projectKey, fields)
		if err != nil {
			return createSaveDoneMsg{err: err}
		}
		if sprintID != 0 {
			err = client.MoveIssuesToSprint(sprintID, []string{issue.Key})
		}
		return createSaveDoneMsg{issue: issue, err: err}
	}
}

// blankIssueFromValid returns a blank issue pre-filled with default type and priority.
func blankIssueFromValid(valid *models.ValidValues) *models.Issue {
	blank := &models.Issue{}
	if len(valid.IssueTypes) > 0 {
		blank.IssueType = valid.IssueTypes[0]
	}
	if len(valid.Priorities) > 0 {
		blank.Priority = valid.Priorities[len(valid.Priorities)/2]
	}
	return blank
}
