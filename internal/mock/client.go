package mock

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/models"
)

// Client is an in-memory api.Client backed by a Fixture. It exists so that
// `tira --dev` can drive the real cobra commands and the real TUI with no
// credentials, no config file, and no network.
//
// It is a test double, not a Jira emulator: it does not reproduce Jira's
// workflow rules, permissions, or field validation. Every method that the real
// client would answer with an HTTP error answers with a descriptive error here,
// and every unsupported behaviour returns an error rather than silently doing
// nothing.
var _ api.Client = (*Client)(nil)

type Options struct {
	// FixturePath is the fixture file to load; empty means the embedded demo
	// fixture.
	FixturePath string
	// Project is the default project for CreateIssue and ValidateProject; empty
	// means the project declared by the fixture.
	Project string
	// BoardID is the board used by board-scoped calls; 0 means the board
	// declared by the fixture.
	BoardID int
	// StatePath is an optional JSON file that persists mutations across
	// invocations. When it exists and is non-empty it *is* the fixture, so
	// FixturePath is ignored; deleting it resets to the fixture. Unknown keys in
	// it are rejected on load.
	StatePath string
}

// Client holds all mutable dev state. A single mutex guards it because the TUI
// issues bulk mutations from goroutines.
type Client struct {
	mu        sync.Mutex
	fixture   *Fixture
	byKey     map[string]int // issue key -> index into fixture.Issues
	project   string
	boardID   int
	source    string
	statePath string
}

// New loads a fixture and returns a client backed by it.
func New(opts Options) (*Client, error) {
	var fixture *Fixture
	source := ""

	if opts.StatePath != "" {
		loaded, ok, err := loadState(opts.StatePath)
		if err != nil {
			return nil, err
		}
		if ok {
			fixture = loaded
			source = fmt.Sprintf("%s (state file; --dev-fixtures ignored)", opts.StatePath)
		}
	}
	if fixture == nil {
		loaded, err := Load(opts.FixturePath)
		if err != nil {
			return nil, err
		}
		fixture = loaded
		source = loaded.source
	}

	project := opts.Project
	if project == "" {
		project = fixture.Project
	}

	boardID := opts.BoardID
	if boardID == 0 {
		boardID = fixture.Board.ID
	}

	c := &Client{
		fixture:   fixture,
		byKey:     make(map[string]int, len(fixture.Issues)),
		project:   project,
		boardID:   boardID,
		source:    source,
		statePath: opts.StatePath,
	}
	for i, issue := range fixture.Issues {
		c.byKey[issue.Key] = i
	}
	return c, nil
}

// Project returns the project key used for CreateIssue and ValidateProject.
func (c *Client) Project() string { return c.project }

// BoardID returns the board used by board-scoped calls.
func (c *Client) BoardID() int { return c.boardID }

// Source describes where the fixture data came from, for the startup notice.
func (c *Client) Source() string { return c.source }

// --- reads -----------------------------------------------------------------

func (c *Client) GetIssue(key string) (*models.Issue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	fx, ok := c.lookup(key)
	if !ok {
		return nil, fmt.Errorf("issue %q not found in dev fixture", key)
	}

	issue := c.toIssue(fx, c.sprintNameFor(key))

	// The real client requests /comment?orderBy=-created, so comments arrive
	// newest-first. Fixtures are authored oldest-first for readability.
	comments := make([]models.Comment, 0, len(fx.Comments))
	for i := len(fx.Comments) - 1; i >= 0; i-- {
		comment := fx.Comments[i]
		comments = append(comments, models.Comment{
			Author:  comment.Author,
			Body:    comment.Body,
			Created: comment.Created,
		})
	}
	issue.Comments = comments

	return &issue, nil
}

func (c *Client) GetBoardColumns(boardID int) ([]models.BoardColumn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkBoard(boardID); err != nil {
		return nil, err
	}
	return c.boardColumns(), nil
}

func (c *Client) GetSprintList(boardID int) ([]models.Sprint, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkBoard(boardID); err != nil {
		return nil, err
	}
	return c.sprintList(), nil
}

func (c *Client) GetSprintGroups(boardID int) ([]models.SprintGroup, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkBoard(boardID); err != nil {
		return nil, err
	}

	groups, err := c.sprintGroupsBatch(c.sprintList())
	if err != nil {
		return nil, err
	}

	// Parity with the real client: the backlog group is appended unconditionally
	// on success because it is not part of the sprint list.
	groups = append(groups, models.SprintGroup{
		Sprint: models.Sprint{Name: "Backlog", State: "backlog"},
		Issues: c.issuesFor(c.fixture.Backlog, ""),
	})
	return groups, nil
}

func (c *Client) GetSprintGroupsBatch(boardID int, sprints []models.Sprint) ([]models.SprintGroup, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkBoard(boardID); err != nil {
		return nil, err
	}
	return c.sprintGroupsBatch(sprints)
}

func (c *Client) GetBacklogIssues(boardID int) ([]models.Issue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkBoard(boardID); err != nil {
		return nil, err
	}
	return c.issuesFor(c.fixture.Backlog, ""), nil
}

func (c *Client) GetActiveSprint(boardID int) ([]models.Issue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkBoard(boardID); err != nil {
		return nil, err
	}
	for _, sp := range c.fixture.Sprints {
		if toSprint(sp).State == "active" {
			return c.issuesFor(sp.Issues, sp.Name), nil
		}
	}
	return []models.Issue{}, nil
}

func (c *Client) GetBacklog(_ string) ([]models.Sprint, error) {
	return nil, errors.New("not implemented")
}

func (c *Client) GetEpics(projectKey, query string) ([]models.Issue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	needle := strings.ToLower(query)
	issues := make([]models.Issue, 0)
	for _, fx := range c.fixture.Issues {
		if fx.Type != "Epic" {
			continue
		}
		if projectKey != "" && projectKeyOf(fx.Key) != projectKey {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(fx.Summary), needle) {
			continue
		}
		// Parity with the real client, which selects only summary and issuetype
		// and orders by summary.
		issues = append(issues, models.Issue{Key: fx.Key, Summary: fx.Summary, IssueType: "Epic"})
	}
	slices.SortFunc(issues, func(a, b models.Issue) int {
		return strings.Compare(strings.ToLower(a.Summary), strings.ToLower(b.Summary))
	})
	return issues, nil
}

func (c *Client) GetEpicChildren(epicKey string) ([]models.Issue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.lookup(epicKey); !ok {
		return nil, fmt.Errorf("epic %q not found in dev fixture", epicKey)
	}

	children := make([]models.Issue, 0)
	for _, fx := range c.fixture.Issues {
		if fx.Epic != epicKey && fx.Parent != epicKey {
			continue
		}
		children = append(children, c.toIssue(fx, c.sprintNameFor(fx.Key)))
	}
	return children, nil
}

func (c *Client) GetValidValues(_ string) (*models.ValidValues, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.validValues(), nil
}

func (c *Client) GetIssueMetadata(_ string) (*models.ValidValues, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.validValues(), nil
}

func (c *Client) GetStatuses(issueKey string) ([]models.Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.lookup(issueKey); !ok {
		return nil, fmt.Errorf("issue %q not found in dev fixture", issueKey)
	}

	transitions := c.transitionsFor(issueKey)
	statuses := make([]models.Status, 0, len(transitions))
	for _, t := range transitions {
		statuses = append(statuses, models.Status{ID: t.ID, Name: t.Name})
	}
	return statuses, nil
}

func (c *Client) ValidateProject(projectKey string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if projectKey == "" {
		return errors.New("dev fixture: project key is empty")
	}
	if !c.projectKnown(projectKey) {
		return fmt.Errorf("project %q not found in dev fixture (fixture declares %q)", projectKey, c.project)
	}
	return nil
}

func (c *Client) SearchAssignees(_, query string) ([]models.Assignee, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	needle := strings.ToLower(query)
	assignees := make([]models.Assignee, 0, len(c.fixture.Users))
	for _, u := range c.fixture.Users {
		if needle != "" && !strings.Contains(strings.ToLower(u.DisplayName), needle) {
			continue
		}
		assignees = append(assignees, models.Assignee{DisplayName: u.DisplayName, AccountID: u.AccountID})
	}
	return assignees, nil
}

// --- mutations -------------------------------------------------------------

func (c *Client) UpdateIssue(key string, fields models.IssueFields) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	fx, err := c.mutableIssue(key)
	if err != nil {
		return err
	}
	// Validate before applying anything, so a bad reference cannot leave a
	// partial change behind.
	if fields.ParentKey != "" {
		if err := c.checkIssueRef("parent", fields.ParentKey); err != nil {
			return err
		}
	}

	if fields.Summary != "" {
		fx.Summary = fields.Summary
	}
	if fields.IssueType != "" {
		fx.Type = fields.IssueType
	}
	if fields.Priority != "" {
		fx.Priority = fields.Priority
	}
	if fields.AssigneeID != "" {
		fx.Assignee = c.displayNameFor(fields.AssigneeID, fx.Assignee)
	}
	if fields.Assignee != "" {
		fx.Assignee = fields.Assignee
	}
	if fields.StoryPoints > 0 {
		fx.StoryPoints = fields.StoryPoints
	}
	if len(fields.Labels) > 0 {
		fx.Labels = slices.Clone(fields.Labels)
	}
	if fields.Description != "" {
		fx.Description = fields.Description
	}
	if fields.AcceptanceCriteria != "" {
		fx.AcceptanceCriteria = fields.AcceptanceCriteria
	}
	if fields.ParentKey != "" {
		fx.Parent = fields.ParentKey
	}

	return c.persist()
}

// SetLabels replaces all labels on an issue; a nil slice clears them.
func (c *Client) SetLabels(issueKey string, labels []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	fx, err := c.mutableIssue(issueKey)
	if err != nil {
		return err
	}
	fx.Labels = slices.Clone(labels)
	return c.persist()
}

func (c *Client) SetAssignee(issueKey, accountID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	fx, err := c.mutableIssue(issueKey)
	if err != nil {
		return err
	}
	if accountID == "" {
		fx.Assignee = ""
		return c.persist()
	}
	// Fall back to the raw account ID when it is not one of the fixture users so
	// the assignment is still visible.
	fx.Assignee = c.displayNameFor(accountID, accountID)
	return c.persist()
}

func (c *Client) SetParent(issueKey, parentKey string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	fx, err := c.mutableIssue(issueKey)
	if err != nil {
		return err
	}
	if parentKey != "" {
		if err := c.checkIssueRef("parent", parentKey); err != nil {
			return err
		}
	}
	fx.Parent = parentKey
	return c.persist()
}

func (c *Client) AddComment(issueKey, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	fx, err := c.mutableIssue(issueKey)
	if err != nil {
		return err
	}
	// Comments are stored oldest-first, because GetIssue reverses them to match
	// the real /comment?orderBy=-created response, so a new one is appended.
	fx.Comments = append(fx.Comments, CommentFixture{
		Author:  "Dev User",
		Created: today(),
		Body:    text,
	})
	return c.persist()
}

func (c *Client) TransitionStatus(issueKey, statusID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	fx, err := c.mutableIssue(issueKey)
	if err != nil {
		return err
	}

	name := ""
	for _, t := range c.transitionsFor(issueKey) {
		if t.ID == statusID {
			name = t.Name
			break
		}
	}
	if name == "" {
		return fmt.Errorf("transition %q is not available for %s in dev fixture", statusID, issueKey)
	}

	fx.Status = name
	fx.StatusID = statusID
	fx.StatusChanged = today()
	return c.persist()
}

func (c *Client) CreateIssue(projectKey string, fields models.IssueFields) (*models.Issue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if projectKey == "" {
		projectKey = c.project
	}

	issueType := fields.IssueType
	if issueType == "" {
		issueType = "Task"
	}

	// Validate before mutating: an unknown project or parent would otherwise be
	// written into the state file, which loadState then rejects on the next run.
	if !c.projectKnown(projectKey) {
		return nil, fmt.Errorf("project %q not found in dev fixture (fixture declares %q)", projectKey, c.project)
	}
	if fields.ParentKey != "" {
		if err := c.checkIssueRef("parent", fields.ParentKey); err != nil {
			return nil, err
		}
	}

	fx := IssueFixture{
		Key:                c.nextIssueKey(projectKey),
		Summary:            fields.Summary,
		Type:               issueType,
		Priority:           fields.Priority,
		Assignee:           fields.Assignee,
		StoryPoints:        fields.StoryPoints,
		Labels:             slices.Clone(fields.Labels),
		Description:        fields.Description,
		AcceptanceCriteria: fields.AcceptanceCriteria,
		Parent:             fields.ParentKey,
	}
	if fields.AssigneeID != "" {
		fx.Assignee = c.displayNameFor(fields.AssigneeID, fx.Assignee)
	}
	// New issues start in the first configured transition ("To Do" in the demo
	// fixture), which mirrors Jira's initial workflow status.
	if transitions := c.transitionsFor(fx.Key); len(transitions) > 0 {
		fx.Status = transitions[0].Name
		fx.StatusID = transitions[0].ID
		fx.StatusChanged = today()
	}

	c.fixture.Issues = append(c.fixture.Issues, fx)
	c.byKey[fx.Key] = len(c.fixture.Issues) - 1
	c.fixture.Backlog = append(c.fixture.Backlog, fx.Key)

	if err := c.persist(); err != nil {
		return nil, err
	}
	created := c.toIssue(fx, "")
	return &created, nil
}

func (c *Client) MoveIssuesToSprint(sprintID int, keys []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	idx, ok := c.sprintIndex()[sprintID]
	if !ok {
		return fmt.Errorf("sprint %d not found in dev fixture", sprintID)
	}
	for _, key := range keys {
		if _, ok := c.lookup(key); !ok {
			return fmt.Errorf("issue %q not found in dev fixture", key)
		}
	}

	c.removeFromContainers(keys)
	c.fixture.Sprints[idx].Issues = append(c.fixture.Sprints[idx].Issues, keys...)
	return c.persist()
}

func (c *Client) MoveIssuesToBacklog(keys []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, key := range keys {
		if _, ok := c.lookup(key); !ok {
			return fmt.Errorf("issue %q not found in dev fixture", key)
		}
	}

	c.removeFromContainers(keys)
	c.fixture.Backlog = append(c.fixture.Backlog, keys...)
	return c.persist()
}

// RankIssues reorders keys within their container. An anchor that does not
// exist is ignored, matching the best-effort behaviour of the real rank call.
func (c *Client) RankIssues(keys []string, rankAfterKey, rankBeforeKey string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	anchor := rankBeforeKey
	before := true
	if anchor == "" {
		anchor = rankAfterKey
		before = false
	}
	if anchor == "" {
		return errors.New("rankIssues: must specify either rankAfterKey or rankBeforeKey")
	}

	container := c.containerOf(anchor)
	if container == nil {
		return nil
	}
	// Best-effort like the real rank call: an anchor that is itself being ranked
	// is a no-op. Without this guard, removing the keys first would delete the
	// anchor and leave the keys in no container at all.
	if slices.Contains(keys, anchor) {
		return nil
	}

	c.removeFromContainers(keys)

	insertAt := -1
	for i, key := range *container {
		if key != anchor {
			continue
		}
		if before {
			insertAt = i
		} else {
			insertAt = i + 1
		}
		break
	}
	if insertAt < 0 {
		return nil
	}

	*container = slices.Insert(*container, insertAt, keys...)
	return c.persist()
}

func (c *Client) CreateSprint(boardID int, name, startDate, endDate string) (*models.Sprint, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.checkBoard(boardID); err != nil {
		return nil, err
	}

	maxID := 0
	for _, sp := range c.fixture.Sprints {
		if sp.ID > maxID {
			maxID = sp.ID
		}
	}

	sp := SprintFixture{
		ID:        maxID + 1,
		Name:      name,
		State:     "future",
		StartDate: startDate,
		EndDate:   endDate,
	}
	c.fixture.Sprints = append(c.fixture.Sprints, sp)

	if err := c.persist(); err != nil {
		return nil, err
	}
	created := toSprint(sp)
	return &created, nil
}

func (c *Client) UpdateSprint(sprintID int, name, startDate, endDate string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	idx, ok := c.sprintIndex()[sprintID]
	if !ok {
		return fmt.Errorf("sprint %d not found in dev fixture", sprintID)
	}

	sp := &c.fixture.Sprints[idx]
	sp.Name = name
	sp.StartDate = startDate
	sp.EndDate = endDate
	return c.persist()
}

func (c *Client) BulkSetAssignee(keys []string, accountID string) []error {
	return c.bulk(keys, func(key string) error { return c.SetAssignee(key, accountID) })
}

func (c *Client) BulkSetParent(keys []string, parentKey string) []error {
	return c.bulk(keys, func(key string) error { return c.SetParent(key, parentKey) })
}

func (c *Client) BulkUpdateIssue(keys []string, fields models.IssueFields) []error {
	return c.bulk(keys, func(key string) error { return c.UpdateIssue(key, fields) })
}

func (c *Client) BulkTransitionStatus(keys []string, statusID string) []error {
	return c.bulk(keys, func(key string) error { return c.TransitionStatus(key, statusID) })
}

// bulk runs op for every key sequentially (fixtures are small) and returns one
// error per key in input order, nil for successes. Like the real client's
// bulkOperation, an empty key list returns nil.
func (c *Client) bulk(keys []string, op func(string) error) []error {
	if len(keys) == 0 {
		return nil
	}
	errs := make([]error, len(keys))
	for i, key := range keys {
		errs[i] = op(key)
	}
	return errs
}

// --- internals -------------------------------------------------------------

// persist writes the optional state file. It is called with c.mu held at the
// end of every successful mutation; the in-memory change is already applied, so
// a write failure is reported without rolling anything back.
func (c *Client) persist() error {
	if c.statePath == "" {
		return nil
	}
	if err := saveState(c.statePath, c.fixture); err != nil {
		return fmt.Errorf("persisting dev state: %w", err)
	}
	return nil
}

func (c *Client) checkBoard(boardID int) error {
	if boardID != c.boardID {
		return fmt.Errorf("board %d not found in dev fixture (have %d)", boardID, c.boardID)
	}
	return nil
}

func (c *Client) checkIssueRef(where, key string) error {
	if _, ok := c.lookup(key); !ok {
		return fmt.Errorf("%s %q not found in dev fixture", where, key)
	}
	return nil
}

// projectKnown reports whether projectKey is the client's project or the prefix
// of at least one fixture issue.
func (c *Client) projectKnown(projectKey string) bool {
	if projectKey == "" {
		return false
	}
	if projectKey == c.project {
		return true
	}
	for _, fx := range c.fixture.Issues {
		if projectKeyOf(fx.Key) == projectKey {
			return true
		}
	}
	return false
}

func (c *Client) lookup(key string) (IssueFixture, bool) {
	idx, ok := c.byKey[key]
	if !ok {
		return IssueFixture{}, false
	}
	return c.fixture.Issues[idx], true
}

func (c *Client) mutableIssue(key string) (*IssueFixture, error) {
	idx, ok := c.byKey[key]
	if !ok {
		return nil, fmt.Errorf("issue %q not found in dev fixture", key)
	}
	return &c.fixture.Issues[idx], nil
}

func (c *Client) sprintIndex() map[int]int {
	index := make(map[int]int, len(c.fixture.Sprints))
	for i, sp := range c.fixture.Sprints {
		index[sp.ID] = i
	}
	return index
}

// sprintNameFor returns the sprint an issue is assigned to, or "" for backlog
// issues.
func (c *Client) sprintNameFor(key string) string {
	for _, sp := range c.fixture.Sprints {
		if slices.Contains(sp.Issues, key) {
			return sp.Name
		}
	}
	return ""
}

func (c *Client) sprintList() []models.Sprint {
	sprints := make([]models.Sprint, 0, len(c.fixture.Sprints))
	for _, sp := range c.fixture.Sprints {
		converted := toSprint(sp)
		// Parity with the real `state=active,future` query.
		if converted.State != "active" && converted.State != "future" {
			continue
		}
		sprints = append(sprints, converted)
	}
	return sprints
}

func (c *Client) sprintGroupsBatch(sprints []models.Sprint) ([]models.SprintGroup, error) {
	index := c.sprintIndex()
	groups := make([]models.SprintGroup, len(sprints))
	for i, sp := range sprints {
		idx, ok := index[sp.ID]
		if !ok {
			return nil, fmt.Errorf("sprint %d not found in dev fixture", sp.ID)
		}
		groups[i] = models.SprintGroup{
			Sprint: sp,
			Issues: c.issuesFor(c.fixture.Sprints[idx].Issues, sp.Name),
		}
	}
	return groups, nil
}

// issuesFor converts board-list keys using the sparse projection the real agile
// endpoints return, so a board in dev mode looks like a real one.
func (c *Client) issuesFor(keys []string, sprintName string) []models.Issue {
	issues := make([]models.Issue, 0, len(keys))
	for _, key := range keys {
		if fx, ok := c.lookup(key); ok {
			issues = append(issues, c.toBoardIssue(fx, sprintName))
		}
	}
	return issues
}

// toBoardIssue is the board-list projection. The real agile endpoints request
// only summary, status, issuetype, priority, assignee, labels, parent, project
// and story points (see fetchAgileIssues in internal/api/client.go), so
// description, acceptance criteria, reporter, status-change date, subtasks, links
// and the parent link are deliberately left zero here; they arrive only from
// GetIssue.
func (c *Client) toBoardIssue(fx IssueFixture, sprintName string) models.Issue {
	issue := models.Issue{
		Key:         fx.Key,
		Summary:     fx.Summary,
		Status:      fx.Status,
		StatusID:    c.statusIDFor(fx),
		IssueType:   fx.Type,
		Priority:    fx.Priority,
		Assignee:    fx.Assignee,
		AssigneeID:  c.accountIDFor(fx.Assignee),
		StoryPoints: fx.StoryPoints,
		Labels:      slices.Clone(fx.Labels),
		ProjectKey:  projectKeyOf(fx.Key),
		SprintName:  sprintName,
	}
	if fx.Epic != "" {
		issue.EpicKey = fx.Epic
		if epic, ok := c.lookup(fx.Epic); ok {
			issue.EpicName = epic.Summary
			issue.EpicStatus = epic.Status
		}
	}
	return issue
}

func (c *Client) boardColumns() []models.BoardColumn {
	byName, ids := c.statusIndex()
	columns := make([]models.BoardColumn, 0, len(c.fixture.Board.Columns))
	for _, col := range c.fixture.Board.Columns {
		refs := col.Statuses
		if len(refs) == 0 {
			refs = []string{col.Name}
		}
		statusIDs := make([]string, 0, len(refs))
		for _, ref := range refs {
			statusIDs = append(statusIDs, resolveStatusRef(ref, byName, ids))
		}
		columns = append(columns, models.BoardColumn{Name: col.Name, StatusIDs: statusIDs})
	}
	return columns
}

func (c *Client) validValues() *models.ValidValues {
	valid := &models.ValidValues{}

	if len(c.fixture.IssueTypes) > 0 {
		valid.IssueTypes = slices.Clone(c.fixture.IssueTypes)
	} else {
		seen := make(map[string]bool)
		for _, fx := range c.fixture.Issues {
			if fx.Type == "" || seen[fx.Type] {
				continue
			}
			seen[fx.Type] = true
			valid.IssueTypes = append(valid.IssueTypes, fx.Type)
		}
		if !seen["Epic"] {
			valid.IssueTypes = append(valid.IssueTypes, "Epic")
		}
	}

	if len(c.fixture.Priorities) > 0 {
		valid.Priorities = slices.Clone(c.fixture.Priorities)
	} else {
		valid.Priorities = []string{"Highest", "High", "Medium", "Low", "Lowest"}
	}

	for _, u := range c.fixture.Users {
		valid.Assignees = append(valid.Assignees, models.Assignee{
			DisplayName: u.DisplayName,
			AccountID:   u.AccountID,
		})
	}

	for _, sp := range c.fixture.Sprints {
		valid.Sprints = append(valid.Sprints, toSprint(sp))
	}

	return valid
}

// toIssue is the full-detail projection used by GetIssue (and returned by
// CreateIssue). Contextual fields that a fixture issue cannot know on its own are
// supplied by the caller.
func (c *Client) toIssue(fx IssueFixture, sprintName string) models.Issue {
	issue := models.Issue{
		Key:                fx.Key,
		Summary:            fx.Summary,
		Description:        fx.Description,
		AcceptanceCriteria: fx.AcceptanceCriteria,
		Status:             fx.Status,
		StatusID:           c.statusIDFor(fx),
		IssueType:          fx.Type,
		Priority:           fx.Priority,
		Assignee:           fx.Assignee,
		AssigneeID:         c.accountIDFor(fx.Assignee),
		Reporter:           fx.Reporter,
		StoryPoints:        fx.StoryPoints,
		Labels:             slices.Clone(fx.Labels),
		ProjectKey:         projectKeyOf(fx.Key),
		SprintName:         sprintName,
		StatusChangedDate:  fx.StatusChanged,
	}

	if fx.Epic != "" {
		issue.EpicKey = fx.Epic
		if epic, ok := c.lookup(fx.Epic); ok {
			issue.EpicName = epic.Summary
			issue.EpicStatus = epic.Status
		}
	}
	if fx.Parent != "" {
		issue.ParentKey = fx.Parent
		if parent, ok := c.lookup(fx.Parent); ok {
			issue.ParentSummary = parent.Summary
		}
	}

	issue.SubTaskCount = len(fx.Subtasks)
	for _, sub := range fx.Subtasks {
		if child, ok := c.lookup(sub); ok {
			issue.SubTasks = append(issue.SubTasks, c.toLinkedIssue("subtask", child))
		}
	}
	for _, link := range fx.Links {
		if target, ok := c.lookup(link.Key); ok {
			issue.LinkedIssues = append(issue.LinkedIssues, c.toLinkedIssue(link.Relationship, target))
		}
	}

	return issue
}

func (c *Client) toLinkedIssue(relationship string, fx IssueFixture) models.LinkedIssue {
	return models.LinkedIssue{
		Relationship: relationship,
		Key:          fx.Key,
		Summary:      fx.Summary,
		Status:       fx.Status,
		IssueType:    fx.Type,
		Priority:     fx.Priority,
		SubTaskCount: len(fx.Subtasks),
	}
}

func (c *Client) statusIDFor(fx IssueFixture) string {
	if fx.StatusID != "" {
		return fx.StatusID
	}
	if fx.Status == "" {
		return ""
	}
	for _, list := range [][]TransitionFixture{
		c.fixture.Transitions[fx.Key],
		c.fixture.Transitions["default"],
	} {
		for _, t := range list {
			if strings.EqualFold(t.Name, fx.Status) {
				return t.ID
			}
		}
	}
	return ""
}

func (c *Client) transitionsFor(key string) []TransitionFixture {
	if list := c.fixture.Transitions[key]; len(list) > 0 {
		return list
	}
	return c.fixture.Transitions["default"]
}

// statusIndex maps lower-cased status names to IDs and collects every known
// status ID, so board columns can be declared with either names or IDs.
func (c *Client) statusIndex() (map[string]string, map[string]bool) {
	byName := make(map[string]string)
	ids := make(map[string]bool)
	for _, list := range c.fixture.Transitions {
		for _, t := range list {
			if t.ID == "" {
				continue
			}
			ids[t.ID] = true
			if t.Name != "" {
				byName[strings.ToLower(t.Name)] = t.ID
			}
		}
	}
	for _, fx := range c.fixture.Issues {
		if fx.StatusID == "" {
			continue
		}
		ids[fx.StatusID] = true
		if fx.Status != "" {
			byName[strings.ToLower(fx.Status)] = fx.StatusID
		}
	}
	return byName, ids
}

func (c *Client) accountIDFor(displayName string) string {
	if displayName == "" {
		return ""
	}
	for _, u := range c.fixture.Users {
		if strings.EqualFold(u.DisplayName, displayName) {
			return u.AccountID
		}
	}
	for _, u := range c.fixture.Users {
		if u.AccountID == displayName {
			return u.AccountID
		}
	}
	return ""
}

func (c *Client) displayNameFor(accountID, fallback string) string {
	if accountID == "" {
		return fallback
	}
	for _, u := range c.fixture.Users {
		if u.AccountID == accountID {
			return u.DisplayName
		}
	}
	return accountID
}

func (c *Client) nextIssueKey(projectKey string) string {
	prefix := projectKey + "-"
	for n := 1; ; n++ {
		key := fmt.Sprintf("%s%d", prefix, n)
		if _, exists := c.byKey[key]; !exists {
			return key
		}
	}
}

func (c *Client) removeFromContainers(keys []string) {
	drop := make(map[string]bool, len(keys))
	for _, key := range keys {
		drop[key] = true
	}
	c.fixture.Backlog = dropKeys(c.fixture.Backlog, drop)
	for i := range c.fixture.Sprints {
		c.fixture.Sprints[i].Issues = dropKeys(c.fixture.Sprints[i].Issues, drop)
	}
}

// containerOf returns a pointer to the key list holding key, or nil.
func (c *Client) containerOf(key string) *[]string {
	if slices.Contains(c.fixture.Backlog, key) {
		return &c.fixture.Backlog
	}
	for i := range c.fixture.Sprints {
		if slices.Contains(c.fixture.Sprints[i].Issues, key) {
			return &c.fixture.Sprints[i].Issues
		}
	}
	return nil
}

func dropKeys(keys []string, drop map[string]bool) []string {
	return slices.DeleteFunc(keys, func(key string) bool { return drop[key] })
}

func resolveStatusRef(ref string, byName map[string]string, ids map[string]bool) string {
	if ref == "" {
		return ""
	}
	if ids[ref] {
		return ref
	}
	if id, ok := byName[strings.ToLower(ref)]; ok {
		return id
	}
	return ref
}

func toSprint(sp SprintFixture) models.Sprint {
	state := sp.State
	if state == "" {
		state = "future"
	}
	return models.Sprint{
		ID:        sp.ID,
		Name:      sp.Name,
		State:     state,
		StartDate: sp.StartDate,
		EndDate:   sp.EndDate,
	}
}

// projectKeyOf extracts the project key from an issue key ("DEMO-3" -> "DEMO").
func projectKeyOf(issueKey string) string {
	if i := strings.LastIndex(issueKey, "-"); i > 0 {
		return issueKey[:i]
	}
	return issueKey
}

func today() string { return time.Now().Format("2006-01-02") }
