## Build & Quality

- `make build` — compile the binary
- `make test` — run all tests
- `make test-race` — run tests with race detector
- `make fmt` — format code in-place
- `make fmt-check` — check formatting without modifying files
- `make vet` — run go vet
- `make lint` — run golangci-lint (requires [golangci-lint](https://golangci-lint.run/welcome/install/))
- `make check` — run all checks (fmt, vet, lint, test) — mirrors CI
- Always run `make check` before pushing

## Architecture

- See [docs/architecture.md](docs/architecture.md) for the full architecture overview
- See [docs/tui-architecture.md](docs/tui-architecture.md) for TUI model details
- See [docs/api-client.md](docs/api-client.md) for API client implementation
- The `internal/tui` package has **NO dependencies** on other internal packages — keep it that way
- All TUI models live in `internal/app/` (package main), internal packages are pure logic
- `api.Client` is an interface — all API access goes through it for testability

### Package Structure

```
cmd/tira/              # Thin CLI layer (Cobra commands)
internal/app/          # All Bubbletea TUI models
internal/api/          # Jira API client (interface + implementation)
internal/config/       # Config loading with viper
internal/models/       # Pure data types (no logic)
internal/tui/          # TUI helpers (zero internal deps)
internal/display/      # Issue → Markdown renderer
internal/editor/       # Template rendering (pure logic)
internal/validator/    # Field validation (pure logic)
internal/debug/        # File-based debug logging
```

## API Client Conventions

For Jira API endpoints not natively supported by the go-jira library, use `c.client.NewRequest` and `c.client.Do` instead of raw `http.Client` requests:

```go
req, err := c.client.NewRequest(ctx, http.MethodPut, "rest/agile/1.0/issue/rank", payload)
if err != nil { return err }
_, err = c.client.Do(req, nil)
return err
```

`NewRequest` handles JSON encoding and base URL resolution; `Do` handles response checking.

### Hybrid API Approach

The client uses two strategies:
1. **Raw HTTP** (`c.http.Get/Do`) for endpoints needing manual JSON parsing or ADF field access
2. **go-jira `NewRequest`/`Do`** for Agile endpoints (`rest/agile/1.0/...`) that go-jira handles

### Concurrent Fetching

`GetIssue` fires 3 goroutines concurrently (issue data, comments, status change date). `GetSprintGroups` fires N+1 goroutines (one per sprint + backlog). Use `sync.WaitGroup` to coordinate.

## Code Conventions

### Color Constants

Use `internal/tui` color constants (e.g. `tui.ColorBlue`) instead of raw string literals like `"12"`:

```go
// Good
style := lipgloss.NewStyle().Foreground(tui.ColorBlue)

// Bad
style := lipgloss.NewStyle().Foreground("12")
```

### Spinner Usage

Use `tui.RunWithSpinner[T]` for any blocking operation that needs a loading indicator — do not create one-off spinner models:

```go
// Good
issue, err := tui.RunWithSpinner("Fetching issue...", func() (*models.Issue, error) {
    return client.GetIssue(key)
})

// Bad - don't create manual spinners
spinner := spinner.New()
spinner.Spinner = spinner.Dot
```

### TUI Helpers

Use `tui.FixedWidth`, `tui.Clamp`, `tui.SplitPanes` and other helpers from `internal/tui/helpers.go` instead of reimplementing:

```go
// Good
keyCol := tui.FixedWidth(issue.Key, 10)

// Bad
keyCol := fmt.Sprintf("%-10s", issue.Key)
```

### File Organization

- Backlog rendering is split: `backlog.go` (model + update) and `backlog_view.go` (rendering)
- Kanban similarly split: `kanban.go` (model + update) and `kanban_view.go` (rendering)
- Edit form split: `edit_form.go` (model) and `edit_cmds.go` (commands)
- The `board` command runs a unified TUI that wraps both backlog and kanban views — Tab toggles between them

### Editor/Validator Packages

Editor/validator packages are pure string/struct logic with no I/O — keep them testable:
- `internal/editor` — Template rendering, parsing, editor invocation
- `internal/validator` — Field validation, error annotation

## Keybindings

Any new keybindings should be updated in:
- [docs/keybindings-backlog.md](docs/keybindings-backlog.md)
- [internal/tui/help.go](internal/tui/help.go) (optional)
- Bottom persistent help (optional)

## Design Decisions and Gotchas

### 1. `internal/tui` Has Zero Internal Dependencies

This is a hard constraint. `internal/tui` is a leaf package that only imports Charm libraries. Violating this creates circular dependencies.

### 2. `markdownToADF` Is a Stub

`markdownToADF(text)` wraps text in a single ADF paragraph plain-text node. It does NOT parse Markdown. Bold/italic/lists in user's Markdown are sent as literal text to Jira.

### 3. Local Optimistic UI for Moves

When issues are moved between sprints, the backlog model updates local state immediately without waiting for API confirmation. A full refresh (`R`) reconciles.

### 4. Rank Failures Are Silent

`blRankDoneMsg` is silently discarded — the move to target sprint already happened; ranking is best-effort.

### 5. Classic vs Next-gen Projects

`cfg.ClassicProject` affects only browser URL construction. Classic projects use `/c/projects/` in the path. API calls work the same for both.

### 6. Story Points Field ID Ambiguity

Story points have no standard Jira field ID. The code tries multiple approaches:
- Name lookups: `"story points"`, `"story point estimate"`
- Direct keys: `story_points`, `customfield_10016`

### 7. Glamour Must Be Pre-initialized

`runBoardTUI` calls `glamour.NewTermRenderer(glamour.WithAutoStyle())` before starting `tea.NewProgram`. This forces `termenv`'s `sync.Once` to run while the TTY is still owned by the main goroutine.

### 8. Comments Limited to 50

`fetchComments` requests `/comment?maxResults=50&orderBy=-created`. Only the 50 most recent comments are fetched.

### 9. Debug Log Location

`debug.log` writes to the **current working directory**, not a temp or config directory. This can clutter project directories.

## Documentation

| Document | Description |
|----------|-------------|
| [docs/architecture.md](docs/architecture.md) | System architecture and package structure |
| [docs/cli-commands.md](docs/cli-commands.md) | CLI command details |
| [docs/configuration.md](docs/configuration.md) | Configuration system |
| [docs/tui-architecture.md](docs/tui-architecture.md) | TUI model architecture |
| [docs/api-client.md](docs/api-client.md) | API client implementation |
| [docs/internal-packages.md](docs/internal-packages.md) | Internal package details |
| [docs/state-machines.md](docs/state-machines.md) | State machine diagrams |
| [docs/glossary.md](docs/glossary.md) | Glossary and key types |
| [docs/keybindings-backlog.md](docs/keybindings-backlog.md) | Keybinding reference |
| [docs/go-idioms-review.md](docs/go-idioms-review.md) | Code quality review |
