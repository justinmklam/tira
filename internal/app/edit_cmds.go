package app

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/debug"
	"github.com/justinmklam/tira/internal/models"
	"github.com/justinmklam/tira/internal/tui"
	"github.com/justinmklam/tira/internal/validator"
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
			debug.LogError("client.GetIssue", err)
			return editFetchedMsg{err: err}
		}
		valid, err := client.GetIssueMetadata(projectKey)
		if err != nil {
			debug.LogWarning("client.GetIssueMetadata", err.Error())
			valid = &models.ValidValues{} // fall back: no fuzzy options
		}
		return editFetchedMsg{issue: issue, valid: valid}
	}
}

// saveEditCmd calls UpdateIssue and returns the result.
func saveEditCmd(client api.Client, key string, fields models.IssueFields) tea.Cmd {
	return func() tea.Msg {
		err := client.UpdateIssue(key, fields)
		if err != nil {
			debug.LogError("client.UpdateIssue", err)
		}
		return editSaveDoneMsg{err: err}
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
			debug.LogWarning("client.GetIssueMetadata", err.Error())
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
			debug.LogError("client.CreateIssue", err)
			return createSaveDoneMsg{err: err}
		}
		if sprintID != 0 {
			err = client.MoveIssuesToSprint(sprintID, []string{issue.Key})
			if err != nil {
				debug.LogError("client.MoveIssuesToSprint", err)
			}
		}
		return createSaveDoneMsg{issue: issue, err: err}
	}
}

// newTypePicker builds an OptionPickerModel for selecting an issue type.
func newTypePicker(typeOpts []string, initialValue string) tui.OptionPickerModel {
	return tui.NewOptionPickerModel(typeOpts, initialValue)
}

// newPriorityPicker builds an OptionPickerModel for selecting a priority.
func newPriorityPicker(priorityOpts []string, initialValue string) tui.OptionPickerModel {
	return tui.NewOptionPickerModel(priorityOpts, initialValue)
}

// newAssigneePicker builds a PickerModel backed by a debounced assignee search.
// projectKey may be derived from an issue key (e.g. "PROJ-1" → "PROJ").
func newAssigneePicker(client api.Client, projectKey string) tui.PickerModel {
	search := func(query string) ([]tui.PickerItem, error) {
		assignees, err := client.SearchAssignees(projectKey, query)
		if err != nil {
			debug.LogError("client.SearchAssignees", err)
			return nil, err
		}
		items := make([]tui.PickerItem, len(assignees))
		for i, a := range assignees {
			items[i] = tui.PickerItem{
				Label:    a.DisplayName,
				SubLabel: a.AccountID,
				Value:    a.AccountID,
			}
		}
		return items, nil
	}
	m := tui.NewPickerModel(search)
	m.NoneItem = &tui.PickerItem{Label: "(none)", SubLabel: "unassign"}
	return m
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
