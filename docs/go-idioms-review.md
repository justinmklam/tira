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

## Priority 2.5 — File Structure in `cmd/tira/`

### Current layout

| File | Contents | Lines |
|------|----------|-------|
| `backlog.go` | `blModel` type + `Init` + `Update` + all update sub-handlers + helper funcs + Cmd funcs | 1412 |
| `backlog_view.go` | `blModel.View()` + all render methods | 593 |
| `kanban.go` | `kanbanModel` type + Init + Update + View — everything in one file | 727 |
| `board.go` | Cobra commands + `boardModel` type + Init + Update + View | 898 |
| `editmodel.go` | `editModel` struct + Init + Update + View (the TUI form widget) | 454 |
| `edit.go` | `editFormState`, msg types, tea.Cmd functions, `newAssigneePicker` | 180 |
| `commentmodel.go` | `commentInputModel` struct + Init + Update + View | 103 |
| `get.go` | `getCmd` Cobra command + `runEditLoop` + `openAndValidate` + `page` | 245 |
| `create.go` | `createCmd` Cobra command | 98 |
| `root.go` | Root Cobra command + config loading | 65 |
| `main.go` | Entry point | 5 |

### Problems

**1. Inconsistent splitting strategy.** Backlog splits model+update from view, but kanban and board don't. If the pattern is worth doing for backlog, it should be applied consistently — or not at all.

**2. `edit.go` naming is misleading.** It sounds like a Cobra command file (matching the `get.go` / `create.go` pattern), but it actually contains shared TUI infrastructure: msg types, tea.Cmd functions, `editFormState`, `newAssigneePicker`, and `blankIssueFromValid`. Meanwhile the actual `--edit` CLI flow lives in `get.go`. A reader looking for the edit command finds the wrong file.

**3. `board.go` mixes three concerns.** It contains Cobra command definitions (`boardCmd`, `backlogCmd`, `kanbanCmd`), the top-level `boardModel` with its ~400-line `Update` method, AND multiple View helper methods. In idiomatic Go, Cobra command wiring and TUI model logic belong in separate files.

**4. `backlog.go` is still 1400 lines after the view split.** The Update method alone is ~130 lines, plus 6 sub-update handlers, plus move/rank logic, plus picker builders, plus clipboard/URL helpers. This is too many concerns for one file.

**5. Not a standard Go pattern.** Go projects typically split files by *concern* or *type*, not by MVC layers. The `_view.go` suffix is borrowed from Elm architecture but isn't a recognized Go convention. Bubbletea projects in the ecosystem (e.g. `charm` tools, `glow`, `soft-serve`) typically use one file per model or one file per feature — not model/view splits.

### Recommended structure

Split by **feature/concern**, with each sub-model self-contained in its own file. Cobra command wiring goes in a dedicated file. Large models can split update from view, but do so consistently.

```
cmd/tira/
├── main.go                  # Entry point (unchanged)
├── root.go                  # Root command + config (unchanged)
├── commands.go              # All Cobra command definitions + init() wiring
│
├── board_model.go           # boardModel type + Init + Update + View
├── board_views.go           # boardModel overlay/form rendering helpers
│
├── backlog_model.go         # blModel type + Init + Update dispatch
├── backlog_update.go        # blModel sub-update handlers (list, filter, detail, pickers)
├── backlog_view.go          # blModel View + render helpers (keep as-is)
│
├── kanban_model.go          # kanbanModel type + Init + Update dispatch
├── kanban_view.go           # kanbanModel View + render helpers
│
├── edit_form.go             # editModel (the TUI form widget — was editmodel.go)
├── edit_cmds.go             # editFormState, msg types, tea.Cmd funcs, pickers (was edit.go)
│
├── comment_form.go          # commentInputModel (was commentmodel.go — rename for consistency)
│
├── get_cmd.go               # get command RunE + runEditLoop + page (was get.go)
├── create_cmd.go            # create command RunE (was create.go)
```

**Key changes:**

1. **Cobra commands extracted** to `commands.go` — all `var xxxCmd` and `init()` wiring in one place, so TUI model files are pure model logic.
2. **Consistent naming** — `_model.go` / `_update.go` / `_view.go` suffix convention applied uniformly. The `_model.go` file contains the type, Init, and Update dispatch. The `_update.go` file (only needed for backlog's size) contains sub-handlers. The `_view.go` file contains View and render helpers.
3. **`edit.go` → `edit_cmds.go`** — makes it clear this file contains tea.Cmd functions and msg types, not a Cobra command.
4. **`editmodel.go` → `edit_form.go`** — aligns with `comment_form.go` and avoids the ambiguous "model" suffix (everything in Bubbletea is a model).
5. **`kanban.go` split** into `kanban_model.go` + `kanban_view.go` — consistent with backlog.
6. **CLI command files** get a `_cmd.go` suffix to distinguish them from TUI model files.

This is a pure rename/reorganization refactor — no logic changes needed, just file boundaries. It can be done incrementally (e.g. extract `commands.go` first, then rename files one by one).

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
| P2.5 — File Structure | 5 | Inconsistent splits, misleading names, mixed concerns |
| P3 — Code Quality | 7 | Duplication, unnecessary allocations, idioms |
| Tests | 8+ | API parsing, concurrency, form logic, kanban mapping |
