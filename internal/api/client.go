package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	jira "github.com/andygrunwald/go-jira/v2/cloud"
	"github.com/justinmklam/lazyjira/internal/config"
	"github.com/justinmklam/lazyjira/internal/models"
)

type Client interface {
	GetIssue(key string) (*models.Issue, error)
	UpdateIssue(key string, fields models.IssueFields) error
	CreateIssue(projectKey string, fields models.IssueFields) (*models.Issue, error)
	GetValidValues(projectKey string) (*models.ValidValues, error)
	GetActiveSprint(boardID int) ([]models.Issue, error)
	GetBacklog(projectKey string) ([]models.Sprint, error)
}

type jiraClient struct {
	client  *jira.Client
	baseURL string
	http    *http.Client
}

func NewClient(cfg *config.Config) (Client, error) {
	tp := jira.BasicAuthTransport{
		Username: cfg.Email,
		APIToken: cfg.Token,
	}
	httpClient := tp.Client()
	c, err := jira.NewClient(cfg.JiraURL, httpClient)
	if err != nil {
		return nil, fmt.Errorf("creating jira client: %w", err)
	}
	return &jiraClient{
		client:  c,
		baseURL: strings.TrimRight(cfg.JiraURL, "/"),
		http:    httpClient,
	}, nil
}

func (c *jiraClient) GetIssue(key string) (*models.Issue, error) {
	// Use go-jira for structured fields.
	issue, _, err := c.client.Issue.Get(context.Background(), key, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching issue %s: %w", key, err)
	}

	result := &models.Issue{
		Key:       issue.Key,
		Summary:   issue.Fields.Summary,
		Status:    issue.Fields.Status.Name,
		IssueType: issue.Fields.Type.Name,
		Priority:  issue.Fields.Priority.Name,
	}

	if issue.Fields.Assignee != nil {
		result.Assignee = issue.Fields.Assignee.DisplayName
		result.AssigneeID = issue.Fields.Assignee.AccountID
	}
	if issue.Fields.Reporter != nil {
		result.Reporter = issue.Fields.Reporter.DisplayName
	}
	if issue.Fields.Parent != nil {
		result.ParentKey = issue.Fields.Parent.Key
	}

	for _, l := range issue.Fields.Labels {
		result.Labels = append(result.Labels, l)
	}

	for _, link := range issue.Fields.IssueLinks {
		if link.OutwardIssue != nil {
			li := models.LinkedIssue{
				Relationship: link.Type.Outward,
				Key:          link.OutwardIssue.Key,
			}
			if link.OutwardIssue.Fields != nil {
				li.Summary = link.OutwardIssue.Fields.Summary
				li.Status = link.OutwardIssue.Fields.Status.Name
			}
			result.LinkedIssues = append(result.LinkedIssues, li)
		}
		if link.InwardIssue != nil {
			li := models.LinkedIssue{
				Relationship: link.Type.Inward,
				Key:          link.InwardIssue.Key,
			}
			if link.InwardIssue.Fields != nil {
				li.Summary = link.InwardIssue.Fields.Summary
				li.Status = link.InwardIssue.Fields.Status.Name
			}
			result.LinkedIssues = append(result.LinkedIssues, li)
		}
	}

	// Fetch ADF fields (description, acceptance criteria) and sprint via raw
	// HTTP: go-jira can't decode ADF objects into strings and sprint is a
	// custom field. ?expand=names gives us the customfield_* → display name map.
	if err := c.fetchRawFields(key, result); err == nil {
		// non-fatal — display whatever we got from go-jira
	}

	return result, nil
}

// rawIssueResponse is the minimal shape we need from /rest/api/3/issue/{key}?expand=names.
type rawIssueResponse struct {
	Fields map[string]json.RawMessage `json:"fields"`
	Names  map[string]string          `json:"names"`
}

// fetchRawFields populates ADF-based fields (description, acceptance criteria)
// and the sprint name by parsing the raw Jira API response.
func (c *jiraClient) fetchRawFields(key string, result *models.Issue) error {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s?expand=names", c.baseURL, key)
	resp, err := c.http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var raw rawIssueResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}

	// Build a reverse map: lowercase display name → field ID.
	nameToID := make(map[string]string, len(raw.Names))
	for id, name := range raw.Names {
		nameToID[strings.ToLower(name)] = id
	}

	// Description
	if fieldID, ok := nameToID["description"]; ok {
		result.Description = c.extractADF(raw.Fields, fieldID)
	} else {
		result.Description = c.extractADF(raw.Fields, "description")
	}

	// Acceptance Criteria (field name varies by instance)
	for _, candidate := range []string{"acceptance criteria", "acceptance criterion"} {
		if fieldID, ok := nameToID[candidate]; ok {
			result.AcceptanceCriteria = c.extractADF(raw.Fields, fieldID)
			break
		}
	}

	// Sprint — stored as an array of sprint objects in a custom field
	if fieldID, ok := nameToID["sprint"]; ok {
		result.SprintName = c.extractSprintName(raw.Fields, fieldID)
	}

	// Parent summary (go-jira only gives us the key)
	if result.ParentKey != "" {
		if fieldID, ok := nameToID["parent"]; ok {
			result.ParentSummary = c.extractParentSummary(raw.Fields, fieldID)
		}
	}

	return nil
}

func (c *jiraClient) extractADF(fields map[string]json.RawMessage, fieldID string) string {
	raw, ok := fields[fieldID]
	if !ok || string(raw) == "null" {
		return ""
	}
	var adf map[string]any
	if err := json.Unmarshal(raw, &adf); err != nil {
		return ""
	}
	return ADFToMarkdown(adf)
}

func (c *jiraClient) extractSprintName(fields map[string]json.RawMessage, fieldID string) string {
	raw, ok := fields[fieldID]
	if !ok || string(raw) == "null" {
		return ""
	}
	// Sprint field is an array; pick the last active/future sprint.
	var sprints []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(raw, &sprints); err != nil {
		return ""
	}
	// Return the last entry (most recent sprint assignment).
	if len(sprints) > 0 {
		return sprints[len(sprints)-1].Name
	}
	return ""
}

func (c *jiraClient) extractParentSummary(fields map[string]json.RawMessage, fieldID string) string {
	raw, ok := fields[fieldID]
	if !ok || string(raw) == "null" {
		return ""
	}
	var parent struct {
		Fields struct {
			Summary string `json:"summary"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(raw, &parent); err != nil {
		return ""
	}
	return parent.Fields.Summary
}

func (c *jiraClient) UpdateIssue(key string, fields models.IssueFields) error {
	return fmt.Errorf("not implemented")
}

func (c *jiraClient) CreateIssue(projectKey string, fields models.IssueFields) (*models.Issue, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *jiraClient) GetValidValues(projectKey string) (*models.ValidValues, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *jiraClient) GetActiveSprint(boardID int) ([]models.Issue, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *jiraClient) GetBacklog(projectKey string) ([]models.Sprint, error) {
	return nil, fmt.Errorf("not implemented")
}
