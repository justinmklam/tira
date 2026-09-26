package app

import (
	"strings"
	"testing"

	"github.com/justinmklam/tira/internal/mock"
	"github.com/justinmklam/tira/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snapshotWidth and snapshotHeight are the terminal size used by the snapshot
// tests; the CLI default is the same 120x40.
const (
	snapshotWidth  = 120
	snapshotHeight = 40
)

func TestRenderBoardSnapshot(t *testing.T) {
	client, err := mock.New(mock.Options{})
	require.NoError(t, err)

	data, err := fetchAllBoardDataCore(client, 1, client.Project())
	require.NoError(t, err)
	require.NotEmpty(t, data.Groups)

	cases := []struct {
		name       string
		view       BoardView
		wantKey    string
		wantSprint string
	}{
		{name: "backlog", view: ViewBacklog, wantKey: "DEMO-3", wantSprint: "DEMO Sprint 1"},
		{name: "kanban", view: ViewKanban, wantKey: "DEMO-3", wantSprint: "DEMO Sprint 1"},
		{name: "epics", view: ViewEpics, wantKey: "DEMO-1", wantSprint: "DEMO Sprint 1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := RenderBoardSnapshot(client, 1, "https://demo.atlassian.net", client.Project(), true, data, tc.view, snapshotWidth, snapshotHeight)

			require.NotEmpty(t, out)
			assert.Contains(t, out, tc.wantSprint)
			assert.Contains(t, out, tc.wantKey)

			lines := strings.Split(out, "\n")
			require.NotEmpty(t, lines)
			for _, line := range lines {
				assert.LessOrEqual(t, tui.DisplayWidth(line), snapshotWidth, "line exceeds the snapshot width: %q", line)
			}
		})
	}
}

// TestRenderBoardSnapshotProgressiveFetch covers the path the CLI actually uses:
// app.FetchBoardData, which loads only the first batch of sprints. The backlog
// group is appended by the async lazy-load command, which a snapshot never runs,
// so it must be absent here.
func TestRenderBoardSnapshotProgressiveFetch(t *testing.T) {
	client, err := mock.New(mock.Options{})
	require.NoError(t, err)

	data, err := fetchBoardDataCore(client, 1, client.Project())
	require.NoError(t, err)
	require.NotEmpty(t, data.Groups)

	out := RenderBoardSnapshot(client, 1, "https://demo.atlassian.net", client.Project(), true, data, ViewBacklog, snapshotWidth, snapshotHeight)
	require.NotEmpty(t, out)
	assert.Contains(t, out, "DEMO Sprint 1")
	assert.Contains(t, out, "DEMO-3")
	assert.NotContains(t, out, "DEMO-7", "backlog issues arrive via the async lazy load and are absent from a snapshot")

	for _, line := range strings.Split(out, "\n") {
		assert.LessOrEqual(t, tui.DisplayWidth(line), snapshotWidth, "line exceeds the snapshot width: %q", line)
	}
}
