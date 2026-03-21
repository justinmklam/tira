# Go Idioms & Best Practices Review

Review of non-idiomatic patterns, code quality issues, and missing test coverage.

---

## Priority 1 — Correctness Bugs

### 1.1 Data race in `GetIssue` (internal/api/client.go:76–111)

Three goroutines share variables without synchronization:
- Goroutine 1 writes `result` via `fetchFullIssue`
- Goroutine 3 reads/writes `result.StatusChangedDate` — but `result` may still be nil
- `fetchErr` is written by goroutine 1 and read after `wg.Wait()`, which is safe, but `comments` and the `result.StatusChangedDate` write have no ordering guarantee relative to `result` being non-nil

**Fix:** Use an `errgroup.Group` or protect shared state with a mutex. Alternatively, have each goroutine return its own result and merge after `wg.Wait()`.

### 1.2 Variable shadowing of receiver (internal/api/client.go:94)

```go
go func() {
    defer wg.Done()
    if c, err := c.fetchComments(key); err == nil {  // 'c' shadows receiver
        comments = c
    }
}()
```

The loop variable `c` (comments) shadows the `*jiraClient` receiver `c`. While it works because the receiver is captured by closure before the shadow, it's confusing and a golangci-lint `govet` shadow warning.

**Fix:** Rename the return value: `if cmts, err := c.fetchComments(key); ...`

### 1.3 `fetchStatusChangeDate` returns first status change, not last (internal/api/client.go:386–397)

The comment says "most recent status change" but the loop iterates forward through the chronological changelog and returns on the first match. For issues that have changed status multiple times, this returns the *earliest* transition, not the latest.

**Fix:** Iterate in reverse (`for i := len(result.Values) - 1; i >= 0; i--`).

### 1.4 `runtime.SetFinalizer` never runs (cmd/tira/root.go:53–59)

```go
func Execute() {
    if debugMode {  // debugMode is always false here — Cobra hasn't parsed flags yet
        runtime.SetFinalizer(...)
    }
    ...
}
```

Even if `debugMode` were true, `runtime.SetFinalizer` on an unreferenced `new(struct{})` is unreliable — the GC may collect it immediately or never run the finalizer before exit.

**Fix:** Add a `PersistentPostRunE` to `rootCmd` that calls `debug.Close()`, or use `defer debug.Close()` in `PersistentPreRunE` after `debug.Init()` succeeds.

---

## Priority 2 — API Client Hygiene

### 2.1 Missing `context.Context` on `Client` interface (internal/api/client.go:19–44)

None of the 20+ `Client` interface methods accept a `context.Context`. This is non-idiomatic for methods that make network calls and prevents:
- Request cancellation when the user quits the TUI
- Timeout enforcement
- Tracing/telemetry propagation

**Fix:** Add `ctx context.Context` as the first parameter to every `Client` method. This is a large refactor — consider doing it method-by-method.

### 2.2 Inconsistent HTTP request construction (internal/api/client.go)

CLAUDE.md documents the convention: use `c.client.NewRequest` + `c.client.Do` for Jira endpoints. The following methods use raw `http.NewRequestWithContext` + `c.http.Do` instead:

| Method | Line |
|--------|------|
| `UpdateIssue` | 526 |
| `CreateIssue` | 645 |
| `SetParent` | 1232 |
| `SetAssignee` | 1296 |
| `TransitionStatus` | 1402 |

Methods that correctly follow the convention: `MoveIssuesToSprint`, `RankIssues`, `MoveIssuesToBacklog`, `AddComment`.

**Fix:** Migrate the raw-HTTP methods to use `c.client.NewRequest`/`c.client.Do`. This also removes the need for `c.http` and `c.baseURL` fields on the struct for those calls.

### 2.3 `strings.NewReader(string(body))` allocations (internal/api/client.go:526,645,1232,1296,1402)

```go
req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, strings.NewReader(string(body)))
```

`body` is already `[]byte` — converting to `string` then wrapping in `strings.NewReader` copies the data unnecessarily.

**Fix:** Use `bytes.NewReader(body)` directly.

### 2.4 Swallowed errors in `GetValidValues` (internal/api/client.go:728–742)

Errors from the priorities and assignees HTTP calls are silently ignored:

```go
if prioResp, err := c.http.Get(prioURL); err == nil {
    // ...
}
// error path: silently returns partial data
```

**Fix:** At minimum, log errors via `debug.LogError`. Consider returning a partial result with a warning, or using `errors.Join` to aggregate.

### 2.5 `GetValidValues` and `GetIssueMetadata` are nearly identical (internal/api/client.go:699–829)

These two methods share ~95% of their code. `GetIssueMetadata`'s doc says it "returns issue types and priorities only (no assignee lookup)" but it actually fetches assignees too.

**Fix:** Have `GetIssueMetadata` call `GetValidValues` (or extract a shared helper), or actually skip the assignee fetch as documented.

---

## Priority 2.5 — Project Structure & Separation of Concerns

### Current layout

| File | Contents | Lines |
|------|----------|-------|
| `cmd/tira/backlog.go` | `blModel` type + `Init` + `Update` + all update sub-handlers + helper funcs + Cmd funcs | 1412 |
| `cmd/tira/backlog_view.go` | `blModel.View()` + all render methods | 593 |
| `cmd/tira/kanban.go` | `kanbanModel` type + Init + Update + View — everything in one file | 727 |
| `cmd/tira/board.go` | Cobra commands + `boardModel` type + Init + Update + View | 898 |
| `cmd/tira/editmodel.go` | `editModel` struct + Init + Update + View (the TUI form widget) | 454 |
| `cmd/tira/edit.go` | `editFormState`, msg types, tea.Cmd functions, `newAssigneePicker` | 180 |
| `cmd/tira/commentmodel.go` | `commentInputModel` struct + Init + Update + View | 103 |
| `cmd/tira/get.go` | `getCmd` Cobra command + `runEditLoop` + `openAndValidate` + `page` | 245 |
| `cmd/tira/create.go` | `createCmd` Cobra command | 98 |
| `cmd/tira/root.go` | Root Cobra command + config loading | 65 |
| `cmd/tira/main.go` | Entry point | 5 |

### Problems

**1. TUI models live in `package main`.** All 4,300+ lines of TUI model logic (board, backlog, kanban, edit form, comment form) are in `cmd/tira/`, which is `package main`. This means none of this code can be imported or unit tested independently. In idiomatic Go, `cmd/` is a thin CLI entry point — business logic and application models belong in `internal/`.

**2. Inconsistent splitting strategy.** Backlog splits model+update from view, but kanban and board don't. If the pattern is worth doing for backlog, it should be applied consistently — or not at all.

**3. `edit.go` naming is misleading.** It sounds like a Cobra command file (matching the `get.go` / `create.go` pattern), but it actually contains shared TUI infrastructure: msg types, tea.Cmd functions, `editFormState`, `newAssigneePicker`, and `blankIssueFromValid`. Meanwhile the actual `--edit` CLI flow lives in `get.go`. A reader looking for the edit command finds the wrong file.

**4. `board.go` mixes three concerns.** It contains Cobra command definitions (`boardCmd`, `backlogCmd`, `kanbanCmd`), the top-level `boardModel` with its ~400-line `Update` method, AND multiple View helper methods. In idiomatic Go, Cobra command wiring and TUI model logic belong in separate files.

**5. `backlog.go` is still 1400 lines after the view split.** The Update method alone is ~130 lines, plus 6 sub-update handlers, plus move/rank logic, plus picker builders, plus clipboard/URL helpers. Too many concerns for one file.

**6. Not a standard Go pattern.** Go projects typically split files by *concern* or *type*, not by MVC layers. The `_view.go` suffix is borrowed from Elm architecture but isn't a recognized Go convention. Bubbletea projects in the ecosystem (e.g. `charm` tools, `glow`, `soft-serve`) typically use one file per model or one file per feature — not model/view splits.

### Recommended structure

Move all TUI models out of `cmd/tira/` into a new `internal/app/` package. The models are tightly coupled — `boardModel` embeds and dispatches to `blModel` and `kanbanModel`, they share msg types — so they belong in one package, not split across separate packages.

`cmd/tira/` becomes a thin CLI layer: just Cobra command wiring, `main()`, and config loading.

```
cmd/tira/
├── main.go              # Entry point (unchanged)
├── root.go              # Root Cobra command + config loading
├── board.go             # board/backlog/kanban Cobra command defs + RunE (thin — calls into internal/app)
├── get.go               # get Cobra command + runEditLoop + page
├── create.go            # create Cobra command

internal/app/
├── board.go             # BoardModel type + Init + Update + View
├── board_overlays.go    # BoardModel overlay/form rendering helpers (edit, comment, assignee, help)
│
├── backlog.go           # BlModel type + Init + Update dispatch
├── backlog_update.go    # BlModel sub-update handlers (list, filter, detail, pickers, move/rank)
├── backlog_view.go      # BlModel View + render helpers
│
├── kanban.go            # KanbanModel type + Init + Update
├── kanban_view.go       # KanbanModel View + render helpers
│
├── edit_form.go         # EditModel (the TUI form widget — was editmodel.go)
├── edit_cmds.go         # EditFormState, msg types, tea.Cmd funcs, pickers (was edit.go)
├── comment_form.go      # CommentInputModel (was commentmodel.go)
│
├── messages.go          # Shared msg types used across models (optional — only if extraction cleans up imports)

internal/tui/            # (unchanged — stays as a zero-dep leaf)
├── spinner.go
├── styles.go
├── helpers.go
├── picker.go
├── help.go
```

**Updated dependency graph:**

```
cmd/tira (thin CLI layer)
 ├── internal/app         ← TUI models (board, backlog, kanban, edit form, comment)
 ├── internal/config
 └── internal/tui         ← RunWithSpinner used directly by CLI commands

internal/app
 ├── internal/api
 ├── internal/models
 ├── internal/tui          ← styles, helpers, picker (still zero internal deps)
 ├── internal/display
 └── internal/debug

internal/tui               ← NO dependencies on other internal packages (unchanged)
```

### Implementation phases

#### Phase 1: Extract `internal/app/` package

The foundational change. Move all TUI model code from `cmd/tira/` to `internal/app/`, exporting the types and constructors that `cmd/tira/` needs to call.

1. Create `internal/app/` directory
2. Move model files: `backlog.go`, `backlog_view.go`, `kanban.go`, `board.go` (model parts only), `editmodel.go`, `edit.go`, `commentmodel.go`
3. Export types that `cmd/tira/` needs: `BoardModel`, `BlModel`, `NewBoardModel`, `FetchBoardData`, `RunBoardTUI`, etc.
4. Keep unexported types/helpers that are internal to the package (msg types, sub-update handlers, render helpers)
5. Strip Cobra command definitions out of `board.go` — leave them in `cmd/tira/board.go`
6. Verify: `make check` passes, no logic changes

#### Phase 2: Rename and split files consistently

Now that everything is in `internal/app/`, apply consistent file organization.

1. Rename `editmodel.go` → `edit_form.go`
2. Rename `edit.go` → `edit_cmds.go`
3. Rename `commentmodel.go` → `comment_form.go`
4. Split `kanban.go` → `kanban.go` (model + update) + `kanban_view.go` (View + render helpers)
5. Split `board.go` → `board.go` (model + update) + `board_overlays.go` (overlay rendering)
6. Split `backlog.go` → `backlog.go` (type + Init + Update dispatch) + `backlog_update.go` (sub-handlers)
7. Verify: `make check` passes, no logic changes

#### Phase 3: Update documentation and CLAUDE.md

Update all references to reflect the new structure.

1. Update `CLAUDE.md`:
   - Change "TUI models live in `cmd/tira/` (package main)" → "TUI models live in `internal/app/`, `cmd/tira/` is a thin CLI layer"
   - Update the backlog rendering note: "`backlog.go` (model + update dispatch), `backlog_update.go` (sub-handlers), and `backlog_view.go` (rendering)"
   - Add: "`internal/app` contains all Bubbletea models — keep Cobra command wiring in `cmd/tira/`"
2. Update `docs/architecture.md`:
   - Add `internal/app/` to the project layout tree
   - Update the dependency graph
   - Update "All TUI model code lives in `cmd/tira/` (package main)" under key invariants
3. Remove this section from the idioms review (it's done)

---

## Priority 3 — Code Quality & Idioms

### 3.1 `fmt.Errorf("%w", err)` adds no context (cmd/tira/root.go:39)

```go
return fmt.Errorf("%w", err)
```

Re-wrapping without added context is pointless. Either add context or return `err` directly.

### 3.2 `RenderIssue` returns `(string, error)` but error is always nil (internal/display/issue.go:12)

```go
func RenderIssue(issue *models.Issue) (string, error) {
    // ... never returns a non-nil error
    return sb.String(), nil
}
```

Forces every caller to handle a phantom error. All callers already check `err` unnecessarily.

**Fix:** Change signature to `func RenderIssue(issue *models.Issue) string`.

### 3.3 Duplicated `containsCI` function

Identical implementations exist in:
- `internal/tui/helpers.go:107–114` (`ContainsCI`, exported)
- `internal/validator/validate.go:85–92` (`containsCI`, unexported)

The architecture forbids `validator` from importing `tui`, so the duplication is intentional — but should be documented with a comment, or the function should be moved to a tiny shared package (e.g. `internal/stringutil`).

### 3.4 O(n²) key lookup in `bulkOperation` (internal/api/client.go:1488–1493)

```go
for i, k := range keys {
    if k == key {
        results <- struct{idx int; err error}{idx: i, err: err}
        break
    }
}
```

Each worker scans the full `keys` slice to find its index.

**Fix:** Send `(index, key)` pairs through the jobs channel, or build a `keyToIdx` map before the loop.

### 3.5 Duplicated picker overlay rendering

These three functions are nearly identical (same width calc, border style, layout):
- `boardModel.viewAssigneePickerOverlay` (cmd/tira/board.go:805–837)
- `kanbanModel.viewAssignPicker` (cmd/tira/kanban.go:405–446)
- `kanbanModel.viewStatusPicker` (cmd/tira/kanban.go:448–489)

**Fix:** Extract a shared `renderPickerOverlay(title string, picker PickerModel, w, h int) string` helper in `internal/tui`.

### 3.6 Duplicated parallel board data fetch

`fetchBoardData` (cmd/tira/board.go:146–174) and `refreshCmd` (cmd/tira/board.go:643–673) contain the same WaitGroup + two-goroutine fetch logic.

**Fix:** Have `refreshCmd` reuse the fetch logic from `fetchBoardData` (just call a shared function and wrap the result in a `boardRefreshDoneMsg`).

### 3.7 Unnecessary `resp, err := ...; return resp, err` (internal/debug/logger.go:106–108)

```go
resp, err := t.Base.RoundTrip(req)
return resp, err
```

**Fix:** `return t.Base.RoundTrip(req)`

---

## Missing Tests & Coverage Gaps

### High-Value Tests (reduce need for manual TUI testing)

#### T1. API response parsing (`internal/api/`)
No tests exist for JSON response parsing. Adding table-driven tests for `fetchFullIssue`, `fetchAgileIssues`, `fetchComments`, and `fetchStatusChangeDate` with fixture JSON would catch regressions in field mapping without needing a live Jira instance.

```go
func TestFetchFullIssue_ParsesAllFields(t *testing.T) { ... }
func TestFetchFullIssue_NilOptionalFields(t *testing.T) { ... }
func TestFetchAgileIssues_StoryPointsFallback(t *testing.T) { ... }
func TestFetchComments_ADFBody(t *testing.T) { ... }
func TestFetchStatusChangeDate_ReturnsLatest(t *testing.T) { ... }
```

**Approach:** Use `httptest.NewServer` to serve canned JSON responses; construct a `jiraClient` pointing at the test server.

#### T2. `bulkOperation` concurrency (`internal/api/`)
Test that `bulkOperation` correctly maps results to the right indices, handles mixed success/failure, and respects the worker pool limit.

```go
func TestBulkOperation_MapsResultsCorrectly(t *testing.T) { ... }
func TestBulkOperation_PartialFailure(t *testing.T) { ... }
func TestBulkOperation_EmptyKeys(t *testing.T) { ... }
```

#### T3. `editFormState.toIssueFields` roundtrip (`cmd/tira/edit.go`)
This is the bridge between the TUI form and the API — test that form values serialize correctly:

```go
func TestToIssueFields_AllFields(t *testing.T) { ... }
func TestToIssueFields_AssigneeUnchanged_ReusesID(t *testing.T) { ... }
func TestToIssueFields_AssigneeChanged_ResolvesID(t *testing.T) { ... }
func TestToIssueFields_EmptyStoryPoints(t *testing.T) { ... }
func TestToIssueFields_LabelsCommaSeparated(t *testing.T) { ... }
```

#### T4. `buildColumns` kanban mapping (`cmd/tira/kanban.go`)
Test that issues land in the correct column based on status ID, and that unmapped statuses fall to the last column:

```go
func TestBuildColumns_MapsStatusIDs(t *testing.T) { ... }
func TestBuildColumns_UnmappedStatusFallsToLast(t *testing.T) { ... }
func TestBuildColumns_EmptyInput(t *testing.T) { ... }
```

#### T5. `blBuildRows` and `blMatchesFilter` (`cmd/tira/backlog.go`)
Test row construction from sprint groups, collapsed state, text filtering, and epic filtering:

```go
func TestBlBuildRows_BasicStructure(t *testing.T) { ... }
func TestBlBuildRows_Collapsed(t *testing.T) { ... }
func TestBlMatchesFilter_TextMatch(t *testing.T) { ... }
func TestBlMatchesFilter_EpicFilter(t *testing.T) { ... }
```

#### T6. `DaysInColumn` and `DaysColor` (`internal/tui/styles.go`)
These drive the kanban age indicators. Currently untested:

```go
func TestDaysInColumn_ValidDate(t *testing.T) { ... }
func TestDaysInColumn_EmptyDate(t *testing.T) { ... }
func TestDaysInColumn_InvalidDate(t *testing.T) { ... }
func TestDaysColor_Thresholds(t *testing.T) { ... }
```

#### T7. `markdownToADF` roundtrip (`internal/api/client.go`)
Test that `markdownToADF` produces valid ADF that Jira accepts, and that `ADFToMarkdown(markdownToADF(text))` preserves content:

```go
func TestMarkdownToADF_Structure(t *testing.T) { ... }
func TestMarkdownToADF_Roundtrip(t *testing.T) { ... }
```

#### T8. `OverlaySize` and `OverlayViewportSize` boundary tests (`internal/tui/helpers.go`)

```go
func TestOverlaySize_Clamping(t *testing.T) { ... }
func TestOverlayViewportSize_MinValues(t *testing.T) { ... }
```

### Lower-Priority Tests

- **Config validation edge cases** — missing individual required fields, partial profiles
- **`OpenEditor`** — difficult to unit test (spawns a process), but could test the editor selection logic (`$EDITOR` → `$VISUAL` → `vi`) in isolation
- **`AnnotateTemplate` stacking** — verify that running annotation twice doesn't stack error comments
- **Picker model** — test cursor movement, debounce token superseding, NoneItem visibility

---

## Summary

| Priority | Count | Theme |
|----------|-------|-------|
| P1 — Correctness | 4 | Data races, wrong results, dead code |
| P2 — API Hygiene | 5 | Context propagation, consistency, error handling |
| P2.5 — Project Structure | 6 | Models in package main, inconsistent splits, misleading names, mixed concerns |
| P3 — Code Quality | 7 | Duplication, unnecessary allocations, idioms |
| Tests | 8+ | API parsing, concurrency, form logic, kanban mapping |
