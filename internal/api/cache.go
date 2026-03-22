package api

import (
	"fmt"
	"strings"
	"sync"

	"github.com/justinmklam/tira/internal/models"
)

// CacheInvalidator is implemented by clients that support cache invalidation.
// Call Invalidate to clear all cached data (e.g. on a manual refresh with R).
type CacheInvalidator interface {
	Invalidate()
}

// cachedClient wraps a Client and caches read-only responses in memory.
// It satisfies both Client and CacheInvalidator.
type cachedClient struct {
	inner Client
	mu    sync.RWMutex
	store map[string]any
}

// NewCachedClient wraps inner with an in-memory cache.
func NewCachedClient(inner Client) *cachedClient {
	return &cachedClient{inner: inner, store: make(map[string]any)}
}

// Invalidate clears all cached entries.
func (c *cachedClient) Invalidate() {
	c.mu.Lock()
	c.store = make(map[string]any)
	c.mu.Unlock()
}

func (c *cachedClient) cget(key string) (any, bool) {
	c.mu.RLock()
	v, ok := c.store[key]
	c.mu.RUnlock()
	return v, ok
}

func (c *cachedClient) cset(key string, val any) {
	c.mu.Lock()
	c.store[key] = val
	c.mu.Unlock()
}

func (c *cachedClient) cdel(key string) {
	c.mu.Lock()
	delete(c.store, key)
	c.mu.Unlock()
}

func (c *cachedClient) cdelPrefix(prefix string) {
	c.mu.Lock()
	for k := range c.store {
		if strings.HasPrefix(k, prefix) {
			delete(c.store, k)
		}
	}
	c.mu.Unlock()
}

// --- Cached read methods ---

func (c *cachedClient) GetIssue(key string) (*models.Issue, error) {
	ckey := "issue:" + key
	if v, ok := c.cget(ckey); ok {
		return v.(*models.Issue), nil
	}
	result, err := c.inner.GetIssue(key)
	if err != nil {
		return nil, err
	}
	c.cset(ckey, result)
	return result, nil
}

func (c *cachedClient) GetBoardColumns(boardID int) ([]models.BoardColumn, error) {
	ckey := fmt.Sprintf("board_cols:%d", boardID)
	if v, ok := c.cget(ckey); ok {
		return v.([]models.BoardColumn), nil
	}
	result, err := c.inner.GetBoardColumns(boardID)
	if err != nil {
		return nil, err
	}
	c.cset(ckey, result)
	return result, nil
}

func (c *cachedClient) GetSprintGroups(boardID int) ([]models.SprintGroup, error) {
	ckey := fmt.Sprintf("sprint_groups:%d", boardID)
	if v, ok := c.cget(ckey); ok {
		return v.([]models.SprintGroup), nil
	}
	result, err := c.inner.GetSprintGroups(boardID)
	if err != nil {
		return nil, err
	}
	c.cset(ckey, result)
	return result, nil
}

func (c *cachedClient) GetValidValues(projectKey string) (*models.ValidValues, error) {
	ckey := "valid_values:" + projectKey
	if v, ok := c.cget(ckey); ok {
		return v.(*models.ValidValues), nil
	}
	result, err := c.inner.GetValidValues(projectKey)
	if err != nil {
		return nil, err
	}
	c.cset(ckey, result)
	return result, nil
}

func (c *cachedClient) GetIssueMetadata(projectKey string) (*models.ValidValues, error) {
	ckey := "metadata:" + projectKey
	if v, ok := c.cget(ckey); ok {
		return v.(*models.ValidValues), nil
	}
	result, err := c.inner.GetIssueMetadata(projectKey)
	if err != nil {
		return nil, err
	}
	c.cset(ckey, result)
	return result, nil
}

func (c *cachedClient) GetEpics(projectKey, query string) ([]models.Issue, error) {
	ckey := fmt.Sprintf("epics:%s:%s", projectKey, query)
	if v, ok := c.cget(ckey); ok {
		return v.([]models.Issue), nil
	}
	result, err := c.inner.GetEpics(projectKey, query)
	if err != nil {
		return nil, err
	}
	c.cset(ckey, result)
	return result, nil
}

// --- Mutating methods: pass through and invalidate affected cache entries ---

func (c *cachedClient) UpdateIssue(key string, fields models.IssueFields) error {
	if err := c.inner.UpdateIssue(key, fields); err != nil {
		return err
	}
	c.cdel("issue:" + key)
	return nil
}

func (c *cachedClient) SetAssignee(issueKey, accountID string) error {
	if err := c.inner.SetAssignee(issueKey, accountID); err != nil {
		return err
	}
	c.cdel("issue:" + issueKey)
	return nil
}

func (c *cachedClient) SetParent(issueKey, parentKey string) error {
	if err := c.inner.SetParent(issueKey, parentKey); err != nil {
		return err
	}
	c.cdel("issue:" + issueKey)
	return nil
}

func (c *cachedClient) TransitionStatus(issueKey, statusID string) error {
	if err := c.inner.TransitionStatus(issueKey, statusID); err != nil {
		return err
	}
	c.cdel("issue:" + issueKey)
	return nil
}

func (c *cachedClient) AddComment(issueKey, text string) error {
	if err := c.inner.AddComment(issueKey, text); err != nil {
		return err
	}
	c.cdel("issue:" + issueKey)
	return nil
}

func (c *cachedClient) MoveIssuesToSprint(sprintID int, keys []string) error {
	if err := c.inner.MoveIssuesToSprint(sprintID, keys); err != nil {
		return err
	}
	c.cdelPrefix("sprint_groups:")
	return nil
}

func (c *cachedClient) MoveIssuesToBacklog(keys []string) error {
	if err := c.inner.MoveIssuesToBacklog(keys); err != nil {
		return err
	}
	c.cdelPrefix("sprint_groups:")
	return nil
}

func (c *cachedClient) RankIssues(keys []string, rankAfterKey, rankBeforeKey string) error {
	if err := c.inner.RankIssues(keys, rankAfterKey, rankBeforeKey); err != nil {
		return err
	}
	c.cdelPrefix("sprint_groups:")
	return nil
}

// --- Bulk operations: invalidate affected issue caches on success ---

func (c *cachedClient) BulkSetAssignee(keys []string, accountID string) []error {
	errs := c.inner.BulkSetAssignee(keys, accountID)
	for i, key := range keys {
		if errs[i] == nil {
			c.cdel("issue:" + key)
		}
	}
	return errs
}

func (c *cachedClient) BulkSetParent(keys []string, parentKey string) []error {
	errs := c.inner.BulkSetParent(keys, parentKey)
	for i, key := range keys {
		if errs[i] == nil {
			c.cdel("issue:" + key)
		}
	}
	return errs
}

func (c *cachedClient) BulkUpdateIssue(keys []string, fields models.IssueFields) []error {
	errs := c.inner.BulkUpdateIssue(keys, fields)
	for i, key := range keys {
		if errs[i] == nil {
			c.cdel("issue:" + key)
		}
	}
	return errs
}

func (c *cachedClient) BulkTransitionStatus(keys []string, transitionID string) []error {
	errs := c.inner.BulkTransitionStatus(keys, transitionID)
	for i, key := range keys {
		if errs[i] == nil {
			c.cdel("issue:" + key)
		}
	}
	return errs
}

// --- Straight pass-throughs (no caching) ---

func (c *cachedClient) CreateIssue(projectKey string, fields models.IssueFields) (*models.Issue, error) {
	return c.inner.CreateIssue(projectKey, fields)
}

func (c *cachedClient) GetActiveSprint(boardID int) ([]models.Issue, error) {
	return c.inner.GetActiveSprint(boardID)
}

func (c *cachedClient) GetBacklog(projectKey string) ([]models.Sprint, error) {
	return c.inner.GetBacklog(projectKey)
}

func (c *cachedClient) GetStatuses(issueKey string) ([]models.Status, error) {
	return c.inner.GetStatuses(issueKey)
}

func (c *cachedClient) SearchAssignees(projectKey, query string) ([]models.Assignee, error) {
	return c.inner.SearchAssignees(projectKey, query)
}

func (c *cachedClient) ValidateProject(projectKey string) error {
	return c.inner.ValidateProject(projectKey)
}

func (c *cachedClient) CreateSprint(boardID int, name, startDate, endDate string) (*models.Sprint, error) {
	return c.inner.CreateSprint(boardID, name, startDate, endDate)
}

func (c *cachedClient) UpdateSprint(sprintID int, name, startDate, endDate string) error {
	return c.inner.UpdateSprint(sprintID, name, startDate, endDate)
}
