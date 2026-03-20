package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	jira "github.com/andygrunwald/go-jira/v2/cloud"
	"github.com/justinmklam/lazyjira/internal/config"
	"github.com/justinmklam/lazyjira/internal/debug"
	"github.com/justinmklam/lazyjira/internal/models"
)

type Client interface {
	GetIssue(key string) (*models.Issue, error)
	UpdateIssue(key string, fields models.IssueFields) error
	CreateIssue(projectKey string, fields models.IssueFields) (*models.Issue, error)
	GetValidValues(projectKey string) (*models.ValidValues, error)
	// GetIssueMetadata returns issue types and priorities only (no assignee lookup).
	GetIssueMetadata(projectKey string) (*models.ValidValues, error)
	GetBoardColumns(boardID int) ([]models.BoardColumn, error)
	GetActiveSprint(boardID int) ([]models.Issue, error)
	GetSprintGroups(boardID int) ([]models.SprintGroup, error)
	GetBacklog(projectKey string) ([]models.Sprint, error)
	GetEpics(projectKey, query string) ([]models.Issue, error)
	MoveIssuesToSprint(sprintID int, keys []string) error
	MoveIssuesToBacklog(keys []string) error
	RankIssues(keys []string, rankAfterKey, rankBeforeKey string) error
	SetParent(issueKey, parentKey string) error
	SearchAssignees(projectKey, query string) ([]models.Assignee, error)
	SetAssignee(issueKey, accountID string) error
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

	// Wrap with debug transport if debug mode is enabled
	if debug.IsEnabled() {
		httpClient.Transport = &debug.Transport{Base: httpClient.Transport}
	}

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
	// Fetch structured+ADF fields and comments concurrently.
	// fetchFullIssue replaces the previous two-step approach (go-jira Issue.Get
	// followed by a separate fetchRawFields call to the same endpoint) with a
	// single raw HTTP request, halving the number of serial API calls.
	var (
		result   *models.Issue
		comments []models.Comment
		fetchErr error
		wg       sync.WaitGroup
	)
	wg.Add(3)
	go func() {
		defer wg.Done()
		result, fetchErr = c.fetchFullIssue(key)
	}()
	go func() {
		defer wg.Done()
		if c, err := c.fetchComments(key); err == nil {
			comments = c
		}
	}()
	go func() {
		defer wg.Done()
		if date, err := c.fetchStatusChangeDate(key); err == nil {
			if result != nil {
				result.StatusChangedDate = date
			}
		}
	}()
	wg.Wait()
	if fetchErr != nil {
		return nil, fetchErr
	}
	result.Comments = comments
	return result, nil
}

// fetchFullIssue fetches a single issue using one raw HTTP request to
// /rest/api/3/issue/{key}?expand=names and parses both structured fields and
// ADF custom fields from it.  This replaces the previous two-step approach
// that called go-jira Issue.Get and then made a second request to the same
// endpoint to obtain field names and ADF content.
func (c *jiraClient) fetchFullIssue(key string) (*models.Issue, error) {
	rawURL := fmt.Sprintf("%s/rest/api/3/issue/%s?expand=names", c.baseURL, key)
	resp, err := c.http.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("fetching issue %s: %w", key, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetching issue %s: HTTP %d", key, resp.StatusCode)
	}

	// Outer envelope: keep Fields as raw bytes so we can unmarshal it twice —
	// once into a typed struct for standard fields, once into a raw map for
	// custom/ADF fields.
	var envelope struct {
		Key    string            `json:"key"`
		Fields json.RawMessage   `json:"fields"`
		Names  map[string]string `json:"names"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}

	// Structured standard fields.
	var sf struct {
		Summary string `json:"summary"`
		Status  struct {
			Name string `json:"name"`
		} `json:"status"`
		IssueType struct {
			Name string `json:"name"`
		} `json:"issuetype"`
		Priority *struct {
			Name string `json:"name"`
		} `json:"priority"`
		Assignee *struct {
			DisplayName string `json:"displayName"`
			AccountID   string `json:"accountId"`
		} `json:"assignee"`
		Reporter *struct {
			DisplayName string `json:"displayName"`
		} `json:"reporter"`
		Parent *struct {
			Key string `json:"key"`
		} `json:"parent"`
		Labels     []string `json:"labels"`
		IssueLinks []struct {
			Type struct {
				Outward string `json:"outward"`
				Inward  string `json:"inward"`
			} `json:"type"`
			OutwardIssue *struct {
				Key    string `json:"key"`
				Fields *struct {
					Summary string `json:"summary"`
					Status  struct {
						Name string `json:"name"`
					} `json:"status"`
				} `json:"fields"`
			} `json:"outwardIssue"`
			InwardIssue *struct {
				Key    string `json:"key"`
				Fields *struct {
					Summary string `json:"summary"`
					Status  struct {
						Name string `json:"name"`
					} `json:"status"`
				} `json:"fields"`
			} `json:"inwardIssue"`
		} `json:"issuelinks"`
	}
	if err := json.Unmarshal(envelope.Fields, &sf); err != nil {
		return nil, err
	}

	result := &models.Issue{
		Key:       envelope.Key,
		Summary:   sf.Summary,
		Status:    sf.Status.Name,
		IssueType: sf.IssueType.Name,
		Labels:    sf.Labels,
	}
	if sf.Priority != nil {
		result.Priority = sf.Priority.Name
	}
	if sf.Assignee != nil {
		result.Assignee = sf.Assignee.DisplayName
		result.AssigneeID = sf.Assignee.AccountID
	}
	if sf.Reporter != nil {
		result.Reporter = sf.Reporter.DisplayName
	}
	if sf.Parent != nil {
		result.ParentKey = sf.Parent.Key
	}
	for _, link := range sf.IssueLinks {
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

	// Raw field map for custom/ADF fields.
	var rawFields map[string]json.RawMessage
	json.Unmarshal(envelope.Fields, &rawFields) //nolint:errcheck // best effort

	// Build name → field ID map.
	nameToID := make(map[string]string, len(envelope.Names))
	for id, name := range envelope.Names {
		nameToID[strings.ToLower(name)] = id
	}

	// Description (ADF).
	if fieldID, ok := nameToID["description"]; ok {
		result.Description = c.extractADF(rawFields, fieldID)
	} else {
		result.Description = c.extractADF(rawFields, "description")
	}

	// Acceptance Criteria (field name varies by Jira instance).
	for _, candidate := range []string{"acceptance criteria", "acceptance criterion"} {
		if fieldID, ok := nameToID[candidate]; ok {
			result.AcceptanceCriteria = c.extractADF(rawFields, fieldID)
			break
		}
	}

	// Sprint name.
	if fieldID, ok := nameToID["sprint"]; ok {
		result.SprintName = c.extractSprintName(rawFields, fieldID)
	}

	// Story points.
	for _, candidate := range []string{"story points", "story point estimate"} {
		if fieldID, ok := nameToID[candidate]; ok {
			if raw, ok := rawFields[fieldID]; ok {
				var sp float64
				if json.Unmarshal(raw, &sp) == nil {
					result.StoryPoints = sp
				}
			}
			break
		}
	}

	// Parent summary.
	if result.ParentKey != "" {
		if fieldID, ok := nameToID["parent"]; ok {
			result.ParentSummary = c.extractParentSummary(rawFields, fieldID)
		}
	}

	return result, nil
}

func (c *jiraClient) fetchComments(key string) ([]models.Comment, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s/comment?maxResults=50&orderBy=-created", c.baseURL, key)
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("comments: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Comments []struct {
			Author struct {
				DisplayName string `json:"displayName"`
			} `json:"author"`
			Body    json.RawMessage `json:"body"`
			Created string          `json:"created"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	comments := make([]models.Comment, 0, len(result.Comments))
	for _, c := range result.Comments {
		var adf map[string]any
		body := ""
		if err := json.Unmarshal(c.Body, &adf); err == nil {
			body = ADFToMarkdown(adf)
		}
		comments = append(comments, models.Comment{
			Author:  c.Author.DisplayName,
			Body:    body,
			Created: c.Created,
		})
	}
	return comments, nil
}

// fetchStatusChangeDate fetches the changelog for an issue and returns the date
// when the status last changed. Returns empty string if no status change found.
func (c *jiraClient) fetchStatusChangeDate(key string) (string, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s/changelog?maxResults=100", c.baseURL, key)
	resp, err := c.http.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("changelog: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Values []struct {
			Created string `json:"created"`
			Items   []struct {
				Field      string `json:"field"`
				FromString string `json:"fromString"`
				ToString   string `json:"toString"`
			} `json:"items"`
		} `json:"values"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	// Find the most recent status change (last entry in changelog that changed status).
	for _, v := range result.Values {
		for _, item := range v.Items {
			if item.Field == "status" {
				// Extract date portion (ISO 8601 format: "2026-03-01T10:30:00.000+0000")
				if len(v.Created) >= 10 {
					return v.Created[:10], nil
				}
				return v.Created, nil
			}
		}
	}
	return "", nil
}

// fetchBatchStatusChangeDates fetches changelogs for multiple issues in
// parallel and returns a map of issue key to status change date.
func (c *jiraClient) fetchBatchStatusChangeDates(keys []string) map[string]string {
	result := make(map[string]string, len(keys))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, key := range keys {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			if date, err := c.fetchStatusChangeDate(k); err == nil && date != "" {
				mu.Lock()
				result[k] = date
				mu.Unlock()
			}
		}(key)
	}
	wg.Wait()
	return result
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

	// Fetch field IDs for this issue so we can resolve custom fields like
	// "Acceptance Criteria" and "Story Points".
	nameToID, _ := c.fetchFieldIDs(key)

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

	// Acceptance Criteria
	if fields.AcceptanceCriteria != "" && nameToID != nil {
		for _, candidate := range []string{"acceptance criteria", "acceptance criterion"} {
			if id, ok := nameToID[candidate]; ok {
				f[id] = markdownToADF(fields.AcceptanceCriteria)
				break
			}
		}
	}

	// Story Points
	if fields.StoryPoints > 0 {
		if id, ok := nameToID["story points"]; ok {
			f[id] = fields.StoryPoints
		} else if id, ok := nameToID["story point estimate"]; ok {
			f[id] = fields.StoryPoints
		} else {
			f["story_points"] = fields.StoryPoints // fallback
		}
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
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update issue %s: HTTP %d: %s", key, resp.StatusCode, string(b))
	}
	return nil
}

func (c *jiraClient) fetchFieldIDs(key string) (map[string]string, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s?expand=names", c.baseURL, key)
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var raw struct {
		Names map[string]string `json:"names"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	nameToID := make(map[string]string, len(raw.Names))
	for id, name := range raw.Names {
		nameToID[strings.ToLower(name)] = id
	}
	return nameToID, nil
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

	// Fetch all field IDs so we can resolve custom fields like
	// "Acceptance Criteria" and "Story Points".
	nameToID, _ := c.fetchAllFieldIDs()

	if fields.Priority != "" {
		f["priority"] = map[string]any{"name": fields.Priority}
	}
	if fields.AssigneeID != "" {
		f["assignee"] = map[string]any{"accountId": fields.AssigneeID}
	}
	if fields.Description != "" {
		f["description"] = markdownToADF(fields.Description)
	}

	// Acceptance Criteria
	if fields.AcceptanceCriteria != "" && nameToID != nil {
		for _, candidate := range []string{"acceptance criteria", "acceptance criterion"} {
			if id, ok := nameToID[candidate]; ok {
				f[id] = markdownToADF(fields.AcceptanceCriteria)
				break
			}
		}
	}

	// Story Points
	if fields.StoryPoints > 0 {
		if id, ok := nameToID["story points"]; ok {
			f[id] = fields.StoryPoints
		} else if id, ok := nameToID["story point estimate"]; ok {
			f[id] = fields.StoryPoints
		} else {
			f["story_points"] = fields.StoryPoints // fallback
		}
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
	defer func() { _ = resp.Body.Close() }()

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

func (c *jiraClient) fetchAllFieldIDs() (map[string]string, error) {
	url := fmt.Sprintf("%s/rest/api/3/field", c.baseURL)
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var fields []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&fields); err != nil {
		return nil, err
	}

	nameToID := make(map[string]string, len(fields))
	for _, f := range fields {
		nameToID[strings.ToLower(f.Name)] = f.ID
	}
	return nameToID, nil
}

func (c *jiraClient) GetValidValues(projectKey string) (*models.ValidValues, error) {
	valid := &models.ValidValues{}

	// Issue types
	url := fmt.Sprintf("%s/rest/api/3/project/%s/statuses", c.baseURL, projectKey)
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
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
		defer func() { _ = prioResp.Body.Close() }()
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
		defer func() { _ = aResp.Body.Close() }()
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

func (c *jiraClient) GetIssueMetadata(projectKey string) (*models.ValidValues, error) {
	valid := &models.ValidValues{}

	// Issue types
	url := fmt.Sprintf("%s/rest/api/3/project/%s/statuses", c.baseURL, projectKey)
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var statusList []struct {
		Name string `json:"name"`
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
		defer func() { _ = prioResp.Body.Close() }()
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
		defer func() { _ = aResp.Body.Close() }()
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
	defer func() { _ = resp.Body.Close() }()
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
	defer func() { _ = issueResp.Body.Close() }()
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
				Summary string `json:"summary"`
				Status  struct {
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
	defer func() { _ = resp.Body.Close() }()
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

func (c *jiraClient) GetSprintGroups(boardID int) ([]models.SprintGroup, error) {
	// Fetch all active and future sprints.
	sprintURL := fmt.Sprintf("%s/rest/agile/1.0/board/%d/sprint?state=active,future&maxResults=50", c.baseURL, boardID)
	resp, err := c.http.Get(sprintURL)
	if err != nil {
		return nil, fmt.Errorf("fetching sprints: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sprints: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var sprintResp struct {
		Values []struct {
			ID        int    `json:"id"`
			Name      string `json:"name"`
			State     string `json:"state"`
			StartDate string `json:"startDate"`
			EndDate   string `json:"endDate"`
		} `json:"values"`
	}
	if err := json.Unmarshal(body, &sprintResp); err != nil {
		return nil, fmt.Errorf("parsing sprints: %w", err)
	}

	const issueFields = "summary,status,issuetype,priority,assignee,labels,parent,story_points,customfield_10016"

	// Fetch all sprint issues and the backlog concurrently.
	// Pre-allocate one slot per sprint; backlog is appended after.
	groups := make([]models.SprintGroup, len(sprintResp.Values))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i, sp := range sprintResp.Values {
		i, sp := i, sp // capture for goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			issueURL := fmt.Sprintf(
				"%s/rest/agile/1.0/sprint/%d/issue?maxResults=200&fields=%s",
				c.baseURL, sp.ID, issueFields,
			)
			issues, err := c.fetchAgileIssues(issueURL, sp.Name)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			groups[i] = models.SprintGroup{
				Sprint: models.Sprint{
					ID:        sp.ID,
					Name:      sp.Name,
					State:     sp.State,
					StartDate: trimDateStr(sp.StartDate),
					EndDate:   trimDateStr(sp.EndDate),
				},
				Issues: issues,
			}
		}()
	}

	// Backlog fetch runs concurrently with sprint fetches.
	var backlogGroup models.SprintGroup
	var hasBacklog bool
	wg.Add(1)
	go func() {
		defer wg.Done()
		backlogURL := fmt.Sprintf(
			"%s/rest/agile/1.0/board/%d/backlog?maxResults=200&fields=%s",
			c.baseURL, boardID, issueFields,
		)
		issues, err := c.fetchAgileIssues(backlogURL, "")
		if err == nil {
			mu.Lock()
			backlogGroup = models.SprintGroup{
				Sprint: models.Sprint{Name: "Backlog", State: "backlog"},
				Issues: issues,
			}
			hasBacklog = true
			mu.Unlock()
		}
	}()

	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if hasBacklog {
		groups = append(groups, backlogGroup)
	}
	return groups, nil
}

// fetchAgileIssues fetches issues from any Jira Agile API endpoint that returns
// an "issues" array (sprint issues, backlog, etc.).
func (c *jiraClient) fetchAgileIssues(url, sprintName string) ([]models.Issue, error) {
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching issues: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("issues: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var data struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
				Status  struct {
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
				// Parent is used in next-gen projects; if the parent is an Epic it
				// represents the epic link.
				Parent *struct {
					Key    string `json:"key"`
					Fields struct {
						Summary   string `json:"summary"`
						Issuetype struct {
							Name string `json:"name"`
						} `json:"issuetype"`
					} `json:"fields"`
				} `json:"parent"`
				// Story points — field ID varies by instance; try both.
				StoryPoints   *float64 `json:"story_points"`
				CustomField16 *float64 `json:"customfield_10016"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parsing issues: %w", err)
	}

	// Extract issue keys for batch status change fetch.
	keys := make([]string, 0, len(data.Issues))
	for _, raw := range data.Issues {
		keys = append(keys, raw.Key)
	}

	// Fetch status change dates in parallel.
	statusDates := c.fetchBatchStatusChangeDates(keys)

	issues := make([]models.Issue, 0, len(data.Issues))
	for _, raw := range data.Issues {
		issue := models.Issue{
			Key:        raw.Key,
			Summary:    raw.Fields.Summary,
			Status:     raw.Fields.Status.Name,
			StatusID:   raw.Fields.Status.ID,
			IssueType:  raw.Fields.Issuetype.Name,
			Priority:   raw.Fields.Priority.Name,
			SprintName: sprintName,
			Labels:     raw.Fields.Labels,
		}
		if raw.Fields.Assignee != nil {
			issue.Assignee = raw.Fields.Assignee.DisplayName
			issue.AssigneeID = raw.Fields.Assignee.AccountID
		}
		if date, ok := statusDates[raw.Key]; ok {
			issue.StatusChangedDate = date
		}
		// Epic: parent issue when the parent type is "Epic".
		if p := raw.Fields.Parent; p != nil && p.Fields.Issuetype.Name == "Epic" {
			issue.EpicKey = p.Key
			issue.EpicName = p.Fields.Summary
		}
		// Story points: prefer the direct alias, fall back to common custom field ID.
		for _, sp := range []*float64{raw.Fields.StoryPoints, raw.Fields.CustomField16} {
			if sp != nil && *sp > 0 {
				issue.StoryPoints = *sp
				break
			}
		}
		issues = append(issues, issue)
	}
	return issues, nil
}

func (c *jiraClient) GetBacklog(projectKey string) ([]models.Sprint, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *jiraClient) GetEpics(projectKey, query string) ([]models.Issue, error) {
	base := fmt.Sprintf(`project="%s" AND issuetype=Epic`, projectKey)
	if query != "" {
		base += fmt.Sprintf(` AND summary ~ "%s"`, strings.ReplaceAll(query, `"`, `\"`))
	}
	jql := url.QueryEscape(base + " ORDER BY summary ASC")
	apiURL := fmt.Sprintf("%s/rest/api/3/search/jql?jql=%s&maxResults=50&fields=summary,issuetype", c.baseURL, jql)
	resp, err := c.http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("fetching epics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("epics: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var data struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parsing epics: %w", err)
	}

	issues := make([]models.Issue, 0, len(data.Issues))
	for _, raw := range data.Issues {
		issues = append(issues, models.Issue{
			Key:       raw.Key,
			Summary:   raw.Fields.Summary,
			IssueType: "Epic",
		})
	}
	return issues, nil
}

func (c *jiraClient) SetParent(issueKey, parentKey string) error {
	var parentField any
	if parentKey != "" {
		parentField = map[string]any{"key": parentKey}
	}
	payload := map[string]any{
		"fields": map[string]any{
			"parent": parentField,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	apiURL := fmt.Sprintf("%s/rest/api/3/issue/%s", c.baseURL, issueKey)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, apiURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("set parent %s: HTTP %d: %s", issueKey, resp.StatusCode, string(b))
	}
	return nil
}

func (c *jiraClient) SearchAssignees(projectKey, query string) ([]models.Assignee, error) {
	apiURL := fmt.Sprintf("%s/rest/api/3/user/assignable/search?project=%s&maxResults=20",
		c.baseURL, url.QueryEscape(projectKey))
	if query != "" {
		apiURL += "&query=" + url.QueryEscape(query)
	}
	resp, err := c.http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("searching assignees: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("assignees: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var users []struct {
		DisplayName string `json:"displayName"`
		AccountID   string `json:"accountId"`
	}
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("parsing assignees: %w", err)
	}
	result := make([]models.Assignee, len(users))
	for i, u := range users {
		result[i] = models.Assignee{DisplayName: u.DisplayName, AccountID: u.AccountID}
	}
	return result, nil
}

func (c *jiraClient) SetAssignee(issueKey, accountID string) error {
	var assigneeField any
	if accountID != "" {
		assigneeField = map[string]any{"accountId": accountID}
	}
	payload := map[string]any{
		"fields": map[string]any{
			"assignee": assigneeField,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	apiURL := fmt.Sprintf("%s/rest/api/3/issue/%s", c.baseURL, issueKey)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, apiURL, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("set assignee %s: HTTP %d: %s", issueKey, resp.StatusCode, string(b))
	}
	return nil
}

func (c *jiraClient) MoveIssuesToSprint(sprintID int, keys []string) error {
	req, err := c.client.NewRequest(context.Background(), http.MethodPost,
		fmt.Sprintf("rest/agile/1.0/sprint/%d/issue", sprintID),
		map[string]any{"issues": keys})
	if err != nil {
		return err
	}
	_, err = c.client.Do(req, nil)
	return err
}

func (c *jiraClient) RankIssues(keys []string, rankAfterKey, rankBeforeKey string) error {
	payload := map[string]any{"issues": keys}
	if rankAfterKey != "" {
		payload["rankAfterIssue"] = rankAfterKey
	} else if rankBeforeKey != "" {
		payload["rankBeforeIssue"] = rankBeforeKey
	} else {
		return fmt.Errorf("rankIssues: must specify either rankAfterKey or rankBeforeKey")
	}
	req, err := c.client.NewRequest(context.Background(), http.MethodPut, "rest/agile/1.0/issue/rank", payload)
	if err != nil {
		return err
	}
	_, err = c.client.Do(req, nil)
	return err
}

func (c *jiraClient) MoveIssuesToBacklog(keys []string) error {
	req, err := c.client.NewRequest(context.Background(), http.MethodPost,
		"rest/agile/1.0/backlog/issue",
		map[string]any{"issues": keys})
	if err != nil {
		return err
	}
	_, err = c.client.Do(req, nil)
	return err
}

// trimDateStr trims a Jira date/datetime string to just the YYYY-MM-DD part.
func trimDateStr(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}
