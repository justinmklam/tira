# TUI Architecture

The tira TUI is built with the [Bubbletea](https://github.com/charmbracelet/bubbletea) framework and provides two views (backlog and kanban) under a unified board interface.

---

## Board TUI Overview

**File:** `internal/app/board.go`

The board TUI runs a single `tea.Program` wrapping a `boardModel`. It manages:
- Two sub-models: `blModel` (backlog) and `kanbanModel` (kanban)
- Multiple overlay states (edit form, create form, assignee picker, help, comment input)
- Shared data (sprint groups, board columns, issue cache)

### boardModel Structure

```go
type boardModel struct {
    activeView     boardView        // current top-level view
    prevView       boardView        // restored after overlay closes
    backlog        blModel
    kanban         kanbanModel
    client         api.Client
    boardID        int
    jiraURL        string
    project        string
    classicProject bool
    initData       boardInitData    // shared data for refresh/rebuild
    width, height  int

    // In-TUI edit state
    editKey     string
    editIssue   *models.Issue
    editValid   *models.ValidValues
    editForm    *editModel
    editErr     string
    editSpinner spinner.Model

    // In-TUI create state
    createSprintID  int
    createResultKey string

    // Assignee picker state
    assigneePicker  tui.PickerModel
    assigneeForEdit bool

    // Help overlay
    helpModel tui.HelpModel

    // Comment state
    commentKey     string
    commentSummary string
    commentForm    *commentInputModel
    commentErr     string
}
```

### boardView Enum

```go
const (
    viewBacklog       boardView = iota
    viewKanban
    viewEditLoading    // fetching issue + valid values
    viewEdit           // edit form active
    viewEditSaving     // API call in flight
    viewCreateLoading  // fetching valid values for new issue
    viewCreate         // create form active
    viewCreateSaving   // create API call in flight
    viewAssigneePicker // assignee fuzzy picker
    viewHelp           // help overlay
    viewComment        // comment textarea active
    viewCommentSaving  // comment API call in flight
)
```

### View Switching

| Key | Action |
|-----|--------|
| `Tab` | Toggle between backlog and kanban |
| `1` | Switch to backlog |
| `2` | Switch to kanban |
| `?` | Open help overlay |

**View switching is gated** by `canSwitchView()`: returns `true` only when the active sub-model is in its base navigation state (not filtering, not in detail view, not in visual mode).

### Message Routing

`boardModel.Update` handles:
1. `tea.WindowSizeMsg` — always forwarded to both sub-models
2. `boardRefreshDoneMsg` — replaces data in both sub-models after a refresh
3. Edit/Create/Comment state machines (switch on `m.activeView`)
4. `"o"` key — opens issue in browser (when in base navigation state)
5. View-switching keys
6. Delegates remaining messages to the active sub-model

When a sub-model wants an action that crosses TUI boundaries (edit, comment, refresh, create), it sets fields in its `result` struct. `boardModel.Update` checks these after delegating and transitions accordingly.

### Refresh

`m.refreshCmd()` returns a `tea.Cmd` that re-fetches all sprint groups and board columns concurrently via `fetchAllBoardDataCore` (no progressive loading — full reload). On completion, sends `boardRefreshDoneMsg`. After a create, `m.createResultKey` is set so the backlog can navigate to the newly created issue.

### Progressive Loading

On initial startup, only the first 3 sprints are fetched (via `fetchBoardDataCore`). After the TUI renders, `lazyLoadCmd` fires in the background to fetch remaining sprints + backlog. Results arrive as `blLazyLoadDoneMsg`, which calls `blModel.appendGroups()` to merge them into the model without disrupting the user's cursor position.

### URL Construction

`boardModel.issueURL(key)` builds the Jira browser URL. The path structure depends on `classicProject`:
- **Next-gen**: `.../jira/software/projects/<PROJECT>/boards/<ID>`
- **Classic**: `.../jira/software/c/projects/<PROJECT>/boards/<ID>`

### Glamour Pre-detection

Before starting `tea.NewProgram`, `runBoardTUI` calls `glamour.NewTermRenderer(glamour.WithAutoStyle())` and discards the result. This forces `termenv`'s `sync.Once` to run while the TTY is still owned by the main goroutine, preventing blocking in background goroutines that later call `glamour.NewTermRenderer`.

---

## Backlog View (blModel)

**Files:** `internal/app/backlog.go`, `internal/app/backlog_view.go`

### State Machine

```
blList ──/──→ blFilter ──enter/esc──→ blList
       ──enter──→ blLoading ──issueFetchedMsg──→ blDetail ──esc/q──→ blList
       ──P──→ blParentPicker ──enter/esc──→ blList
       ──A──→ blAssignPicker ──enter/esc──→ blList
       ──S──→ blStoryPointInput ──enter/esc──→ blList
       ──s──→ blStatusPicker ──enter/esc──→ blList
       ──F──→ blEpicFilterPicker ──enter/esc──→ blList
```

### Row Model

The backlog flattens all sprint groups into a single scrollable list of `blRow` records:

```go
type blRow struct {
    kind     blRowKind  // blRowSprint | blRowIssue | blRowSpacer
    groupIdx int        // index into m.groups
    issueIdx int        // -1 for sprint header rows
}
```

`blBuildRows` rebuilds this flat list whenever data changes or filters change. Sprint groups are separated by spacer rows (skipped during navigation).

### Selection Modes

**Spacebar (single select):** Toggles `m.selected[issue.Key]`

**Visual mode (`v`):**
1. Press `v` to start visual selection at current cursor position
2. Sets `m.visualMode = true` and `m.visualAnchor = m.cursor`
3. Moving with `j`/`k` extends the range
4. Press `v` again or `Enter` to commit (toggles with existing selection)
5. Press `Esc` to cancel

`m.allSelected()` = union of `m.selected` and `m.visualIssueKeys()`, with XOR deselection of already-selected items

**Cut (`x`):** Marks `m.cutKeys` for move

**Paste (`p`):** Moves cut keys to current group

### Move Operations

All moves use `blMoveMultiCmd` which:
1. Calls `client.MoveIssuesToSprint(sprintID, keys)` or `client.MoveIssuesToBacklog(keys)`
2. Calls `client.RankIssues(keys, rankAfterKey, "")` to place issues at the bottom of the target group
3. Returns `blMoveMultiDoneMsg`

On `blMoveMultiDoneMsg`, the model performs a **local optimistic update** — removes moved issues from their source groups and appends to the target group — without waiting for a full refresh. The cursor navigates to the first moved issue's new position.

**Rank failures are non-fatal** (`blRankDoneMsg` is silently discarded) — the local state is already correct.

### Collapse

`m.collapsed[groupIdx]` tracks collapsed sprint groups:
- `z` — Toggle collapse current group
- `Z` — Toggle all (collapses all if any is expanded, expands all otherwise)

### Filtering

- `m.filter` — Case-insensitive substring filter on Key and Summary
- `m.filterEpic` — Filters to issues with a matching EpicKey or EpicName
- Both filters are applied together in `blMatchesFilter`

### Column Layout

Fixed column widths:
```
KEY(10)  SUMMARY(dynamic)  EPIC(16)  TYPE(8)  SP(5)  ASSIGNEE(14)
```

The summary column takes all remaining space. All columns are rendered with `tui.FixedWidth` (pads or truncates to exact rune count with `…` for overflow).

### Epic Coloring

`tui.EpicColor(epicKey)` hashes the epic key (sum of rune values) to pick from a 10-color palette, giving each epic a consistent color across sessions without configuration.

### Sprint Header Rendering

Sprint headers display:
- Collapse icon (`▼`/`▶`)
- Sprint name in bold
- Date range badge (`Mar 1 – Mar 14`) or state label
- Issue count right-aligned, connected with `─` fill characters
- Color: green for active, blue for future, dim for others

---

## Kanban View (kanbanModel)

**File:** `internal/app/kanban.go`

### State Machine

```
stateBoard ──enter──→ stateLoading ──issueFetchedMsg──→ stateDetail ──esc/q──→ stateBoard
           ──A──→ stateAssignPicker ──enter/esc──→ stateBoard
           ──s──→ stateStatusPicker ──enter/esc──→ stateBoard
```

### Column Model

`buildColumns(boardCols, issues)` maps issues into columns using a `statusID → colIndex` lookup built from `BoardColumn.StatusIDs`. Issues whose `StatusID` is not found in any column fall into the **last column** (catch-all).

**Navigation:**
- `h`/`l` — Move between columns (`m.colIdx`)
- `j`/`k` — Move between issues within a column (`m.rowIdxs[m.colIdx]`)

Each column maintains its own cursor position (`rowIdxs` slice), preserved across refreshes (clamped to new column size).

### Detail View

On `enter`, `fetchIssueCmd` is fired as a `tea.Cmd`. It:
1. Calls `client.GetIssue(key)` — 3 concurrent goroutines (issue data, comments, status change date)
2. Renders to Markdown via `display.RenderIssue`
3. Renders Markdown via glamour with `styles.DarkStyleConfig` (fixed style to avoid TTY detection in goroutine)
4. Returns `issueFetchedMsg{issue, content}`

The content is set into a `viewport.Model` for scrollable display.

### Result Signaling

When the user presses `e` or `c`, `kanbanModel` sets `m.result.editKey` or `m.result.commentKey`. `boardModel.Update` checks these after delegating and transitions to the edit/comment flow.

---

## In-TUI Edit Flow (editModel)

**File:** `internal/app/edit_form.go`

`editModel` is a custom multi-field form shown as a centered modal overlay. It is **NOT** using `charmbracelet/huh` — it's a hand-rolled form using `bubbles/textinput` and `bubbles/textarea`.

### Fields

| # | Field | Widget | Notes |
|---|-------|--------|-------|
| 0 | Summary | textinput | Full overlay width |
| 1 | Type | textinput | Fixed width, suggestions from validValues.IssueTypes |
| 2 | Priority | textinput | Fixed width, suggestions from validValues.Priorities |
| 3 | Assignee | textinput | Fixed width, opens PickerModel on Enter |
| 4 | Story Points | textinput | Fixed width |
| 5 | Labels | textinput | Fixed width, comma-separated |
| 6 | Description | textarea | Full width |
| 7 | Acceptance Criteria | textarea | Full width |

### Navigation

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Cycle forward/backward through all 8 fields |
| `Enter` | Accept suggestion (if panel open) or advance to next field. For Assignee: opens picker overlay |
| `Up` / `Down` | Cycle through inline suggestions when panel is visible |
| `Ctrl+S` | Validate and submit (sets `m.completed = true`) |
| `Esc` | If form is dirty: sets `m.confirmAbort = true` (y/n prompt); if clean: sets `m.aborted = true` |

### Suggestions Panel

When focused on Type or Priority fields, an inline suggestion list appears to the right of the input. Populated by fuzzy-matching the current input value against `typeOpts` or `priorityOpts` using `github.com/lithammer/fuzzysearch/fuzzy`.

### Assignee Picker Integration

When the user presses `Enter` on the Assignee field:
1. `m.wantAssigneePicker = true`
2. `boardModel.Update` detects this, creates a `tui.PickerModel` backed by `client.SearchAssignees`
3. Transitions to `viewAssigneePicker`
4. On completion, `m.editForm.setAssignee(label, value)` is called to inject the result

### Dirty Detection

`m.initialState` captures form values at creation time. `isDirty()` compares current values to initial to decide whether to show the abort confirmation prompt.

### Async Commands

```go
// In boardModel:
saveEditCmd(client, key, fields)   → editSaveDoneMsg
saveCreateCmd(client, project, fields, sprintID) → createSaveDoneMsg
fetchEditDataCmd(client, key)      → editFetchedMsg
fetchCreateDataCmd(client, project) → createFetchedMsg
```

`saveCreateCmd` additionally calls `client.MoveIssuesToSprint(sprintID, [key])` if `sprintID != 0`.

---

## In-TUI Comment Flow (commentInputModel)

**File:** `internal/app/comment_form.go`

A simple `textarea.Model` wrapper:

| Key | Action |
|-----|--------|
| `Ctrl+S` | Submits if textarea is non-empty (sets `m.completed = true`) |
| `Esc` | If textarea is non-empty: prompts y/n abort confirmation; if empty: sets `m.aborted = true` |

After comment saves (`commentSaveDoneMsg`), `boardModel` refreshes the detail view if currently in the detail state for backlog or kanban (re-fetches the issue to show the new comment).

---

## Design Decisions

### 1. Unified Board Model

Running both backlog and kanban under a single `boardModel` allows:
- Shared data (no duplicate API calls when switching views)
- Seamless view toggling with `Tab`
- Consistent overlay states (edit, comment, picker) across views

### 2. Result Struct Pattern

Sub-models signal cross-boundary actions via `result` structs:
```go
type blResult struct {
    editKey      string
    commentKey   string
    wantRefresh  bool
    // ...
}
```

After delegating `Update`, `boardModel` checks `blResult` and transitions accordingly. This keeps sub-models decoupled from the parent.

### 3. Local Optimistic UI

Move operations update local state immediately without waiting for API confirmation. This provides instant feedback but may diverge from server state. A full refresh (`R`) reconciles.

### 4. View Switching Gates

View switching is blocked when:
- Filter is active
- Detail view is open
- Visual mode is active

This prevents accidental state loss.

---

## See Also

- [Keybindings](keybindings-backlog.md) — Complete keybinding reference
- [State Machines](state-machines.md) — Visual state diagrams
- [Internal Packages](internal-packages.md) — TUI helper packages
