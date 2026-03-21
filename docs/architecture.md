# tira — architecture

## Overview

tira is a terminal UI for Jira built in Go with the Charm ecosystem (Bubbletea, Bubbles, Lipgloss, Glamour). It provides two interactive views (backlog and kanban) under a unified board TUI, plus CLI commands for fetching, editing, and creating issues via `$EDITOR`.

---

## Project layout

```
tira/
├── cmd/tira/                  # Thin CLI layer — Cobra commands + config
│   ├── main.go                # Entry point → Execute()
│   ├── root.go                # Cobra root command, --debug flag, config loading
│   ├── board.go               # board/backlog/kanban Cobra commands (calls into internal/app)
│   ├── get.go                 # `get <key> [--edit]` — fetch/display/edit single issue
│   └── create.go              # `create` — new issue via $EDITOR template
│
├── internal/
│   ├── app/                   # All Bubbletea TUI models
│   │   ├── board.go           # boardModel type + Init + Update + FetchBoardData + RunBoardTUI
│   │   ├── board_overlays.go  # boardModel overlay rendering (edit form, assignee, help, comment)
│   │   ├── backlog.go         # blModel type + Init + Update dispatch + helpers
│   │   ├── backlog_update.go  # blModel sub-update handlers (list, filter, detail, pickers, move/rank)
│   │   ├── backlog_view.go    # blModel View + render helpers
│   │   ├── kanban.go          # kanbanModel type + Init + Update
│   │   ├── kanban_view.go     # kanbanModel View + render helpers
│   │   ├── edit_form.go       # editModel (TUI form widget)
│   │   ├── edit_cmds.go       # editFormState, msg types, tea.Cmd funcs, pickers
│   │   └── comment_form.go    # commentInputModel
│   ├── api/
│   │   ├── client.go          # Jira API client (interface + jiraClient impl)
│   │   └── adf.go             # Atlassian Document Format → Markdown converter
│   ├── config/
│   │   └── config.go          # Config loading (env vars + optional YAML)
│   ├── models/
│   │   └── models.go          # Shared data types (Issue, IssueFields, Sprint, etc.)
│   ├── tui/
│   │   ├── spinner.go         # Generic RunWithSpinner[T] for async operations
│   │   ├── styles.go          # Centralized color constants and reusable styles
│   │   └── helpers.go         # Shared TUI utilities (FixedWidth, Clamp, SplitPanes)
│   ├── display/
│   │   └── issue.go           # Issue → Markdown renderer (for detail pane + pager)
│   ├── editor/
│   │   ├── template.go        # Issue → editable markdown template
│   │   ├── parse.go           # Parse edited template → IssueFields
│   │   └── open.go            # Open $EDITOR and block until exit
│   └── validator/
│       ├── validate.go        # Field validation against valid values
│       └── annotate.go        # Inline error annotation in template files
│
├── docs/
│   ├── architecture.md        # This file
│   └── keybindings-backlog.md
├── go.mod
├── Makefile
├── CLAUDE.md
└── config.example.yaml
```

---

## Package dependency graph

```
cmd/tira (thin CLI layer — Cobra commands + config)
 ├── internal/app         ← TUI models (board, backlog, kanban, edit form, comment)
 ├── internal/api         ← Jira REST API client
 ├── internal/config      ← Config loading
 └── internal/tui         ← RunWithSpinner used directly by CLI commands

internal/app
 ├── internal/api
 ├── internal/models
 ├── internal/tui          ← styles, helpers, picker (still zero internal deps)
 ├── internal/display
 └── internal/debug

internal/api
 ├── internal/config
 └── internal/models

internal/tui              ← NO dependencies on other internal packages
 └── (only charmbracelet/bubbles, lipgloss)

internal/display
 └── internal/models

internal/editor
 └── internal/models

internal/validator        ← Pure logic, no TUI dependency
 └── internal/models
```

Key invariants:
- `internal/tui` has **zero** dependencies on other internal packages — it's a leaf.
- `internal/editor` and `internal/validator` are pure string/struct logic — no I/O, no TUI.
- `internal/api` depends only on `config` and `models` — no TUI or display coupling.
- All TUI model code lives in `internal/app/` — `cmd/tira/` is a thin CLI layer (Cobra commands + config).

---

## TUI architecture

### Unified board model

The board TUI runs a single `tea.Program` that wraps both the backlog and kanban views. This allows toggling between views without exiting or re-fetching data.

```
boardModel (top-level tea.Model)
 ├── activeView: backlog | kanban
 ├── blModel      ← backlog tree + multi-select + move operations
 ├── kanbanModel  ← kanban columns
 ├── client       ← shared API client
 └── shared state (boardID, sprint data, detail pane)
```

**View switching**: `Tab` toggles between views. `1` and `2` switch directly. Data is shared — the backlog's sprint groups and kanban's active sprint are fetched once and shared between views.

**State machine** (per sub-view):

```
blModel:     blList → blFilter (/) → blList
                    → blLoading (enter) → blDetail (esc) → blList

kanbanModel: stateBoard → stateLoading (enter) → stateDetail (esc) → stateBoard
```

### Bubbletea patterns used

1. **Async fetch via Cmd**: API calls run in goroutines, results delivered as `tea.Msg`.
2. **Spinner overlay**: `spinner.Model` ticks during loading states.
3. **Viewport scrolling**: Detail pane uses `viewport.Model` for scrollable content.
4. **Text input**: Filter bar uses `textinput.Model`.
5. **Split pane layout**: `tui.SplitPanes()` renders list + detail side-by-side.

### Generic spinner

`tui.RunWithSpinner[T]` eliminates boilerplate for any blocking operation:

```go
issues, err := tui.RunWithSpinner("Fetching…", func() ([]Issue, error) {
    return client.GetActiveSprint(boardID)
})
```

Uses Go generics to avoid per-type spinner model duplication.

---

## API client

`api.Client` is an interface with a single implementation (`jiraClient`). This enables testing with mock clients.

```go
type Client interface {
    GetIssue(key string) (*models.Issue, error)
    UpdateIssue(key string, fields models.IssueFields) error
    CreateIssue(projectKey string, fields models.IssueFields) (*models.Issue, error)
    GetValidValues(projectKey string) (*models.ValidValues, error)
    GetBoardColumns(boardID int) ([]models.BoardColumn, error)
    GetActiveSprint(boardID int) ([]models.Issue, error)
    GetSprintGroups(boardID int) ([]models.SprintGroup, error)
    MoveIssuesToSprint(sprintID int, keys []string) error
    MoveIssuesToBacklog(keys []string) error
}
```

**Hybrid approach**: Uses `go-jira` for structured fields, raw HTTP for ADF (Atlassian Document Format) fields that go-jira can't decode. Sprint group fetches run concurrently with `sync.WaitGroup`.

---

## Editor flow

The `get --edit` and `create` commands share the same template-based editing loop:

```
RenderTemplate() → WriteTempFile() → OpenEditor() → ReadFile()
    → ParseTemplate() → Validate() → [errors? AnnotateTemplate() → loop]
    → ResolveAssigneeID() → UpdateIssue/CreateIssue
```

- **Template format**: YAML-like front matter + markdown body, separated by `---`
- **Sentinel line**: `<!-- tira: ... -->` detects template corruption
- **Validation loop**: Errors annotated inline, user can fix and re-save
- **Pure functions**: `ParseTemplate()` and `Validate()` take strings/structs, return values — no I/O.

---

## Authentication

Stateless, environment-variable based:

```bash
export JIRA_URL=https://yourorg.atlassian.net
export JIRA_EMAIL=you@example.com
export JIRA_API_TOKEN=your_token_here
```

Optional YAML config at `~/.config/tira/config.yaml` for non-secret defaults:

```yaml
default_project: MP
default_board_id: 42
```

---

## Shared TUI infrastructure (`internal/tui`)

| File | Purpose |
|------|---------|
| `spinner.go` | `RunWithSpinner[T]` — generic async spinner for any blocking operation |
| `styles.go` | Color constants (`ColorRed`, `ColorBlue`, etc.), shared styles (`DimStyle`, `BoldBlue`), `IssueTypeColor()`, `EpicColor()` |
| `helpers.go` | `FixedWidth`, `Clamp`, `SplitPanes`, `ListPaneWidth`, `DetailPaneWidth`, `ContainsCI` |

Design constraint: This package has **no dependencies** on other internal packages. It only imports Charm libraries. This keeps it reusable and prevents circular dependencies.
