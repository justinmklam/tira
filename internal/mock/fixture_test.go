package mock

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFixture writes content to a temporary YAML file and returns its path.
func writeFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

const minimalFixture = `project: DEMO
board:
  id: 1
  columns:
    - name: To Do
      statuses: [To Do]
issues:
  - key: DEMO-1
    summary: Only issue
`

func TestLoadEmbeddedFixture(t *testing.T) {
	f, err := Load("")
	require.NoError(t, err)
	require.NotNil(t, f)

	assert.Equal(t, "DEMO", f.Project)
	assert.Equal(t, 1, f.Board.ID)
	assert.Len(t, f.Board.Columns, 3)
	assert.GreaterOrEqual(t, len(f.Issues), 8)
	assert.Len(t, f.Users, 3)
	assert.Len(t, f.Transitions["default"], 3)

	epics := 0
	var withAC, withComments, withSubtasks, withLinks, done int
	for _, issue := range f.Issues {
		if issue.Type == "Epic" {
			epics++
		}
		if issue.AcceptanceCriteria != "" {
			withAC++
		}
		if len(issue.Comments) >= 2 {
			withComments++
		}
		if len(issue.Subtasks) >= 2 {
			withSubtasks++
		}
		if len(issue.Links) > 0 {
			withLinks++
		}
		if issue.Status == "Done" {
			done++
		}
	}
	assert.Equal(t, 2, epics, "fixture should declare two epics")
	assert.GreaterOrEqual(t, withAC, 1)
	assert.GreaterOrEqual(t, withComments, 1)
	assert.GreaterOrEqual(t, withSubtasks, 1)
	assert.GreaterOrEqual(t, withLinks, 1)
	assert.GreaterOrEqual(t, done, 1, "fixture needs a Done issue for the Done column")

	assert.Empty(t, f.Priorities, "priorities are derived by the client, not declared")
	assert.Empty(t, f.IssueTypes, "issue_types are derived by the client, not declared")
}

func TestLoadFromPath(t *testing.T) {
	path := writeFixture(t, minimalFixture)

	f, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "DEMO", f.Project)
	require.Len(t, f.Issues, 1)
	assert.Equal(t, "DEMO-1", f.Issues[0].Key)
	assert.Equal(t, path, f.source)
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	path := writeFixture(t, `project: DEMO
bogus_key: true
board:
  id: 1
  columns:
    - name: To Do
`)

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus_key")
}

func TestLoadRejectsDirectory(t *testing.T) {
	dir := t.TempDir()

	_, err := Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a directory")
}

func TestLoadRejectsMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	require.Error(t, err)
}

func TestLoadRejectsEmptyFile(t *testing.T) {
	_, err := Load(writeFixture(t, ""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestLoadRejectsDanglingReferences(t *testing.T) {
	cases := map[string]string{
		"dangling epic": `project: DEMO
board:
  id: 1
  columns:
    - name: To Do
issues:
  - key: DEMO-1
    summary: One
    epic: DEMO-999
`,
		"dangling parent": `project: DEMO
board:
  id: 1
  columns:
    - name: To Do
issues:
  - key: DEMO-1
    summary: One
    parent: DEMO-999
`,
		"dangling link": `project: DEMO
board:
  id: 1
  columns:
    - name: To Do
issues:
  - key: DEMO-1
    summary: One
    links:
      - relationship: blocks
        key: DEMO-999
`,
		"dangling subtask": `project: DEMO
board:
  id: 1
  columns:
    - name: To Do
issues:
  - key: DEMO-1
    summary: One
    subtasks: [DEMO-999]
`,
		"dangling backlog entry": `project: DEMO
board:
  id: 1
  columns:
    - name: To Do
backlog: [DEMO-999]
issues:
  - key: DEMO-1
    summary: One
`,
		"dangling sprint issue": `project: DEMO
board:
  id: 1
  columns:
    - name: To Do
sprints:
  - id: 1
    name: S1
    state: active
    issues: [DEMO-999]
issues:
  - key: DEMO-1
    summary: One
`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeFixture(t, content))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "DEMO-999")
		})
	}
}

func TestValidateRejectsStructuralProblems(t *testing.T) {
	cases := map[string]struct {
		content string
		want    string
	}{
		"missing project": {
			content: `board:
  id: 1
  columns:
    - name: To Do
`,
			want: "project is required",
		},
		"invalid board id": {
			content: `project: DEMO
board:
  id: 0
  columns:
    - name: To Do
`,
			want: "board.id",
		},
		"no columns": {
			content: `project: DEMO
board:
  id: 1
`,
			want: "board.columns",
		},
		"duplicate key": {
			content: `project: DEMO
board:
  id: 1
  columns:
    - name: To Do
issues:
  - key: DEMO-1
    summary: One
  - key: DEMO-1
    summary: Two
`,
			want: "duplicate issue key",
		},
		"foreign project key": {
			content: `project: DEMO
board:
  id: 1
  columns:
    - name: To Do
issues:
  - key: OTHER-1
    summary: One
`,
			want: "does not start with project prefix",
		},
		"missing key": {
			content: `project: DEMO
board:
  id: 1
  columns:
    - name: To Do
issues:
  - summary: One
`,
			want: "must have a key",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeFixture(t, tc.content))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
