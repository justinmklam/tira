package mock

import (
	"testing"

	"github.com/justinmklam/tira/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T) *Client {
	t.Helper()
	c, err := New(Options{})
	require.NoError(t, err)
	return c
}

func keys(issues []models.Issue) []string {
	out := make([]string, 0, len(issues))
	for _, issue := range issues {
		out = append(out, issue.Key)
	}
	return out
}

func TestGetIssueCarriesFixtureContext(t *testing.T) {
	c := newTestClient(t)

	issue, err := c.GetIssue("DEMO-5")
	require.NoError(t, err)

	assert.Equal(t, "DEMO", issue.ProjectKey)
	assert.Equal(t, "DEMO Sprint 1", issue.SprintName)
	assert.Equal(t, "11", issue.StatusID)
	assert.Equal(t, "DEMO-1", issue.EpicKey)
	assert.Equal(t, "Checkout revamp", issue.EpicName)
	assert.Equal(t, "To Do", issue.EpicStatus)
	assert.Equal(t, "acct-ada", issue.AssigneeID)

	// The real client requests ?orderBy=-created, so comments come newest-first
	// even though fixtures are authored oldest-first.
	require.Len(t, issue.Comments, 2)
	assert.Equal(t, "Ada Lovelace", issue.Comments[0].Author)
	assert.Equal(t, "Grace Hopper", issue.Comments[1].Author)

	// Mutating the returned value must not reach into fixture state.
	issue.Comments[0].Body = "mutated"
	issue.Labels = append(issue.Labels, "mutated")

	again, err := c.GetIssue("DEMO-5")
	require.NoError(t, err)
	assert.NotEqual(t, "mutated", again.Comments[0].Body)
	assert.NotContains(t, again.Labels, "mutated")
}

func TestGetIssueIncludesSubtasksAndLinks(t *testing.T) {
	c := newTestClient(t)

	issue, err := c.GetIssue("DEMO-7")
	require.NoError(t, err)

	assert.Empty(t, issue.SprintName, "backlog issues have no sprint name")
	assert.Equal(t, 2, issue.SubTaskCount)
	require.Len(t, issue.SubTasks, 2)
	assert.Equal(t, "DEMO-10", issue.SubTasks[0].Key)
	assert.Equal(t, "subtask", issue.SubTasks[0].Relationship)
	assert.Equal(t, "In Progress", issue.SubTasks[1].Status)

	require.Len(t, issue.LinkedIssues, 1)
	assert.Equal(t, "blocks", issue.LinkedIssues[0].Relationship)
	assert.Equal(t, "DEMO-3", issue.LinkedIssues[0].Key)
}

func TestGetIssueUnknownKey(t *testing.T) {
	c := newTestClient(t)

	_, err := c.GetIssue("DEMO-999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DEMO-999")
}

func TestGetSprintGroupsEndsWithBacklog(t *testing.T) {
	c := newTestClient(t)

	groups, err := c.GetSprintGroups(1)
	require.NoError(t, err)
	require.Len(t, groups, 3)

	assert.Equal(t, "DEMO Sprint 1", groups[0].Sprint.Name)
	assert.Equal(t, "active", groups[0].Sprint.State)
	assert.Equal(t, "DEMO Sprint 2", groups[1].Sprint.Name)

	last := groups[len(groups)-1]
	assert.Equal(t, "Backlog", last.Sprint.Name)
	assert.Equal(t, "backlog", last.Sprint.State)
	assert.Equal(t, []string{"DEMO-7", "DEMO-8", "DEMO-9"}, keys(last.Issues))
}

func TestGetSprintListExcludesClosedSprints(t *testing.T) {
	path := writeFixture(t, `project: DEMO
board:
  id: 7
  columns:
    - name: To Do
sprints:
  - id: 1
    name: Active
    state: active
  - id: 2
    name: Closed
    state: closed
  - id: 3
    name: Undated
issues:
  - key: DEMO-1
    summary: One
`)
	c, err := New(Options{FixturePath: path, BoardID: 7})
	require.NoError(t, err)
	require.Equal(t, 7, c.BoardID())
	assert.Equal(t, "DEMO", c.Project())

	sprints, err := c.GetSprintList(7)
	require.NoError(t, err)
	require.Len(t, sprints, 2)
	assert.Equal(t, "Active", sprints[0].Name)
	assert.Equal(t, "Undated", sprints[1].Name)
	assert.Equal(t, "future", sprints[1].State, "a missing state is treated as future")
}

func TestGetSprintGroupsBatchOrderAndEmptyGroups(t *testing.T) {
	c := newTestClient(t)

	sprints, err := c.GetSprintList(1)
	require.NoError(t, err)
	require.Len(t, sprints, 2)

	// Requested order is preserved, and each issue carries its sprint name.
	groups, err := c.GetSprintGroupsBatch(1, []models.Sprint{sprints[1], sprints[0]})
	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, "DEMO Sprint 2", groups[0].Sprint.Name)
	assert.Equal(t, []string{"DEMO-6"}, keys(groups[0].Issues))
	assert.Equal(t, "DEMO Sprint 2", groups[0].Issues[0].SprintName)
	assert.Equal(t, "DEMO Sprint 1", groups[1].Sprint.Name)

	created, err := c.CreateSprint(1, "Empty Sprint", "2026-04-01", "2026-04-14")
	require.NoError(t, err)

	groups, err = c.GetSprintGroupsBatch(1, []models.Sprint{*created})
	require.NoError(t, err, "a sprint with no issues is an empty group, not an error")
	require.Len(t, groups, 1)
	assert.Empty(t, groups[0].Issues)
}

func TestUnknownBoardSprintAndIssueErrors(t *testing.T) {
	c := newTestClient(t)

	_, err := c.GetBoardColumns(99)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "board 99")

	_, err = c.GetSprintList(99)
	require.Error(t, err)

	_, err = c.GetSprintGroups(99)
	require.Error(t, err)

	_, err = c.GetBacklogIssues(99)
	require.Error(t, err)

	_, err = c.GetActiveSprint(99)
	require.Error(t, err)

	_, err = c.CreateSprint(99, "Nope", "2026-04-01", "2026-04-14")
	require.Error(t, err)

	_, err = c.GetSprintGroupsBatch(1, []models.Sprint{{ID: 99, Name: "Nope"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sprint 99")

	_, err = c.GetStatuses("DEMO-999")
	require.Error(t, err)

	_, err = c.GetEpicChildren("DEMO-999")
	require.Error(t, err)

	require.Error(t, c.UpdateSprint(99, "Nope", "", ""))
}

func TestBoardColumnsResolveStatusNames(t *testing.T) {
	c := newTestClient(t)

	columns, err := c.GetBoardColumns(1)
	require.NoError(t, err)
	require.Len(t, columns, 3)
	assert.Equal(t, "To Do", columns[0].Name)
	assert.Equal(t, []string{"11"}, columns[0].StatusIDs)
	assert.Equal(t, []string{"21"}, columns[1].StatusIDs)
	assert.Equal(t, []string{"31"}, columns[2].StatusIDs)
}

func TestMutationsAreVisibleToTheNextRead(t *testing.T) {
	c := newTestClient(t)

	require.NoError(t, c.UpdateIssue("DEMO-3", models.IssueFields{
		Summary:     "Renamed story",
		Priority:    "Lowest",
		Description: "New description",
	}))

	issue := mustGet(t, c, "DEMO-3")
	assert.Equal(t, "Renamed story", issue.Summary)
	assert.Equal(t, "Lowest", issue.Priority)
	assert.Equal(t, "New description", issue.Description)
	assert.Equal(t, "Story", issue.IssueType, "unset fields are preserved")

	require.NoError(t, c.SetLabels("DEMO-3", []string{"alpha", "beta"}))
	issue = mustGet(t, c, "DEMO-3")
	assert.Equal(t, []string{"alpha", "beta"}, issue.Labels)

	require.NoError(t, c.SetLabels("DEMO-3", nil))
	assert.Empty(t, mustGet(t, c, "DEMO-3").Labels)

	require.NoError(t, c.SetAssignee("DEMO-3", "acct-grace"))
	issue = mustGet(t, c, "DEMO-3")
	assert.Equal(t, "Grace Hopper", issue.Assignee)
	assert.Equal(t, "acct-grace", issue.AssigneeID)

	require.NoError(t, c.SetAssignee("DEMO-3", ""))
	assert.Empty(t, mustGet(t, c, "DEMO-3").Assignee)

	require.NoError(t, c.AddComment("DEMO-3", "hello from a test"))
	issue = mustGet(t, c, "DEMO-3")
	require.Len(t, issue.Comments, 1)
	assert.Equal(t, "Dev User", issue.Comments[0].Author)
	assert.Equal(t, today(), issue.Comments[0].Created)

	require.NoError(t, c.SetParent("DEMO-3", "DEMO-2"))
	issue = mustGet(t, c, "DEMO-3")
	assert.Equal(t, "DEMO-2", issue.ParentKey)
	assert.Equal(t, "Platform hardening", issue.ParentSummary)

	require.Error(t, c.SetParent("DEMO-3", "DEMO-999"), "an unknown parent must not be applied")
	assert.Equal(t, "DEMO-2", mustGet(t, c, "DEMO-3").ParentKey)
}

func TestTransitionStatusUpdatesStatusAndDate(t *testing.T) {
	c := newTestClient(t)

	require.NoError(t, c.TransitionStatus("DEMO-3", "31"))

	issue := mustGet(t, c, "DEMO-3")
	assert.Equal(t, "Done", issue.Status)
	assert.Equal(t, "31", issue.StatusID)
	assert.Equal(t, today(), issue.StatusChangedDate)

	require.Error(t, c.TransitionStatus("DEMO-3", "999"))
	assert.Equal(t, "Done", mustGet(t, c, "DEMO-3").Status, "a bad transition changes nothing")

	require.Error(t, c.TransitionStatus("DEMO-999", "31"))
}

func TestGetStatusesFallsBackToDefaultTransitions(t *testing.T) {
	c := newTestClient(t)

	statuses, err := c.GetStatuses("DEMO-3")
	require.NoError(t, err)
	require.Len(t, statuses, 3)
	assert.Equal(t, models.Status{ID: "11", Name: "To Do"}, statuses[0])
	assert.Equal(t, models.Status{ID: "31", Name: "Done"}, statuses[2])
}

func TestCreateIssueAssignsNextFreeKeyAndBacklogsIt(t *testing.T) {
	c := newTestClient(t)

	created, err := c.CreateIssue("DEMO", models.IssueFields{
		Summary:     "Brand new story",
		IssueType:   "Story",
		StoryPoints: 3,
		Labels:      []string{"new"},
	})
	require.NoError(t, err)
	assert.Equal(t, "DEMO-12", created.Key)
	assert.Equal(t, "DEMO", created.ProjectKey)
	assert.Equal(t, "Brand new story", created.Summary)
	assert.Equal(t, "To Do", created.Status)
	assert.Equal(t, "11", created.StatusID)

	backlog, err := c.GetBacklogIssues(1)
	require.NoError(t, err)
	assert.Contains(t, keys(backlog), "DEMO-12")

	again, err := c.CreateIssue("DEMO", models.IssueFields{Summary: "Second"})
	require.NoError(t, err)
	assert.Equal(t, "DEMO-13", again.Key)
	assert.Equal(t, "Task", again.IssueType, "a missing type defaults to Task")

	issue := mustGet(t, c, "DEMO-12")
	assert.Equal(t, 3.0, issue.StoryPoints)
	assert.Equal(t, []string{"new"}, issue.Labels)
}

func TestMoveIssuesBetweenSprintsAndBacklog(t *testing.T) {
	c := newTestClient(t)

	require.NoError(t, c.MoveIssuesToSprint(1, []string{"DEMO-7", "DEMO-9"}))

	active, err := c.GetActiveSprint(1)
	require.NoError(t, err)
	assert.Equal(t, []string{"DEMO-3", "DEMO-4", "DEMO-5", "DEMO-7", "DEMO-9"}, keys(active))

	backlog, err := c.GetBacklogIssues(1)
	require.NoError(t, err)
	assert.Equal(t, []string{"DEMO-8"}, keys(backlog))

	// An unknown sprint or issue applies no partial change.
	require.Error(t, c.MoveIssuesToSprint(99, []string{"DEMO-8"}))
	require.Error(t, c.MoveIssuesToSprint(1, []string{"DEMO-8", "DEMO-999"}))
	active, err = c.GetActiveSprint(1)
	require.NoError(t, err)
	assert.NotContains(t, keys(active), "DEMO-8")

	require.NoError(t, c.MoveIssuesToBacklog([]string{"DEMO-7"}))
	backlog, err = c.GetBacklogIssues(1)
	require.NoError(t, err)
	assert.Equal(t, []string{"DEMO-8", "DEMO-7"}, keys(backlog))
}

func TestRankIssuesReordersWithinContainer(t *testing.T) {
	c := newTestClient(t)

	require.NoError(t, c.RankIssues([]string{"DEMO-5"}, "", "DEMO-3"))
	assert.Equal(t, []string{"DEMO-5", "DEMO-3", "DEMO-4"}, mustActive(t, c))

	require.NoError(t, c.RankIssues([]string{"DEMO-5"}, "DEMO-4", ""))
	assert.Equal(t, []string{"DEMO-3", "DEMO-4", "DEMO-5"}, mustActive(t, c))

	// A missing anchor is best-effort, not an error.
	require.NoError(t, c.RankIssues([]string{"DEMO-3"}, "DEMO-999", ""))
	assert.Equal(t, []string{"DEMO-3", "DEMO-4", "DEMO-5"}, mustActive(t, c))

	require.Error(t, c.RankIssues([]string{"DEMO-3"}, "", ""))
}

func TestBulkOperationsReportPerKeyErrors(t *testing.T) {
	c := newTestClient(t)

	errs := c.BulkUpdateIssue([]string{"DEMO-3", "DEMO-999", "DEMO-4"}, models.IssueFields{Priority: "High"})
	require.Len(t, errs, 3)
	assert.NoError(t, errs[0])
	assert.Error(t, errs[1])
	assert.NoError(t, errs[2], "one failure must not abort the rest")
	assert.Equal(t, "High", mustGet(t, c, "DEMO-4").Priority)

	errs = c.BulkTransitionStatus([]string{"DEMO-3", "DEMO-999"}, "31")
	require.Len(t, errs, 2)
	assert.NoError(t, errs[0])
	assert.Error(t, errs[1])
	assert.Equal(t, "Done", mustGet(t, c, "DEMO-3").Status)

	errs = c.BulkSetAssignee([]string{"DEMO-3"}, "acct-grace")
	require.Len(t, errs, 1)
	assert.NoError(t, errs[0])
	assert.Equal(t, "Grace Hopper", mustGet(t, c, "DEMO-3").Assignee)

	errs = c.BulkSetParent([]string{"DEMO-3"}, "DEMO-2")
	require.Len(t, errs, 1)
	assert.NoError(t, errs[0])
	assert.Equal(t, "DEMO-2", mustGet(t, c, "DEMO-3").ParentKey)

	assert.Nil(t, c.BulkUpdateIssue(nil, models.IssueFields{}), "no keys means no errors")
}

func TestGetEpicsAndEpicChildren(t *testing.T) {
	c := newTestClient(t)

	epics, err := c.GetEpics("DEMO", "")
	require.NoError(t, err)
	require.Len(t, epics, 2)
	assert.Equal(t, []string{"DEMO-1", "DEMO-2"}, keys(epics))

	filtered, err := c.GetEpics("DEMO", "PLATFORM")
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "DEMO-2", filtered[0].Key)

	assert.Empty(t, mustEpics(t, c, "OTHER", ""))

	children, err := c.GetEpicChildren("DEMO-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"DEMO-3", "DEMO-4", "DEMO-5", "DEMO-6"}, keys(children))

	// A parent link also counts as a child relationship.
	children, err = c.GetEpicChildren("DEMO-7")
	require.NoError(t, err)
	assert.Equal(t, []string{"DEMO-10", "DEMO-11"}, keys(children))
}

func TestValidValuesAndProjectValidation(t *testing.T) {
	c := newTestClient(t)

	valid, err := c.GetValidValues("DEMO")
	require.NoError(t, err)
	assert.Equal(t, []string{"Epic", "Story", "Task", "Bug", "Subtask"}, valid.IssueTypes)
	assert.Equal(t, []string{"Highest", "High", "Medium", "Low", "Lowest"}, valid.Priorities)
	assert.Len(t, valid.Assignees, 3)
	assert.Len(t, valid.Sprints, 2)

	metadata, err := c.GetIssueMetadata("DEMO")
	require.NoError(t, err)
	assert.Equal(t, valid.IssueTypes, metadata.IssueTypes)

	require.NoError(t, c.ValidateProject("DEMO"))
	require.Error(t, c.ValidateProject("NOPE"))
	require.Error(t, c.ValidateProject(""))
}

func TestSearchAssigneesFiltersBySubstring(t *testing.T) {
	c := newTestClient(t)

	all, err := c.SearchAssignees("DEMO", "")
	require.NoError(t, err)
	assert.Len(t, all, 3)

	grace, err := c.SearchAssignees("DEMO", "GRAC")
	require.NoError(t, err)
	require.Len(t, grace, 1)
	assert.Equal(t, models.Assignee{DisplayName: "Grace Hopper", AccountID: "acct-grace"}, grace[0])
}

func TestAddCommentKeepsNewestFirst(t *testing.T) {
	c := newTestClient(t)

	// DEMO-5 has two fixture comments, authored oldest-first.
	require.Len(t, mustGet(t, c, "DEMO-5").Comments, 2)

	require.NoError(t, c.AddComment("DEMO-5", "newest"))

	comments := mustGet(t, c, "DEMO-5").Comments
	require.Len(t, comments, 3)
	assert.Equal(t, "newest", comments[0].Body, "a new comment must render first")
	assert.Equal(t, "Ada Lovelace", comments[1].Author, "the existing order is preserved")
	assert.Equal(t, "Grace Hopper", comments[2].Author)
}

func TestBoardListIssuesUseTheSparseAgileProjection(t *testing.T) {
	c := newTestClient(t)

	groups, err := c.GetSprintGroups(1)
	require.NoError(t, err)

	var listIssue models.Issue
	for _, group := range groups {
		for _, issue := range group.Issues {
			if issue.Key == "DEMO-3" {
				listIssue = issue
			}
		}
	}
	require.Equal(t, "DEMO-3", listIssue.Key, "DEMO-3 must appear in the board data")

	// Fields the agile endpoints do request.
	assert.Equal(t, "DEMO", listIssue.ProjectKey)
	assert.Equal(t, "DEMO Sprint 1", listIssue.SprintName)
	assert.Equal(t, "21", listIssue.StatusID)
	assert.Equal(t, 5.0, listIssue.StoryPoints)
	// Epic linkage drives buildEpicItems, so it must survive the sparse projection.
	assert.Equal(t, "DEMO-1", listIssue.EpicKey)
	assert.Equal(t, "Checkout revamp", listIssue.EpicName)
	assert.Equal(t, "To Do", listIssue.EpicStatus)

	// Fields only GetIssue can supply; the real agile payload does not include
	// them, so the board must not appear to have them either.
	assert.Empty(t, listIssue.Description)
	assert.Empty(t, listIssue.AcceptanceCriteria)
	assert.Empty(t, listIssue.Reporter)
	assert.Empty(t, listIssue.StatusChangedDate)
	assert.Empty(t, listIssue.ParentKey)
	assert.Empty(t, listIssue.ParentSummary)
	assert.Empty(t, listIssue.SubTasks)
	assert.Empty(t, listIssue.LinkedIssues)
	assert.Zero(t, listIssue.SubTaskCount)
}

func TestGetEpicsAreSortedBySummary(t *testing.T) {
	path := writeFixture(t, `project: DEMO
board:
  id: 1
  columns:
    - name: To Do
issues:
  - key: DEMO-1
    summary: zeta epic
    type: Epic
  - key: DEMO-2
    summary: Alpha epic
    type: Epic
  - key: DEMO-3
    summary: not an epic
    type: Story
`)
	c, err := New(Options{FixturePath: path})
	require.NoError(t, err)

	epics, err := c.GetEpics("DEMO", "")
	require.NoError(t, err)
	require.Len(t, epics, 2)
	assert.Equal(t, []string{"DEMO-2", "DEMO-1"}, keys(epics), "sorted by summary, like ORDER BY summary ASC")

	// The real query selects only summary and issuetype.
	assert.Equal(t, "Epic", epics[0].IssueType)
	assert.Empty(t, epics[0].Status)
	assert.Empty(t, epics[0].ProjectKey)
}

func TestCreateIssueRejectsUnknownProjectAndParent(t *testing.T) {
	c := newTestClient(t)

	_, err := c.CreateIssue("NOPE", models.IssueFields{Summary: "unknown project"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOPE")

	_, err = c.CreateIssue("DEMO", models.IssueFields{Summary: "orphan", ParentKey: "DEMO-999"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DEMO-999")

	// Neither attempt may create anything or consume the next free key.
	_, err = c.GetIssue("DEMO-12")
	require.Error(t, err)

	created, err := c.CreateIssue("DEMO", models.IssueFields{Summary: "valid"})
	require.NoError(t, err)
	assert.Equal(t, "DEMO-12", created.Key)
}

func TestUpdateIssueRejectsUnknownParentWithoutPartialChange(t *testing.T) {
	c := newTestClient(t)

	err := c.UpdateIssue("DEMO-3", models.IssueFields{Summary: "must not stick", ParentKey: "DEMO-999"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DEMO-999")

	issue := mustGet(t, c, "DEMO-3")
	assert.Equal(t, "Redesign checkout form", issue.Summary, "no field may be applied when validation fails")
	assert.Empty(t, issue.ParentKey)
}

func TestRankIssuesIsNoOpWhenTheAnchorIsAlsoRanked(t *testing.T) {
	c := newTestClient(t)

	require.NoError(t, c.RankIssues([]string{"DEMO-3"}, "", "DEMO-3"))
	assert.Equal(t, []string{"DEMO-3", "DEMO-4", "DEMO-5"}, mustActive(t, c),
		"ranking an issue against itself must not drop it from every container")
}

func TestGetBacklogIsNotImplemented(t *testing.T) {
	c := newTestClient(t)

	_, err := c.GetBacklog("DEMO")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

func TestGetActiveSprintReturnsEmptyWhenNoneActive(t *testing.T) {
	path := writeFixture(t, `project: DEMO
board:
  id: 1
  columns:
    - name: To Do
sprints:
  - id: 1
    name: Future
    state: future
issues:
  - key: DEMO-1
    summary: One
`)
	c, err := New(Options{FixturePath: path})
	require.NoError(t, err)

	issues, err := c.GetActiveSprint(1)
	require.NoError(t, err)
	assert.Empty(t, issues)
}

func TestSetAndUpdateSprints(t *testing.T) {
	c := newTestClient(t)

	created, err := c.CreateSprint(1, "Sprint 3", "2026-04-01", "2026-04-14")
	require.NoError(t, err)
	assert.Equal(t, 3, created.ID)
	assert.Equal(t, "future", created.State)

	sprints, err := c.GetSprintList(1)
	require.NoError(t, err)
	require.Len(t, sprints, 3)
	assert.Equal(t, "Sprint 3", sprints[2].Name)

	require.NoError(t, c.UpdateSprint(3, "Sprint 3 renamed", "2026-04-02", "2026-04-15"))
	sprints, err = c.GetSprintList(1)
	require.NoError(t, err)
	assert.Equal(t, "Sprint 3 renamed", sprints[2].Name)
	assert.Equal(t, "2026-04-02", sprints[2].StartDate)
	assert.Equal(t, "2026-04-15", sprints[2].EndDate)
}

func mustGet(t *testing.T, c *Client, key string) *models.Issue {
	t.Helper()
	issue, err := c.GetIssue(key)
	require.NoError(t, err)
	return issue
}

func mustActive(t *testing.T, c *Client) []string {
	t.Helper()
	issues, err := c.GetActiveSprint(1)
	require.NoError(t, err)
	return keys(issues)
}

func mustEpics(t *testing.T, c *Client, project, query string) []models.Issue {
	t.Helper()
	epics, err := c.GetEpics(project, query)
	require.NoError(t, err)
	return epics
}
