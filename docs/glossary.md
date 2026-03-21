# Glossary and Key Types

This document provides a glossary of terms and key types used throughout the tira codebase.

---

## Glossary

| Term | Meaning |
|------|---------|
| **ADF** | Atlassian Document Format — Jira's rich text JSON format |
| **Agile API** | `rest/agile/1.0/...` endpoints for sprint/board operations |
| **blModel** | The backlog TUI model (`bl` = backlog) |
| **blRow** | A single row in the backlog flat list (sprint header, issue, or spacer) |
| **blResult** | Struct returned by blModel to boardModel to signal cross-boundary actions |
| **Board** | A Jira software board; has an ID and associated columns |
| **BoardColumn** | A column in the kanban view with a name and a list of Jira status IDs |
| **boardView** | Enum of all top-level states of boardModel |
| **Classic project** | A Jira "company-managed" project (older project type) |
| **cut/paste** | Backlog move paradigm: `x` marks issues as cut, `p` pastes to target sprint |
| **editFetchedMsg** | tea.Msg carrying issue + valid values for the edit form |
| **editModel** | In-TUI issue edit form (not using charmbracelet/huh) |
| **epic** | A Jira issue type "Epic"; also used as a grouping concept for issues |
| **EpicColor** | Deterministic color derived from hashing the epic key |
| **filterEpic** | Active epic filter in the backlog view |
| **IssueFields** | Subset of Issue used for create/update operations |
| **IssueType** | Jira issue type (Bug, Story, Task, Epic, Sub-task, etc.) |
| **kanbanModel** | The kanban TUI model |
| **kanbanResult** | Struct returned by kanbanModel to boardModel |
| **Next-gen project** | A Jira "team-managed" project (newer project type) |
| **PickerModel** | Reusable debounced search picker widget |
| **prevView** | boardModel field that remembers which view to return to after an overlay |
| **sentinel** | The `<!-- tira: ... -->` comment that marks the template as valid |
| **SprintGroup** | A sprint plus its list of issues (includes a synthetic "Backlog" group) |
| **StatusID** | Internal Jira status ID (used for kanban column mapping) |
| **transition ID** | The ID of a Jira workflow transition (used to change status) |
| **ValidValues** | Allowed values for enumerable fields (types, priorities, assignees) |
| **visual mode** | Backlog selection mode where j/k extends a range (vim-style) |

---

## Key Types and Interfaces

### api.Client (interface)

The central interface for all Jira API access. The single concrete implementation is `jiraClient`. All TUI models receive a `api.Client` — never a concrete type — enabling mock substitution in tests.

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
    // ... and more
}
```

**See:** [API Client](api-client.md)

---

### tui.PickerModel (struct)

The shared picker widget used for assignees, parents, status transitions, and epic filters.

```go
type PickerModel struct {
    TextInput       textinput.Model
    Choices         []pickerChoice
    Cursor          int
    Completed       bool
    Aborted         bool
    SearchFunc      PickerSearchFunc
    debounceToken   int
    searchToken     int
    // ...
}
```

Create with `tui.NewPickerModel(searchFunc)`. Embed in a parent model and delegate `Update` to it. Check `Completed`/`Aborted` after each `Update`. Call `Init()` to start the initial search.

**See:** [Internal Packages](internal-packages.md)

---

### tui.RunWithSpinner (generic function)

```go
func RunWithSpinner[T any](label string, fn func() (T, error)) (T, error)
```

The only sanctioned way to show a spinner for a blocking operation before the board TUI starts. Do not create one-off spinner models.

**Usage:**
```go
issue, err := tui.RunWithSpinner("Fetching issue...", func() (*models.Issue, error) {
    return client.GetIssue(key)
})
```

---

### blModel (struct, package main)

The backlog TUI model. Lives in `internal/app/backlog.go`. Has `Init()`, `Update()`, `View()` satisfying `tea.Model`. Communicates with `boardModel` via the `result blResult` field (checked after each `Update` delegation).

**Key fields:**
```go
type blModel struct {
    groups      []models.SprintGroup
    rows        []blRow
    cursor      int
    selected    map[string]bool
    collapsed   map[int]bool
    filter      string
    filterEpic  string
    visualMode  bool
    visualAnchor int
    result      blResult
    // ...
}
```

**See:** [TUI Architecture](tui-architecture.md)

---

### kanbanModel (struct, package main)

The kanban TUI model. Lives in `internal/app/kanban.go`. Same delegation pattern as `blModel` with `result kanbanResult`.

**Key fields:**
```go
type kanbanModel struct {
    columns   [][]models.Issue
    colNames  []string
    colIdx    int
    rowIdxs   []int
    result    kanbanResult
    // ...
}
```

**See:** [TUI Architecture](tui-architecture.md)

---

### boardModel (struct, package main)

The top-level TUI model. Lives in `internal/app/board.go`. Owns both sub-models and all overlay states. Coordinates transitions between views and handles all async API results.

**See:** [TUI Architecture](tui-architecture.md)

---

### editModel (struct, package main)

The in-TUI issue edit form. Lives in `internal/app/edit_form.go`. Signals `completed`/`aborted` as boolean fields. Signals `wantAssigneePicker` when the user presses Enter on the Assignee field.

**Fields:**
```
0: Summary          (textinput, full overlay width)
1: Type             (textinput, fixed width, suggestions)
2: Priority         (textinput, fixed width, suggestions)
3: Assignee         (textinput, fixed width, opens PickerModel on enter)
4: Story Points     (textinput, fixed width)
5: Labels           (textinput, fixed width, comma-separated)
6: Description      (textarea, full width)
7: Acceptance Criteria (textarea, full width)
```

**See:** [TUI Architecture](tui-architecture.md)

---

### commentInputModel (struct, package main)

The in-TUI comment textarea wrapper. Lives in `internal/app/comment_form.go`. Signals `completed`/`aborted` as boolean fields.

**See:** [TUI Architecture](tui-architecture.md)

---

### config.Config (struct)

Loaded by `config.Load(profileName)`. Contains all credentials and defaults for one Jira profile.

```go
type Config struct {
    JiraURL        string `mapstructure:"jira_url"`
    Email          string `mapstructure:"email"`
    Token          string `mapstructure:"token"`
    Project        string `mapstructure:"project"`
    BoardID        int    `mapstructure:"board_id"`
    ClassicProject bool   `mapstructure:"classic_project"`
}
```

**See:** [Configuration](configuration.md)

---

### models.Issue (struct)

The core domain type. Populated by `GetIssue` (full detail including ADF fields) or `fetchAgileIssues` (list view fields only — no Description, AcceptanceCriteria, LinkedIssues, Comments).

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
```

**See:** [Internal Packages](internal-packages.md)

---

### models.IssueFields (struct)

Write-only subset of Issue used for `UpdateIssue` and `CreateIssue`. `Assignee` is a display name; `AssigneeID` is the account ID resolved from it.

```go
type IssueFields struct {
    Summary, IssueType, Priority   string
    Assignee   string  // display name (resolved to AssigneeID before API call)
    AssigneeID string
    StoryPoints float64
    Labels      []string
    Description, AcceptanceCriteria string
    ParentKey   string
}
```

**See:** [Internal Packages](internal-packages.md)

---

### editor.sentinel (constant)

```go
const sentinel = "<!-- tira: do not remove this line or change field names -->"
```

This string must be present in any template passed to `ParseTemplate`. Its absence is treated as template corruption.

**See:** [Internal Packages](internal-packages.md)

---

### validator.ValidationError (struct)

```go
type ValidationError struct {
    Field   string
    Value   string
    Message string
}
```

Implements `error`. Returned as a slice from `Validate()`. Used by `AnnotateTemplate` to inject error comments.

**See:** [Internal Packages](internal-packages.md)

---

## See Also

- [Architecture](architecture.md) — Overall system design
- [API Client](api-client.md) — API interface details
- [TUI Architecture](tui-architecture.md) — Model descriptions
