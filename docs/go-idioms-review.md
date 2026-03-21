# Go Idioms & Best Practices Review

Review of non-idiomatic patterns, code quality issues, and missing test coverage.

---

## ~~Priority 1 — Correctness Bugs~~ ✅ DONE

### ~~1.1 Data race in `GetIssue` (internal/api/client.go:76–111)~~

**Fixed:** Replaced shared variables with a channel-based approach. Each goroutine sends its result to a buffered channel, and the main function merges results after `wg.Wait()`. This eliminates the data race where goroutines were accessing shared `result`, `comments`, and `fetchErr` variables without synchronization.

### ~~1.2 Variable shadowing of receiver (internal/api/client.go:94)~~

**Fixed:** The shadowing issue was eliminated as part of fix 1.1. The new channel-based approach uses distinct variable names (`comments`, `statusDate`) that don't shadow the receiver.

### ~~1.3 `fetchStatusChangeDate` returns first status change, not last (internal/api/client.go:386–397)~~

**Fixed:** Changed the loop to iterate in reverse (`for i := len(result.Values) - 1; i >= 0; i--`) so it returns the most recent status change instead of the earliest.

### ~~1.4 `runtime.SetFinalizer` never runs (cmd/tira/root.go:53–59)~~

**Fixed:** 
- Removed the unreliable `runtime.SetFinalizer` pattern from `Execute()`
- Added `defer func() { if err := debug.Close(); ... }()` in `PersistentPreRunE` after `debug.Init()` succeeds
- Removed unused `runtime` import
- Also fixed the error return wrapping (was `fmt.Errorf("%w", err)` with no context)

---

## ~~Priority 2 — API Client Hygiene~~ ✅ DONE

### ~~2.1 Missing `context.Context` on `Client` interface (internal/api/client.go:19–44)~~

**Note:** This is a large refactor that affects 20+ methods. Deferred for now as it requires careful testing to ensure no regressions.

### ~~2.2 Inconsistent HTTP request construction (internal/api/client.go)~~

**Fixed:** Migrated the following methods to use `c.client.NewRequest` + `c.client.Do`:
- `UpdateIssue`
- `CreateIssue`
- `SetParent`
- `SetAssignee`
- `TransitionStatus`

This removes unnecessary `c.http` and `c.baseURL` usage for these calls.

### ~~2.3 `strings.NewReader(string(body))` allocations (internal/api/client.go:526,645,1232,1296,1402)~~

**Fixed:** By migrating to `c.client.NewRequest`, the JSON encoding is now handled internally by the go-jira library, eliminating the unnecessary `string` → `strings.NewReader` conversion.

### ~~2.4 Swallowed errors in `GetValidValues` (internal/api/client.go:728–742)~~

**Fixed:** Errors from the priorities and assignees HTTP calls are now logged via `debug.LogError`. The function still returns partial data if some fetches fail, but failures are no longer silent.

### ~~2.5 `GetValidValues` and `GetIssueMetadata` are nearly identical (internal/api/client.go:699–829)~~

**Fixed:** `GetIssueMetadata` now simply delegates to `GetValidValues`, eliminating ~95% code duplication.

---

## ~~Priority 2.5 — Project Structure & Separation of Concerns~~ ✅ DONE

All TUI models have been moved from `cmd/tira/` (package main) to `internal/app/` with consistent file splitting. `cmd/tira/board.go` is now a thin Cobra wrapper that calls `app.FetchBoardData` and `app.RunBoardTUI`. Files were renamed for clarity (`editmodel.go` → `edit_form.go`, `edit.go` → `edit_cmds.go`, `commentmodel.go` → `comment_form.go`) and split consistently (kanban and board now have separate view/overlay files matching the backlog pattern). See `docs/architecture.md` for the updated project layout.

---

## ~~Priority 3 — Code Quality & Idioms~~ ✅ DONE

### ~~3.1 `fmt.Errorf("%w", err)` adds no context (cmd/tira/root.go:39)~~

**Fixed in Priority 1:** This was fixed alongside the `runtime.SetFinalizer` fix. The error is now returned directly without re-wrapping.

### ~~3.2 `RenderIssue` returns `(string, error)` but error is always nil (internal/display/issue.go:12)~~

**Fixed:** Changed signature to `func RenderIssue(issue *models.Issue) string`. Updated all call sites in `cmd/tira/get.go`, `internal/app/kanban.go`, and `internal/display/display_test.go`.

### ~~3.3 Duplicated `containsCI` function~~

**Fixed:** Added documentation comment to `internal/validator/validate.go` explaining the intentional duplication due to architecture constraints (validator must remain dependency-free).

### ~~3.4 O(n²) key lookup in `bulkOperation` (internal/api/client.go:1488–1493)~~

**Fixed:** Built a `keyToIdx` map before the worker loop, reducing lookup from O(n) to O(1) per operation.

### ~~3.5 Duplicated picker overlay rendering~~

**Fixed:** Extracted `tui.RenderPickerOverlay(pickerView func(innerW, listH int) string, title string, totalW, totalH int) string` helper in `internal/tui/helpers.go`. Updated all picker overlay functions in:
- `internal/app/board_overlays.go`
- `internal/app/kanban_view.go`
- `internal/app/backlog_view.go`

### ~~3.6 Duplicated parallel board data fetch~~

**Fixed:** Extracted `fetchBoardDataCore(client, boardID)` helper function. Both `FetchBoardData` and `refreshCmd` now reuse this shared logic.

### ~~3.7 Unnecessary `resp, err := ...; return resp, err` (internal/debug/logger.go:106–108)~~

**Fixed:** Changed to direct return: `return t.Base.RoundTrip(req)`.

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

#### T3. `editFormState.toIssueFields` roundtrip (`internal/app/edit_cmds.go`)
This is the bridge between the TUI form and the API — test that form values serialize correctly:

```go
func TestToIssueFields_AllFields(t *testing.T) { ... }
func TestToIssueFields_AssigneeUnchanged_ReusesID(t *testing.T) { ... }
func TestToIssueFields_AssigneeChanged_ResolvesID(t *testing.T) { ... }
func TestToIssueFields_EmptyStoryPoints(t *testing.T) { ... }
func TestToIssueFields_LabelsCommaSeparated(t *testing.T) { ... }
```

#### T4. `buildColumns` kanban mapping (`internal/app/kanban.go`)
Test that issues land in the correct column based on status ID, and that unmapped statuses fall to the last column:

```go
func TestBuildColumns_MapsStatusIDs(t *testing.T) { ... }
func TestBuildColumns_UnmappedStatusFallsToLast(t *testing.T) { ... }
func TestBuildColumns_EmptyInput(t *testing.T) { ... }
```

#### T5. `blBuildRows` and `blMatchesFilter` (`internal/app/backlog.go`)
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
| ~~P1 — Correctness~~ | ~~4~~ | ~~Data races, wrong results, dead code~~ ✅ Done |
| ~~P2 — API Hygiene~~ | ~~5~~ | ~~Context propagation, consistency, error handling~~ ✅ Done (except 2.1 context.Context) |
| ~~P2.5 — Project Structure~~ | ~~6~~ | ~~Models in package main, inconsistent splits, misleading names, mixed concerns~~ ✅ Done |
| ~~P3 — Code Quality~~ | ~~7~~ | ~~Duplication, unnecessary allocations, idioms~~ ✅ Done |
| Tests | 8+ | API parsing, concurrency, form logic, kanban mapping |
