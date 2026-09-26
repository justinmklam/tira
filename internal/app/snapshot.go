package app

import (
	"strings"

	"charm.land/bubbletea/v2"
	"github.com/justinmklam/tira/internal/api"
	"github.com/justinmklam/tira/internal/tui"
)

// RenderBoardSnapshot renders one board frame at the given terminal size
// without starting a Bubble Tea program. Only state available synchronously
// after the initial board fetch is rendered; regions populated by async
// tea.Cmds (the sidebar detail, epic children, and lazy-loaded sprint batches)
// are empty.
//
// The frame is clipped to width cells because some view footers are wider than
// a narrow terminal and would otherwise wrap into stray fragments. It returns
// "" if the model does not survive the size update, so callers never panic on
// an unexpected model type.
func RenderBoardSnapshot(client api.Client, boardID int, jiraURL, project string,
	classicProject bool, data BoardInitData, view BoardView, width, height int) string {
	m, _ := newBoardModel(client, boardID, jiraURL, project, classicProject, data, view)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	board, ok := updated.(boardModel)
	if !ok {
		return ""
	}

	content := board.View().Content
	if width <= 0 {
		return content
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = tui.TruncateWidth(line, width)
	}
	return strings.Join(lines, "\n")
}
