# Internal Packages

This document covers the internal packages that support the TUI and CLI commands.

---

## Package Structure

```
internal/
├── tui/           # Shared TUI infrastructure (zero deps on other internal pkgs)
├── models/        # Pure data types (no logic)
├── display/       # Issue → Markdown renderer
├── editor/        # Template rendering and editor integration
├── validator/     # Field validation and error annotation
└── debug/         # File-based debug logging
```

---

## Shared TUI Infrastructure (`internal/tui`)

**Design constraint:** This package has **zero dependencies** on other internal packages. It only imports Charm libraries. This keeps it reusable and prevents circular dependencies.

### spinner.go — Generic Spinner

```go
func RunWithSpinner[T any](label string, fn func() (T, error)) (T, error)
```

Runs `fn` in a goroutine while displaying a spinner in the terminal. Used for all blocking pre-TUI operations (config load, initial data fetch, issue fetch for pager view).

Uses Go generics to avoid per-type spinner model duplication. The spinner runs its own `tea.Program` on stderr, which exits when the goroutine result arrives.

**Usage:**
```go
issue, err := tui.RunWithSpinner("Fetching issue...", func() (*models.Issue, error) {
    return client.GetIssue(key)
})
```

### styles.go — Color Constants and Styles

**Color constants** (terminal 256-color palette indices):

| Constant | Value | Use |
|----------|-------|-----|
| `SpinnerColor` | `12` (blue) | Spinner dots |
| `ColorRed` | `9` | Errors, bugs |
| `ColorGreen` | `10` | Success, active sprint, stories |
| `ColorYellow` | `11` | Selection, sub-tasks |
| `ColorBlue` | `12` | Keys, headers, borders |
| `ColorMagenta` | `13` | Epics, visual mode indicator |
| `ColorOrange` | `208` | Cut indicator, aging issues (6-9 days) |
| `ColorWhite` | `15` | Selected row text |
| `ColorFg` | `252` | Normal foreground |
| `ColorFgBright` | `255` | Bright foreground |
| `ColorDim` | `244` | Secondary text |
| `ColorDimmer` | `240` | Very dim (separators) |
| `ColorBg` | `237` | Selected row background |

**Reusable styles:**
- `DimStyle` — foreground `ColorDim`
- `BoldBlue` — bold foreground `ColorBlue`
- `SelectedBg` — background `ColorBg`

**Helper functions:**
- `IssueTypeColor(issueType)` — Maps `"bug"→Red`, `"story"→Green`, `"task"→Blue`, `"epic"→Magenta`, `"sub-task"/"subtask"→Yellow`, default→Dim
- `EpicColor(epicKey)` — Deterministic 10-color hash of epic key
- `DaysInColumn(statusChangedDate)` — Days since status changed (from ISO date string)
- `DaysColor(days)` — Green (0-2), Yellow (3-5), Orange (6-9), Red (10+)

### helpers.go — Layout Utilities

| Function | Description |
|----------|-------------|
| `FixedWidth(s string, n int) string` | Pad/truncate to exactly `n` runes; uses `…` for overflow |
| `Clamp(v, lo, hi int) int` | Constrain `v` to `[lo, hi]` |
| `SplitPanes(left, right string, leftWidth, height int) string` | Side-by-side layout with dim `│` separator |
| `ListPaneWidth(totalWidth int) int` | 40% of total, min 30 |
| `DetailPaneWidth(totalWidth int) int` | Remainder after list pane |
| `OverlaySize(w, h int) (w, h int)` | 85% width (max 140, min 60); 95% height (min 15) |
| `OverlayViewportSize(w, h int) (vpW, vpH int)` | Subtracts border + chrome from OverlaySize |
| `HelpOverlaySize(w, h int) (w, h int)` | 70% width (max 100, min 60); 90% height (max 50, min 25) |
| `ContainsCI(list []string, val string) bool` | Case-insensitive membership check |

### picker.go — Reusable Search Picker

`PickerModel` is a reusable search picker with:
- A `textinput.Model` for the search query
- Debounced server-side search (default 300ms debounce)
- Stale result rejection via `searchToken` and `debounceToken` integers
- Optional `NoneItem` prepended (shown only when query is empty, returns `nil` from `SelectedItem`)
- `InitialValue` positions cursor on matching item when results load
- `Completed`/`Aborted` flags set by Enter/Esc

**View** takes `(innerW, maxListRows int)` — does NOT include a border; caller wraps it in a lipgloss bordered box.

**Used for:** assignee selection, parent/epic selection, status transition selection, epic filter selection.

### help.go — Scrollable Help Overlay

`HelpModel` is a simple scrollable overlay. `HelpSections()` returns the static list of keybinding sections. `View(innerW, innerH)` renders the visible window based on `ScrollOffset`. `Update(msg, innerH)` handles `j`/`k`, `Ctrl+D`/`Ctrl+U`, `g`/`G` for scrolling.

---

## Data Models (`internal/models`)

**File:** `internal/models/models.go`

All types are pure data structs — no methods, no logic.

### Core Types

```go
type Issue struct {
    Key, Summary, Description, AcceptanceCriteria string
    Status, StatusID                               string
    IssueType, Priority                            string
    Assignee, AssigneeID                           string
    Reporter                                       string
    StoryPoints                                    float64
    Labels                                         []string
    EpicKey, EpicName                              string
    SprintName                                     string
    ParentKey, ParentSummary                       string
    LinkedIssues                                   []LinkedIssue
    Comments                                       []Comment
    StatusChangedDate                              string // ISO date "YYYY-MM-DD"
}

type IssueFields struct {
    Summary, IssueType, Priority   string
    Assignee   string  // display name (resolved to AssigneeID before API call)
    AssigneeID string
    StoryPoints float64
    Labels      []string
    Description, AcceptanceCriteria string
    ParentKey   string
}

type ValidValues struct {
    IssueTypes []string
    Priorities []string
    Assignees  []Assignee
    Sprints    []Sprint
}

type Sprint struct {
    ID        int
    Name      string
    State     string  // "active" | "future" | "closed" | "backlog"
    StartDate string  // "YYYY-MM-DD"
    EndDate   string  // "YYYY-MM-DD"
}

type SprintGroup struct {
    Sprint Sprint
    Issues []Issue
}

type BoardColumn struct {
    Name      string
    StatusIDs []string
}

type Status struct {
    ID   string  // transition ID (not status ID)
    Name string
}
```

**Important distinction:** `Status.ID` in `models.Status` is the **transition ID** (from `/transitions` endpoint), not the Jira status ID. `Issue.StatusID` is the actual Jira status ID used for kanban column mapping.

---

## Display Package (`internal/display`)

**File:** `internal/display/issue.go`

`RenderIssue(issue *models.Issue) string` produces a pure Markdown string.

### Output Structure

1. **Metadata list** (Assignee, Status, Story Points, Type, Priority, Reporter, Sprint, Parent, Labels) — aligned with non-breaking spaces so goldmark doesn't collapse padding
2. `# Description` section
3. `# Acceptance Criteria` section (if non-empty)
4. `# Linked Work Items` section (if any)
5. `# Comments` section (with author, formatted timestamp, body)

Comments are sorted newest-first (Jira returns them `orderBy=-created`). Each comment is separated by `---`.

**Timestamp formatting:** tries multiple Jira timestamp formats in order, falls back to the raw string.

---

## Editor Package (`internal/editor`)

**Files:** `template.go`, `parse.go`, `open.go`

The editor package handles template-based issue editing. It is **pure string/struct logic with no I/O** (except `OpenEditor` which execs a process).

### Template Format

The template is a plain text file with two sections separated by `---`:

```markdown
<!-- tira: do not remove this line or change field names -->
<!-- Valid types: Bug, Story, Task, Epic, Sub-task -->
type: Story

<!-- Valid priorities: Highest, High, Medium, Low, Lowest -->
priority: Medium

assignee: Jane Smith

<!-- Enter a number or leave blank -->
story_points: 5

<!-- Comma-separated, e.g. backend, auth -->
labels: backend, auth

<!-- parent: MP-42: Parent issue summary -->

---

# MP-101: Issue summary here

## Description

Description text here...

## Acceptance Criteria

Criteria here...

## Linked Work Items

<!-- blocks MP-50: Other issue (In Progress) -->
```

**Key design points:**
- The sentinel comment `<!-- tira: do not remove this line or change field names -->` is checked by `ParseTemplate` to detect template corruption
- Comments (lines starting with `<!--`) are ignored during parsing
- The `---` separator on its own line divides front-matter from body
- The H1 heading (`# KEY: Summary`) provides the Summary field during parse
- `## Description` and `## Acceptance Criteria` sections are extracted by `extractSection`
- `## Linked Work Items` is read-only (comments only, not parsed into writable fields)
- Parent is shown as a read-only comment, not editable through the template

### Functions

| Function | Description |
|----------|-------------|
| `RenderTemplate(issue *models.Issue, valid *models.ValidValues) string` | Renders an issue to editable template format |
| `WriteTempFile(content string) (string, error)` | Writes template to temp file, returns path |
| `ParseTemplate(content string) (*models.IssueFields, error)` | Parses edited template back to IssueFields |
| `OpenEditor(filepath string) error` | Opens `$EDITOR` and blocks until exit |

### Editor Selection

`OpenEditor` resolves the editor via `$EDITOR` → `$VISUAL` → `vi`. It supports editors with arguments (e.g., `EDITOR="code --wait"`).

---

## Validator Package (`internal/validator`)

**Files:** `validate.go`, `annotate.go`

The validator package provides field validation and error annotation. It is **pure logic with no TUI dependency**.

### Validation

`Validate(fields *models.IssueFields, valid *models.ValidValues) []ValidationError` checks:
- `IssueType` must be in `valid.IssueTypes` (case-insensitive)
- `Priority` must be in `valid.Priorities` (case-insensitive)
- `Assignee` must be in `valid.Assignees` display names (case-insensitive)
- `StoryPoints` must be non-negative

### Error Annotation

`AnnotateTemplate(content string, errs []ValidationError) string` rewrites the template by inserting `<!-- ERROR: ... -->` comments directly above the failing field line. It replaces existing hint comments to prevent stacking on repeat failures.

### Assignee Resolution

`ResolveAssigneeID(fields *models.IssueFields, valid *models.ValidValues) error` translates the display name typed by the user into a Jira `accountId` needed for the API call.

### containsCI Duplication

The `containsCI` function exists in both `internal/tui/helpers.go` and `internal/validator/validate.go`. This is intentional — the architecture forbids `validator` from importing `tui`.

---

## Debug Package (`internal/debug`)

**File:** `internal/debug/logger.go`

File-based debug logger for troubleshooting.

### Functions

| Function | Description |
|----------|-------------|
| `Init() error` | Creates `./debug.log` (overwrites) on first call; safe to call multiple times |
| `Logf(format string, args ...any)` | Write formatted log with timestamp |
| `Log(msg string)` | Write plain log message |
| `LogError(err error)` | Write error log |
| `LogWarning(msg string)` | Write warning log |
| `IsEnabled() bool` | Returns `true` if initialized |
| `Close() error` | Close the log file |

### HTTP Transport

`Transport` implements `http.RoundTripper`; logs all requests (method, URL, body) when debug is enabled. Wraps the base transport for inspection.

### Log File Location

The debug logger writes to `./debug.log` in the **current working directory**, not in a temp or config directory. This can clutter project directories.

---

## Package Dependencies

```
internal/tui              ← NO dependencies on other internal packages
 └── (only charmbracelet/bubbles, lipgloss)

internal/models           ← Pure data types, no dependencies

internal/display
 └── internal/models

internal/editor
 └── internal/models

internal/validator
 └── internal/models

internal/debug            ← No internal dependencies

internal/api
 ├── internal/config
 ├── internal/models
 └── internal/debug
```

---

## See Also

- [API Client](api-client.md) — How API client uses models and debug
- [TUI Architecture](tui-architecture.md) — How TUI models use tui helpers
- [Architecture](architecture.md) — Overall package dependency graph
