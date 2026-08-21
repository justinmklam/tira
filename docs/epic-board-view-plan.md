# Epic Board View Implementation Plan

## Executive Summary

Add **Epics** as a third view in the unified board TUI. The view will derive its contents from the same ordered `[]models.SprintGroup` used by the backlog, include only epics referenced by visible project issues, and order each epic by the position of its first child issue in the flattened sprint/backlog sequence.

The recommended approach introduces an `epicModel` in `internal/app` without adding a Jira API method. This avoids discrepancies between Jira search order and board order, supports progressive loading naturally, and keeps the epic list synchronized with refreshes and local backlog changes.

## Confirmed Product Contract

| Area | Decision |
|---|---|
| Placement | Third board view alongside Backlog and Kanban |
| Scope | Only epics referenced by issues in the loaded active/future sprints or backlog |
| Ordering | Position of the first child issue in flattened sprint/backlog order |
| Loading | Progressive: show discovered epics immediately and update after lazy loading |
| Navigation | `1` Backlog, `2` Kanban, `3` Epics; `Tab` cycles all three |
| Primary action | `Enter` opens the full epic detail overlay |
| Other actions | Navigate, show sidebar details, open in Jira, and filter/jump to backlog |
| Proposed backlog action | `b` applies the selected epic as the backlog filter and switches to Backlog |

## ASCII Mockups

### Epic list view

The list follows the first represented child issue's position in the flattened
sprint/backlog order. The sidebar follows the existing Backlog split-pane
pattern.

```text
+--------------------------------------------------------------------------------------------------+
| Backlog [1]   Kanban [2]   Epics [3]                              R refresh  ? help  q quit      |
+---------------------------------------------+----------------------------------------------------+
| EPICS                                       | Selected Epic                                     |
|                                             |                                                    |
| > PLATFORM-42  Platform modernization       | PLATFORM-42  Platform modernization               |
|   AUTH-17      Authentication                | Type: Epic                                        |
|   CHECKOUT-9   Checkout improvements         | Children: 8                                       |
|                                             | First appears: Sprint 12                         |
|                                             |                                                    |
+---------------------------------------------+----------------------------------------------------+
| j/k move  enter details  b filter backlog  o open Jira       1/2/3 views  tab next  ctrl+d/u scroll |
+--------------------------------------------------------------------------------------------------+
```

### Epic detail overlay

`Enter` opens the selected epic in a scrollable detail overlay. The body can
reuse the existing issue-to-Markdown/Glamour rendering path, but the footer
should expose only epic-supported actions.

```text
                         +------------------------------------------------------+
                         | PLATFORM-42  Platform modernization                   |
                         |                                                      |
                         | Type       Epic                                       |
                         | Status     In Progress                               |
                         | Priority   High                                       |
                         | Children   8                                          |
                         |                                                      |
                         | # Description                                        |
                         | Modernize the platform foundations...                 |
                         |                                                      |
                         |                                                      |
                         | o: open in browser   esc/q: back   j/k: scroll       |
                         +------------------------------------------------------+
```

### Progressive loading

The initial view renders from the first loaded sprints, then grows when the
remaining sprints and backlog arrive.

```text
Initial render                         After lazy load
+----------------------------------+    +----------------------------------+
| Epics  (loading more...)         |    | Epics                            |
| > PLATFORM-42  Platform          |    | > PLATFORM-42  Platform          |
|   AUTH-17      Authentication     |    |   AUTH-17      Authentication     |
|                                  |    |   CHECKOUT-9   Checkout            |
|                                  |    |                                  |
| Loading remaining sprints...     |    | All board data loaded            |
+----------------------------------+    +----------------------------------+
                 ------------------------>
```

## Current State

| Concern | Current implementation | Implication |
|---|---|---|
| Board shell | `internal/app/board.go` owns `blModel` and `kanbanModel` | Add `epicModel` as a third peer model |
| Board data | `BoardInitData.Groups` contains ordered sprint groups; backlog is appended during lazy load | The required epic order can be derived without another API call |
| Progressive loading | First three sprints load initially; remaining sprints and backlog arrive through `blLazyLoadDoneMsg` | Epic rows must rebuild incrementally and expose loading/error state |
| Board ordering | Issue slices returned by Agile sprint/backlog endpoints are used directly | Iterating groups and issues in slice order reproduces the displayed board order |
| Epic metadata | Agile issues contain `EpicKey` and `EpicName` when their parent is an Epic | Enough data exists to build the list, title, and counts |
| Epic lookup API | `GetEpics` is alphabetical, capped at 50, and used by pickers | It does not satisfy the chosen scope or ordering contract |
| Issue details | `GetIssue`, `fetchIssueCmd`, `renderIssueContent`, and detail/sidebar renderers already exist | Reuse these for selected epic details |
| Backlog filtering | `blModel.filterEpic` and `blBuildRows` already support epic filtering | Extract/reuse a setter so the epic view can apply the filter consistently |
| Refresh | `R` replaces all groups and rebuilds backlog/kanban | Rebuild the epic projection from the same refreshed groups |
| Local moves | Backlog performs optimistic in-memory reorder/move operations | Rebuild epics when entering the view so first-child order reflects local state |
| Parent changes | Targeted `GetIssue` refresh preserves old `EpicKey`/`EpicName` | Parent-changing operations need an authoritative board refresh |
| Documentation | Docs describe exactly two board views | README, CLI, architecture, state-machine, and keybinding docs must be updated |

## Architectural Constraints

- Keep all Bubble Tea models in `internal/app`.
- Do not add internal-package dependencies to `internal/tui`.
- Keep the Jira client interface unchanged unless implementation reveals missing authoritative data.
- Preserve progressive initial rendering and avoid blocking startup on epic-specific requests.
- Preserve existing Backlog and Kanban behavior, including overlays and gated view switching.
- Treat the backlog groups as the source of truth for epic membership and order.
- Surface lazy-load failures in the epic view rather than presenting a complete-looking partial list.

## Options Considered

### Option A: Derived `epicModel` from board groups

Create a third Bubble Tea model that projects unique epics from ordered sprint groups. The first occurrence of an `EpicKey` establishes its display order; later children update counts and summary metadata. The model is refreshed from `m.backlog.groups` at board lifecycle boundaries and whenever the user enters the epic view.

**Implementation sketch**

1. Add `ViewEpics` and wire three-view switching in `boardModel`.
2. Add `internal/app/epic.go` and `epic_view.go`.
3. Implement a pure `buildEpicItems(groups)` projection helper.
4. Reuse the existing issue-detail and sidebar rendering/fetch helpers.
5. Rebuild after initial load, lazy load, full refresh, and before switching into Epics.
6. Add an epic result action that applies the backlog epic filter and switches views.
7. Update tests and documentation.

| Pros | Cons |
|---|---|
| Exact match to board membership and ordering | Depends on all relevant board groups eventually loading |
| No extra Jira request or pagination path | Needs explicit synchronization after local mutations |
| Progressive loading is straightforward | Epics with no represented child are intentionally absent |
| Small, reversible change isolated to TUI layer | Parent mutations require a full refresh for authoritative metadata |

**Risk and rollback**

- Main risk: stale projection after a mutation path that changes group order or epic parent.
- Mitigation: centralize projection rebuilds and force full refresh for parent changes.
- Rollback: remove `ViewEpics`, the epic model files, and related switching/docs; no persisted data or API contract changes.

### Option B: Fetch project epics and overlay board positions

Use `Client.GetEpics` to fetch epics, build a map of first-child board positions, discard unrepresented results, and sort the remainder by those positions.

**Implementation sketch**

1. Expand `GetEpics` to paginate and return fields needed by the page.
2. Add epic fetch state to initial/lazy board loading.
3. Join fetched epics against `EpicKey` values from board groups.
4. Sort joined records by first child position.
5. Add the same epic TUI and navigation work as Option A.

| Pros | Cons |
|---|---|
| Epic summaries come directly from epic issues | Adds an unnecessary API call for the chosen scope |
| Could later expand to unrepresented epics | Existing endpoint is alphabetical and capped at 50 |
| Separates catalog metadata from child issues | More loading/error states and cache invalidation |
| | Join timing complicates progressive updates |

**Risk and rollback**

- Main risk: partial or inconsistent results when epic search and board loading complete at different times.
- Rollback requires reverting both TUI and API/cache changes.

### Option C: Introduce a shared board projection/index layer

Refactor backlog, kanban, and epics to consume a new shared board-state abstraction that owns sprint groups and derived indexes. The epic list would become one projection alongside backlog rows and kanban columns.

**Implementation sketch**

1. Add a shared board-state type in `internal/app`.
2. Move group replacement, append, issue patch, insert, and optimistic move logic behind it.
3. Make backlog, kanban, and epic models consume projections from shared state.
4. Rework board message routing and mutation synchronization.
5. Add broad regression coverage for all three views.

| Pros | Cons |
|---|---|
| Strong long-term consistency across views | Large refactor for a focused feature |
| Centralizes derived state and mutation handling | Higher regression risk in mature backlog behavior |
| Makes future aggregate views easier | Harder to review and roll back |
| Avoids ad hoc synchronization hooks | Delays user-visible value |

**Risk and rollback**

- Main risk: regressions across backlog movement, filtering, sidebar state, and kanban refresh.
- Rollback is a broad code revert because existing models would be restructured.

## Comparison Matrix

Scores are 1-5, where 5 is best.

| Criterion | Weight | Option A | Option B | Option C |
|---|---:|---:|---:|---:|
| Complexity | 20% | 5 | 3 | 2 |
| Operational risk | 25% | 5 | 3 | 3 |
| Maintainability | 20% | 4 | 3 | 5 |
| Performance | 15% | 5 | 2 | 5 |
| Reversibility | 20% | 5 | 4 | 2 |
| **Weighted score** | **100%** | **4.8** | **3.0** | **3.3** |

## Recommendation

Use **Option A: Derived `epicModel` from board groups**.

- It directly implements the confirmed board-represented scope and first-child ordering rule.
- It avoids the current `GetEpics` limit and alphabetical ordering mismatch.
- It preserves progressive loading with no additional startup request.
- It is isolated, testable, and reversible without changing persisted data or the `api.Client` interface.
- Its synchronization risks are bounded and can be handled at explicit board lifecycle points.

## Detailed Implementation Plan

### Phase 1: Define the epic projection and model

1. Add `internal/app/epic.go` with:
   - `epicState` values for list, loading detail, and detail overlay.
   - `epicItem` containing epic key, summary, first group name/state, child count, and optional aggregate story points.
   - `epicResult` for cross-model actions such as `filterBacklogKey`, refresh, and quit.
   - `epicModel` state for items, cursor, offset, dimensions, sidebar content/cache, loading status, and lazy-load error.
2. Implement `buildEpicItems(groups []models.SprintGroup) []epicItem`:
   - Iterate groups in existing slice order.
   - Iterate each group's issues in existing slice order.
   - Ignore issues without `EpicKey`.
   - Create an item only on the epic's first occurrence.
   - Preserve that first occurrence as the sort position.
   - Increment child count and aggregate points for later children.
   - Prefer `EpicName`; fall back to the epic key if the name is absent.
3. Implement `refreshData(groups, loading, err)`:
   - Preserve the selected epic by key when possible.
   - Clamp cursor/offset when items disappear.
   - Refresh the sidebar preview and full-detail request only when selection changes.
4. Reuse `fetchIssueCmd`, `renderIssueContent`, `renderSidebarContent`, and the existing Glamour rendering path instead of duplicating issue display logic. Generalize or wrap the existing detail border renderer so the epic overlay does not expose out-of-scope edit/comment actions.

### Phase 2: Build the epic view and interactions

1. Add `internal/app/epic_view.go`.
2. Render a split-pane page consistent with Backlog:
   - Top bar labeled `Epics`.
   - Left list with key, summary, first sprint/backlog location, and child count.
   - Right sidebar showing full selected epic details.
   - Progressive-loading indicator while lazy data is outstanding.
   - Explicit partial-load error if lazy loading fails.
   - Empty state when no represented epics exist.
3. Implement list navigation:
   - `j`/`k`, arrows, `g`/`G`, and page movement consistent with existing models.
   - `ctrl+d`/`ctrl+u` for sidebar scrolling.
4. Implement actions:
   - `Enter`: fetch and open the full epic detail overlay with an epic-specific footer.
   - `Esc`/`q` from detail: return to the epic list.
   - `o`: open the selected epic in Jira.
   - `b`: emit `filterBacklogKey` for the selected epic.
   - `R`: request the standard full-board refresh.
   - `?`: use the existing board help overlay.

### Phase 3: Integrate Epics into `boardModel`

1. Add exported `ViewEpics` after `ViewKanban`.
2. Add `epics epicModel` to `boardModel` and initialize it from `BoardInitData.Groups`.
3. Track whether the lazy board load is pending:
   - Initialize the epic model as loading because `lazyLoadCmd` always fetches backlog and may fetch remaining sprints.
   - On `blLazyLoadDoneMsg`, append groups to backlog, update `initData`, rebuild epics, and clear loading.
   - Preserve and display any lazy-load error in the epic model.
4. On `boardRefreshDoneMsg`, rebuild backlog, kanban, and epics from the same authoritative result.
5. Before switching into `ViewEpics`, rebuild from `m.backlog.groups` so optimistic backlog moves/reorders are reflected even though they occur inside `blModel`.
6. Forward `tea.WindowSizeMsg` and active-view messages to `epicModel`.
7. Extend view routing in `board_overlays.go` to return `m.epics.View()`.
8. Extend global behaviors:
   - `Tab`: Backlog -> Kanban -> Epics -> Backlog.
   - `1`, `2`, `3`: direct navigation.
   - `canSwitchView`: allow switching only from the epic list state.
   - `canOpenInBrowser`: support epic list and detail states.
   - `issueURL`: use the standard Jira browse URL for epics.
9. Handle `epicResult.filterBacklogKey`:
   - Apply the filter through a backlog helper.
   - Rebuild backlog rows.
   - Move the cursor to the first matching issue when available.
   - Refresh the backlog sidebar.
   - Switch to `ViewBacklog`.
10. Handle epic refresh and quit results through the same board-level paths as the existing views.

### Phase 4: Centralize backlog epic filtering and mutation correctness

1. Extract a `blModel.setEpicFilter(epicKey string)` helper from the existing epic picker completion path.
2. Use the helper from both:
   - The backlog's `F` picker.
   - The epic view's `b` action.
3. Ensure the helper resets cursor/offset safely, selects the first matching issue, and updates sidebar state.
4. Change successful parent/epic assignment to request a full board refresh:
   - `GetIssue` currently lacks authoritative agile `EpicKey`/`EpicName`.
   - `patchIssue` intentionally preserves the previous epic fields.
   - A full refresh is therefore required to update epic membership correctly.
5. Review create-with-parent behavior:
   - If a newly created issue can be assigned to an epic in the create form, use a full board refresh after creation when `ParentKey` is set.
   - Preserve the current targeted insert path for creations without an epic parent.
6. Confirm local move/reorder paths are reflected by the rebuild-before-entry rule; avoid coupling `blModel` directly to `epicModel`.

### Phase 5: CLI and documentation

1. Update `cmd/tira/board.go`:
   - Accept `--view epics`.
   - Update command descriptions/examples from two views to three.
2. Update `cmd/tira/board_test.go` for `epics`, case-insensitivity, and invalid-value messaging.
3. Update:
   - `README.md`
   - `docs/cli-commands.md`
   - `docs/keybindings-backlog.md`
   - `docs/architecture.md`
   - `docs/tui-architecture.md`
   - `docs/state-machines.md`
   - `docs/internal-packages.md` if model inventory is documented there
4. Document the exact scope: active/future sprints plus backlog, excluding epics with no represented child.
5. Document progressive behavior and the possibility of the list growing while background loading completes.

### Phase 6: Test and verify

1. Add `internal/app/epic_test.go` covering:
   - First-child ordering across multiple sprint groups and backlog.
   - Deduplication of epics with multiple children.
   - Child counts and aggregate points.
   - Fallback when `EpicName` is empty.
   - Ignoring issues with no epic.
   - Progressive append behavior.
   - Preserving selected epic by key after rebuild.
   - Empty-list cursor safety.
2. Add epic model update tests covering:
   - Navigation and scroll clamping.
   - Detail loading and return.
   - `b`, `R`, quit, and sidebar scroll results.
3. Add board integration tests covering:
   - Three-way `Tab` cycling.
   - Direct `1`/`2`/`3` switching.
   - Switching gates while epic detail is open.
   - Lazy-load and full-refresh synchronization.
   - Applying an epic filter and switching to the backlog.
4. Add or extend backlog tests for the shared epic-filter helper and parent-change refresh behavior.
5. Run the repository-required validation after implementation:
   - `make fmt`
   - `make check`

## Key Files

### New files

- `internal/app/epic.go`
- `internal/app/epic_view.go`
- `internal/app/epic_test.go`

### Primary modifications

- `internal/app/board.go`
- `internal/app/board_overlays.go`
- `internal/app/backlog.go`
- `internal/app/backlog_update.go`
- `cmd/tira/board.go`
- `cmd/tira/board_test.go`

### Documentation modifications

- `README.md`
- `docs/cli-commands.md`
- `docs/keybindings-backlog.md`
- `docs/architecture.md`
- `docs/tui-architecture.md`
- `docs/state-machines.md`
- `docs/internal-packages.md`

## Validation Criteria

- Epic membership comes only from the selected project's loaded board groups.
- Epic order exactly matches the first represented child issue's flattened board position.
- Duplicate child issues never create duplicate epic rows.
- Progressive loading does not reset the user's selected epic when the key remains present.
- Lazy-load failures are visible and do not masquerade as a complete list.
- `Enter` opens epic details; `b` applies the backlog epic filter; `o` opens Jira.
- Backlog, Kanban, and Epics switch correctly through `Tab` and keys `1`/`2`/`3`.
- Full refresh updates all three views from one authoritative dataset.
- Parent changes update epic membership through an authoritative refresh.
- Existing backlog move, filter, edit, create, sidebar, and kanban behaviors remain intact.
- Documentation and CLI help match implemented behavior.

## Current Implementation Status

- The Epic list is implemented with backlog-style fixed-width columns and aligned
  headers for `KEY`, `SUMMARY`, `FIRST APPEARS`, `SP`, and `CHILDREN`.
- Epic keys use the shared deterministic epic palette; sprint locations use
  deterministic board-order palette colors; selected rows use the shared
  backlog surface/highlight treatment.
- Story-point formatting is shared between Backlog and Epics.
- Rendering and formatting tests cover column alignment, displayed values, and
  story-point edge cases.
- Full Go tests, vet, formatting, and diff checks pass. The repository's
  `golangci-lint` check remains environment-blocked because the installed linter
  requires Go 1.27.

## Notes and Non-Goals

- This plan does not list epics that have no child issue in the active/future sprints or backlog.
- This plan does not add epic creation, editing, ranking, or drag/move behavior to the epic page.
- The existing alphabetical `GetEpics` method remains available for parent/filter pickers and is not used by the page.
- Closed sprints remain outside the epic page because the current board data fetch requests only active and future sprints plus backlog.
