# State Machines

This document describes the state machines used throughout the tira TUI.

---

## boardModel View States

The top-level board TUI state machine manages view switching and overlay states.

```mermaid
stateDiagram-v2
    [*] --> viewBacklog : startView=backlog
    [*] --> viewKanban : startView=kanban
    
    viewBacklog --> viewKanban : Tab / 2
    viewKanban --> viewBacklog : Tab / 1
    
    viewBacklog --> viewEditLoading : e key (from backlog)
    viewKanban --> viewEditLoading : e key (from kanban)
    
    viewEditLoading --> viewEdit : editFetchedMsg (ok)
    viewEditLoading --> viewBacklog : editFetchedMsg (err)
    
    viewEdit --> viewEditSaving : ctrl+s (form complete)
    viewEdit --> viewBacklog : esc / abort
    viewEdit --> viewAssigneePicker : enter on Assignee field
    
    viewEditSaving --> viewBacklog : editSaveDoneMsg
    
    viewAssigneePicker --> viewEdit : selection made / esc
    
    viewBacklog --> viewCreateLoading : a/C key
    viewCreateLoading --> viewCreate : createFetchedMsg (ok)
    
    viewCreate --> viewCreateSaving : ctrl+s
    viewCreate --> viewBacklog : abort
    
    viewCreateSaving --> viewBacklog : createSaveDoneMsg
    
    viewBacklog --> viewComment : c key
    viewKanban --> viewComment : c key
    
    viewComment --> viewCommentSaving : ctrl+s
    viewCommentSaving --> viewBacklog : commentSaveDoneMsg
    
    viewBacklog --> viewHelp : ?
    viewKanban --> viewHelp : ?
    
    viewHelp --> viewBacklog : esc / ?
    viewHelp --> viewKanban : esc / ?
```

### State Descriptions

| State | Description |
|-------|-------------|
| `viewBacklog` | Backlog view active (base state) |
| `viewKanban` | Kanban view active (base state) |
| `viewEditLoading` | Fetching issue + valid values for edit |
| `viewEdit` | Edit form active (huh-like multi-field input) |
| `viewEditSaving` | API call to update issue in flight |
| `viewCreateLoading` | Fetching valid values for new issue |
| `viewCreate` | Create form active |
| `viewCreateSaving` | API call to create issue in flight |
| `viewAssigneePicker` | Assignee fuzzy picker overlay |
| `viewHelp` | Help overlay active |
| `viewComment` | Comment textarea active |
| `viewCommentSaving` | API call to add comment in flight |

### View Switching Rules

View switching (Tab, 1, 2) is **gated** by `canSwitchView()`:

- Returns `true` only when the active sub-model is in its base navigation state
- Blocked when: filter active, detail view open, visual mode active

---

## blModel (Backlog) States

The backlog view state machine handles navigation, filtering, and detail operations.

```mermaid
stateDiagram-v2
    [*] --> blList
    
    blList --> blFilter : /
    blFilter --> blList : Enter (apply)
    blFilter --> blList : Esc (clear)
    
    blList --> blLoading : Enter on issue
    blLoading --> blDetail : issueFetchedMsg
    blDetail --> blList : Esc / q
    
    blList --> blParentPicker : P
    blParentPicker --> blList : Enter / Esc
    
    blList --> blAssignPicker : A
    blAssignPicker --> blList : Enter / Esc
    
    blList --> blStoryPointInput : S
    blStoryPointInput --> blList : Enter / Esc
    
    blList --> blStatusPicker : s
    blStatusPicker --> blList : Enter / Esc
    
    blList --> blEpicFilterPicker : F
    blEpicFilterPicker --> blList : Enter / Esc
```

### State Descriptions

| State | Description |
|-------|-------------|
| `blList` | Normal list navigation (base state) |
| `blFilter` | Filter mode active (`/` key) |
| `blLoading` | Fetching issue for detail view |
| `blDetail` | Issue detail pane open |
| `blParentPicker` | Parent/epic picker overlay |
| `blAssignPicker` | Assignee picker overlay |
| `blStoryPointInput` | Story points inline input |
| `blStatusPicker` | Status transition picker |
| `blEpicFilterPicker` | Epic filter picker |

---

## kanbanModel (Kanban) States

The kanban view state machine handles column navigation and detail operations.

```mermaid
stateDiagram-v2
    [*] --> stateBoard
    
    stateBoard --> stateLoading : Enter on issue
    stateLoading --> stateDetail : issueFetchedMsg
    stateDetail --> stateBoard : Esc / q
    
    stateBoard --> stateAssignPicker : A
    stateAssignPicker --> stateBoard : Enter / Esc
    
    stateBoard --> stateStatusPicker : s
    stateStatusPicker --> stateBoard : Enter / Esc
```

### State Descriptions

| State | Description |
|-------|-------------|
| `stateBoard` | Column navigation (base state) |
| `stateLoading` | Fetching issue for detail view |
| `stateDetail` | Issue detail pane open (viewport) |
| `stateAssignPicker` | Assignee picker overlay |
| `stateStatusPicker` | Status transition picker |

---

## editModel (Edit Form) States

The in-TUI edit form state machine handles field navigation and submission.

```mermaid
stateDiagram-v2
    [*] --> editing
    
    editing --> editing : Tab / Shift+Tab (next/prev field)
    editing --> editing : Enter (advance or accept suggestion)
    editing --> suggestions : Focus on Type/Priority
    
    suggestions --> editing : Enter (accept) or Esc (dismiss)
    
    editing --> confirmAbort : Esc (if dirty)
    editing --> [*] : Esc (if clean)
    
    confirmAbort --> editing : n (continue editing)
    confirmAbort --> [*] : y (abort)
    
    editing --> [*] : Ctrl+S (submit)
```

### State Descriptions

| State | Description |
|-------|-------------|
| `editing` | Normal form editing (base state) |
| `suggestions` | Inline suggestions panel visible |
| `confirmAbort` | Dirty-check confirmation prompt |

---

## commentInputModel States

The comment input state machine is simple:

```mermaid
stateDiagram-v2
    [*] --> typing
    
    typing --> [*] : Ctrl+S (submit if non-empty)
    typing --> confirmAbort : Esc (if non-empty)
    typing --> [*] : Esc (if empty)
    
    confirmAbort --> typing : n (continue)
    confirmAbort --> [*] : y (abort)
```

---

## State Transitions via Result Structs

Sub-models signal cross-boundary actions via `result` structs. After delegating `Update`, `boardModel` checks these and transitions accordingly.

### blResult

```go
type blResult struct {
    editKey      string  // Open edit form
    commentKey   string  // Open comment input
    wantRefresh  bool    // Refresh board data
    moveMulti    struct{...}  // Move issues operation
    // ...
}
```

### kanbanResult

```go
type kanbanResult struct {
    editKey      string  // Open edit form
    commentKey   string  // Open comment input
    // ...
}
```

---

## Concurrency States

### Async Operations

All API calls inside the TUI are wrapped as `tea.Cmd` functions that run in goroutines:

```go
func fetchIssueCmd(client api.Client, key string, vpW int) tea.Cmd {
    return func() tea.Msg {
        // runs in goroutine
        issue, err := client.GetIssue(key)
        return issueFetchedMsg{issue: issue, err: err}
    }
}
```

### Message Types

| Message | Trigger |
|---------|---------|
| `issueFetchedMsg` | Issue fetch complete |
| `editFetchedMsg` | Edit data fetch complete |
| `createFetchedMsg` | Create data fetch complete |
| `editSaveDoneMsg` | Edit save complete |
| `createSaveDoneMsg` | Create save complete |
| `commentSaveDoneMsg` | Comment save complete |
| `boardRefreshDoneMsg` | Board refresh complete |
| `blMoveMultiDoneMsg` | Multi-move operation complete |
| `blRankDoneMsg` | Rank operation complete |

---

## See Also

- [TUI Architecture](tui-architecture.md) — Detailed model descriptions
- [Keybindings](keybindings-backlog.md) — Keys that trigger state transitions
