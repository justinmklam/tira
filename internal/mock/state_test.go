package mock

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/justinmklam/tira/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateRoundTripsMutationsAcrossClients(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "nested", "state.json")

	c, err := New(Options{StatePath: statePath})
	require.NoError(t, err)
	assert.Equal(t, "embedded demo fixture", c.Source())
	assert.NoFileExists(t, statePath, "the state file stays untouched until the first mutation")

	require.NoError(t, c.UpdateIssue("DEMO-3", models.IssueFields{Summary: "Persisted summary"}))
	require.NoError(t, c.SetLabels("DEMO-3", []string{"persisted"}))
	require.NoError(t, c.AddComment("DEMO-3", "persisted comment"))
	require.NoError(t, c.MoveIssuesToSprint(1, []string{"DEMO-7"}))
	require.FileExists(t, statePath)

	// A second client pointed at the same state file resumes the mutation.
	reopened, err := New(Options{StatePath: statePath})
	require.NoError(t, err)
	assert.Contains(t, reopened.Source(), "state file")
	assert.Contains(t, reopened.Source(), statePath)

	issue := mustGet(t, reopened, "DEMO-3")
	assert.Equal(t, "Persisted summary", issue.Summary)
	assert.Equal(t, []string{"persisted"}, issue.Labels)
	require.Len(t, issue.Comments, 1)
	assert.Equal(t, "persisted comment", issue.Comments[0].Body)

	backlog, err := reopened.GetBacklogIssues(1)
	require.NoError(t, err)
	assert.NotContains(t, keys(backlog), "DEMO-7")
}

func TestStateFileWinsOverFixtures(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	c, err := New(Options{StatePath: statePath})
	require.NoError(t, err)
	require.NoError(t, c.UpdateIssue("DEMO-1", models.IssueFields{Summary: "From state"}))

	// A fixture that declares a different summary must be ignored while the
	// state file exists.
	fixturePath := writeFixture(t, minimalFixture+`  - key: DEMO-2
    summary: Second
`)
	reopened, err := New(Options{FixturePath: fixturePath, StatePath: statePath})
	require.NoError(t, err)
	assert.Equal(t, "From state", mustGet(t, reopened, "DEMO-1").Summary)
}

func TestStateSaveFailureKeepsInMemoryChange(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")

	c, err := New(Options{StatePath: statePath})
	require.NoError(t, err)

	// Turn the state path into an existing directory so the atomic rename fails
	// (deterministic regardless of the test user's privileges).
	require.NoError(t, os.Mkdir(statePath, 0o750))

	err = c.UpdateIssue("DEMO-3", models.IssueFields{Summary: "Applied but unsaved"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "persisting dev state")

	assert.Equal(t, "Applied but unsaved", mustGet(t, c, "DEMO-3").Summary,
		"the in-memory change is kept even when the state write fails")
}

func TestStateRejectsUnknownKeys(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, os.WriteFile(statePath, []byte(`{
  "project": "DEMO",
  "bogus_key": true,
  "board": {"id": 1, "columns": [{"name": "To Do"}]}
}`), 0o600))

	_, err := New(Options{StatePath: statePath})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus_key")
}

func TestEmptyStateFileFallsBackToFixtures(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, os.WriteFile(statePath, nil, 0o600))

	c, err := New(Options{StatePath: statePath})
	require.NoError(t, err)
	assert.Equal(t, "embedded demo fixture", c.Source())
	assert.Equal(t, "Checkout revamp", mustGet(t, c, "DEMO-1").Summary)
}

func TestStateDirectoryPathErrors(t *testing.T) {
	dir := t.TempDir()

	_, err := New(Options{StatePath: dir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a directory")
}

func TestRejectedMutationsDoNotBrickTheStateFile(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")

	c, err := New(Options{StatePath: statePath})
	require.NoError(t, err)
	require.NoError(t, c.UpdateIssue("DEMO-3", models.IssueFields{Summary: "persisted"}))
	require.FileExists(t, statePath)

	// These would violate fixture integrity, so they must be rejected before
	// anything is written; otherwise the next run could not load the state file.
	_, err = c.CreateIssue("DEMO", models.IssueFields{Summary: "orphan", ParentKey: "DEMO-999"})
	require.Error(t, err)
	require.Error(t, c.UpdateIssue("DEMO-3", models.IssueFields{Summary: "x", ParentKey: "NOPE-1"}))
	require.Error(t, c.BulkUpdateIssue([]string{"DEMO-4"}, models.IssueFields{ParentKey: "DEMO-999"})[0])

	reopened, err := New(Options{StatePath: statePath})
	require.NoError(t, err, "the state written before the rejected mutations must still load")
	assert.Equal(t, "persisted", mustGet(t, reopened, "DEMO-3").Summary)
}

func TestStateFileIsValidJSON(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")

	c, err := New(Options{StatePath: statePath})
	require.NoError(t, err)
	_, err = c.CreateSprint(1, "Sprint 3", "2026-04-01", "2026-04-14")
	require.NoError(t, err)

	data, err := os.ReadFile(statePath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "\n  \"project\": \"DEMO\"", "state is indented JSON")

	// And the saved state reloads through the same validation as a fixture.
	reopened, err := New(Options{StatePath: statePath})
	require.NoError(t, err)
	sprints, err := reopened.GetSprintList(1)
	require.NoError(t, err)
	assert.Len(t, sprints, 3)
}
