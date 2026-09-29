package app

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/justinmklam/tira/internal/api"
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
			out := renderBoardContent(t, client, client.Project(), data, tc.view, snapshotWidth, snapshotHeight)

			require.NotEmpty(t, out)
			assert.Contains(t, out, tc.wantSprint)
			assert.Contains(t, out, tc.wantKey)

			lines := strings.Split(out, "\n")
			require.NotEmpty(t, lines)
			assert.LessOrEqual(t, len(lines), snapshotHeight, "view renders more lines than the terminal height")
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

	out := renderBoardContent(t, client, client.Project(), data, ViewBacklog, snapshotWidth, snapshotHeight)
	require.NotEmpty(t, out)
	assert.Contains(t, out, "DEMO Sprint 1")
	assert.Contains(t, out, "DEMO-3")
	assert.NotContains(t, out, "DEMO-7", "backlog issues arrive via the async lazy load and are absent from a snapshot")

	lines := strings.Split(out, "\n")
	assert.LessOrEqual(t, len(lines), snapshotHeight, "view renders more lines than the terminal height")
	for _, line := range lines {
		assert.LessOrEqual(t, tui.DisplayWidth(line), snapshotWidth, "line exceeds the snapshot width: %q", line)
	}
}

// renderBoardContent renders the board without RenderBoardSnapshot's per-line
// clipping, so width and height assertions can actually fail. RenderBoardSnapshot
// truncates every line to the terminal width and never clips the line count.
func renderBoardContent(t *testing.T, client api.Client, project string, data BoardInitData, view BoardView, w, h int) string {
	t.Helper()
	return sizedBoardModel(t, client, project, data, view, w, h).View().Content
}

// sizedBoardModel builds the board and applies the viewport, returning the model
// itself so tests can assert against the rendered geometry.
func sizedBoardModel(t *testing.T, client api.Client, project string, data BoardInitData, view BoardView, w, h int) boardModel {
	t.Helper()
	m, _ := newBoardModel(client, 1, "https://demo.atlassian.net", project, true, data, view, "")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	board, ok := updated.(boardModel)
	if !ok {
		t.Fatalf("board model did not survive the size update")
	}
	return board
}

// TestRenderBoardSnapshotThemeMatrix is the width/height regression gate across
// every theme, viewport, and view. It runs on un-truncated content, so a row or
// footer that overflows actually fails here.
func TestRenderBoardSnapshotThemeMatrix(t *testing.T) {
	client, err := mock.New(mock.Options{})
	require.NoError(t, err)

	data, err := fetchAllBoardDataCore(client, 1, client.Project())
	require.NoError(t, err)
	require.NotEmpty(t, data.Groups)

	sizes := []struct{ w, h int }{{120, 40}, {80, 24}}
	views := []struct {
		name string
		view BoardView
	}{
		{"backlog", ViewBacklog},
		{"kanban", ViewKanban},
		{"epics", ViewEpics},
	}

	for _, theme := range tui.ThemeNames() {
		for _, size := range sizes {
			for _, v := range views {
				t.Run(fmt.Sprintf("%s/%dx%d/%s", theme, size.w, size.h, v.name), func(t *testing.T) {
					require.NoError(t, tui.SetTheme(theme))
					t.Cleanup(func() { _ = tui.SetTheme("default") })

					board := sizedBoardModel(t, client, client.Project(), data, v.view, size.w, size.h)
					content := board.View().Content
					lines := strings.Split(content, "\n")

					require.LessOrEqual(t, len(lines), size.h, "view renders more lines than the terminal height")
					for _, line := range lines {
						require.LessOrEqual(t, tui.DisplayWidth(line), size.w, "line exceeds the terminal width: %q", line)
					}

					// Index 0 is the top pad, index 1 the tab strip, and index 2 the
					// topmost rule: the frame's top border for the split views and the
					// columns' top border for kanban (R2 removed the TabDivider).
					require.GreaterOrEqual(t, len(lines), 3)
					rule := ansi.Strip(lines[2])
					require.True(t, strings.HasPrefix(rule, "╭"), "index 2 is not the top frame border: %q", rule)
					switch v.view {
					case ViewBacklog, ViewEpics:
						require.Equal(t, 1, strings.Count(rule, "Details"), "the detail pane title should appear once: %q", rule)
						require.NotContains(t, rule, "Backlog")
						require.NotContains(t, rule, "Epics")
					case ViewKanban:
						require.Equal(t, len(board.kanban.columns), strings.Count(ansi.Strip(content), "╭"),
							"one frame per kanban column and none per card (R1)")
					}

					// The kanban view pads between the board and the footer, so the
					// empty-line check applies to the split-pane views only.
					if v.view != ViewKanban {
						for i := 1; i < len(lines)-1; i++ {
							require.NotEqual(t, "", lines[i], "empty line in the middle of the body at %d", i)
						}
					}
				})
			}
		}
	}
}

// TestKanbanColumnTitlesDifferByStatusColour asserts the columns carry their own
// status hue. It compares the two columns' escape sequences to each other rather
// than to a literal, so it survives a palette change.
func TestKanbanColumnTitlesDifferByStatusColour(t *testing.T) {
	client, err := mock.New(mock.Options{})
	require.NoError(t, err)

	data, err := fetchAllBoardDataCore(client, 1, client.Project())
	require.NoError(t, err)

	require.NoError(t, tui.SetTheme("default"))
	t.Cleanup(func() { _ = tui.SetTheme("default") })

	content := renderBoardContent(t, client, client.Project(), data, ViewKanban, 120, 40)

	var todoLine, doneLine string
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "TO DO (") {
			todoLine = line
		}
		if strings.Contains(line, "DONE (") {
			doneLine = line
		}
	}
	require.NotEmpty(t, todoLine, "To Do title row not found")
	require.NotEmpty(t, doneLine, "Done title row not found")

	require.NotEqual(t, ansiBefore(todoLine, "TO DO"), ansiBefore(doneLine, "DONE"),
		"To Do and Done titles render in the same colour")
}

// ansiBefore returns the last ANSI escape sequence that precedes name in line.
func ansiBefore(line, name string) string {
	idx := strings.Index(line, name)
	if idx < 0 {
		return ""
	}
	before := line[:idx]
	last := strings.LastIndex(before, "\x1b[")
	if last < 0 {
		return ""
	}
	return before[last:]
}
