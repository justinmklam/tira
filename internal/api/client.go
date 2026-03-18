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
	GetBoardColumns(boardID int) ([]models.BoardColumn, error)
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
	payload := map[string]any{
		"fields": map[string]any{},
	}
	f := payload["fields"].(map[string]any)
	if fields.Summary != "" {
		f["summary"] = fields.Summary
	}
	if fields.IssueType != "" {
		f["issuetype"] = map[string]any{"name": fields.IssueType}
	}
	if fields.Priority != "" {
		f["priority"] = map[string]any{"name": fields.Priority}
	}
	if fields.AssigneeID != "" {
		f["assignee"] = map[string]any{"accountId": fields.AssigneeID}
	}
	if fields.Description != "" {
		f["description"] = markdownToADF(fields.Description)
	}
	if fields.StoryPoints > 0 {
		f["story_points"] = fields.StoryPoints
	}
	if len(fields.Labels) > 0 {
		f["labels"] = fields.Labels
	}

	url := fmt.Sprintf("%s/rest/api/3/issue/%s", c.baseURL, key)
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update issue %s: HTTP %d: %s", key, resp.StatusCode, string(b))
	}
	return nil
}

// markdownToADF wraps plain text/markdown as a minimal ADF paragraph doc.
// For now we send the description as a plain-text paragraph block.
func markdownToADF(text string) map[string]any {
	return map[string]any{
		"version": 1,
		"type":    "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type": "text",
						"text": text,
					},
				},
			},
		},
	}
}

func (c *jiraClient) CreateIssue(projectKey string, fields models.IssueFields) (*models.Issue, error) {
	f := map[string]any{
		"project":   map[string]any{"key": projectKey},
		"summary":   fields.Summary,
		"issuetype": map[string]any{"name": fields.IssueType},
	}
	if fields.Priority != "" {
		f["priority"] = map[string]any{"name": fields.Priority}
	}
	if fields.AssigneeID != "" {
		f["assignee"] = map[string]any{"accountId": fields.AssigneeID}
	}
	if fields.Description != "" {
		f["description"] = markdownToADF(fields.Description)
	}
	if fields.StoryPoints > 0 {
		f["story_points"] = fields.StoryPoints
	}
	if len(fields.Labels) > 0 {
		f["labels"] = fields.Labels
	}
	if fields.ParentKey != "" {
		f["parent"] = map[string]any{"key": fields.ParentKey}
	}

	payload := map[string]any{"fields": f}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/rest/api/3/issue", c.baseURL)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("create issue: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var created struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil {
		return nil, fmt.Errorf("parsing create response: %w", err)
	}

	return &models.Issue{Key: created.Key}, nil
}

func (c *jiraClient) GetValidValues(projectKey string) (*models.ValidValues, error) {
	valid := &models.ValidValues{}

	// Issue types
	url := fmt.Sprintf("%s/rest/api/3/project/%s/statuses", c.baseURL, projectKey)
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var statusList []struct {
		Name     string `json:"name"`
		Subtask  bool   `json:"subtask"`
		Statuses []struct {
			Name string `json:"name"`
		} `json:"statuses"`
	}
	if body, err := io.ReadAll(resp.Body); err == nil {
		if err2 := json.Unmarshal(body, &statusList); err2 == nil {
			seen := make(map[string]bool)
			for _, t := range statusList {
				if !seen[t.Name] {
					valid.IssueTypes = append(valid.IssueTypes, t.Name)
					seen[t.Name] = true
				}
			}
		}
	}

	// Priorities
	prioURL := fmt.Sprintf("%s/rest/api/3/priority", c.baseURL)
	if prioResp, err := c.http.Get(prioURL); err == nil {
		defer prioResp.Body.Close()
		var priorities []struct {
			Name string `json:"name"`
		}
		if body, err := io.ReadAll(prioResp.Body); err == nil {
			if err2 := json.Unmarshal(body, &priorities); err2 == nil {
				for _, p := range priorities {
					valid.Priorities = append(valid.Priorities, p.Name)
				}
			}
		}
	}

	// Assignees
	assigneeURL := fmt.Sprintf("%s/rest/api/3/user/assignable/search?project=%s&maxResults=50", c.baseURL, projectKey)
	if aResp, err := c.http.Get(assigneeURL); err == nil {
		defer aResp.Body.Close()
		var assignees []struct {
			DisplayName string `json:"displayName"`
			AccountID   string `json:"accountId"`
		}
		if body, err := io.ReadAll(aResp.Body); err == nil {
			if err2 := json.Unmarshal(body, &assignees); err2 == nil {
				for _, a := range assignees {
					valid.Assignees = append(valid.Assignees, models.Assignee{
						DisplayName: a.DisplayName,
						AccountID:   a.AccountID,
					})
				}
			}
		}
	}

	return valid, nil
}

func (c *jiraClient) GetActiveSprint(boardID int) ([]models.Issue, error) {
	// Step 1: find the active sprint for the board.
	sprintURL := fmt.Sprintf("%s/rest/agile/1.0/board/%d/sprint?state=active", c.baseURL, boardID)
	resp, err := c.http.Get(sprintURL)
	if err != nil {
		return nil, fmt.Errorf("fetching active sprint: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("active sprint: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var sprintResp struct {
		Values []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"values"`
	}
	if err := json.Unmarshal(body, &sprintResp); err != nil {
		return nil, fmt.Errorf("parsing sprint response: %w", err)
	}
	if len(sprintResp.Values) == 0 {
		return nil, fmt.Errorf("no active sprint found for board %d", boardID)
	}
	sprint := sprintResp.Values[0]

	// Step 2: fetch issues in that sprint.
	issueURL := fmt.Sprintf(
		"%s/rest/agile/1.0/sprint/%d/issue?maxResults=200&fields=summary,status,issuetype,priority,assignee,labels",
		c.baseURL, sprint.ID,
	)
	issueResp, err := c.http.Get(issueURL)
	if err != nil {
		return nil, fmt.Errorf("fetching sprint issues: %w", err)
	}
	defer issueResp.Body.Close()
	issueBody, err := io.ReadAll(issueResp.Body)
	if err != nil {
		return nil, err
	}
	if issueResp.StatusCode >= 300 {
		return nil, fmt.Errorf("sprint issues: HTTP %d: %s", issueResp.StatusCode, string(issueBody))
	}

	var issueData struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary  string `json:"summary"`
				Status   struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"status"`
				Issuetype struct {
					Name string `json:"name"`
				} `json:"issuetype"`
				Priority struct {
					Name string `json:"name"`
				} `json:"priority"`
				Assignee *struct {
					DisplayName string `json:"displayName"`
					AccountID   string `json:"accountId"`
				} `json:"assignee"`
				Labels []string `json:"labels"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(issueBody, &issueData); err != nil {
		return nil, fmt.Errorf("parsing sprint issues: %w", err)
	}

	issues := make([]models.Issue, 0, len(issueData.Issues))
	for _, raw := range issueData.Issues {
		issue := models.Issue{
			Key:        raw.Key,
			Summary:    raw.Fields.Summary,
			Status:     raw.Fields.Status.Name,
			StatusID:   raw.Fields.Status.ID,
			IssueType:  raw.Fields.Issuetype.Name,
			Priority:   raw.Fields.Priority.Name,
			SprintName: sprint.Name,
			Labels:     raw.Fields.Labels,
		}
		if raw.Fields.Assignee != nil {
			issue.Assignee = raw.Fields.Assignee.DisplayName
			issue.AssigneeID = raw.Fields.Assignee.AccountID
		}
		issues = append(issues, issue)
	}
	return issues, nil
}

func (c *jiraClient) GetBoardColumns(boardID int) ([]models.BoardColumn, error) {
	url := fmt.Sprintf("%s/rest/agile/1.0/board/%d/configuration", c.baseURL, boardID)
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching board configuration: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("board configuration: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var config struct {
		ColumnConfig struct {
			Columns []struct {
				Name     string `json:"name"`
				Statuses []struct {
					ID string `json:"id"`
				} `json:"statuses"`
			} `json:"columns"`
		} `json:"columnConfig"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, fmt.Errorf("parsing board configuration: %w", err)
	}

	cols := make([]models.BoardColumn, len(config.ColumnConfig.Columns))
	for i, col := range config.ColumnConfig.Columns {
		ids := make([]string, len(col.Statuses))
		for j, s := range col.Statuses {
			ids[j] = s.ID
		}
		cols[i] = models.BoardColumn{Name: col.Name, StatusIDs: ids}
	}
	return cols, nil
}

func (c *jiraClient) GetBacklog(projectKey string) ([]models.Sprint, error) {
	return nil, fmt.Errorf("not implemented")
}
