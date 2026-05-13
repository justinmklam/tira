# Bubble Tea v2 / Lip Gloss v2 Migration Spec

## Overview

Migrate the tira codebase from Charm v1 libraries to v2:

| Library | Current | Target |
|---------|---------|--------|
| bubbletea | `github.com/charmbracelet/bubbletea v1.3.10` | `charm.land/bubbletea/v2` |
| lipgloss | `github.com/charmbracelet/lipgloss v1.1.1-...` | `charm.land/lipgloss/v2` |
| bubbles | `github.com/charmbracelet/bubbles v1.0.0` | `charm.land/bubbles/v2` |
| glamour | `github.com/charmbracelet/glamour v1.0.0` | `charm.land/glamour/v2` |
| huh | `github.com/charmbracelet/huh v1.0.0` | `charm.land/huh/v2` |

**Glamour v2** is now available at `charm.land/glamour/v2` and must be migrated. Key breaking changes: `WithAutoStyle()` removed (use explicit `WithStylePath("dark")`), `WithColorProfile()` removed (color downsampling moved to lipgloss output layer), and import path changes for `glamour/ansi` and `glamour/styles`. The `ansi.StyleConfig` struct is unchanged — only import paths change.

**Huh v2** depends on bubbletea/lipgloss v2. It's used in one place (`cmd/tira/get.go`) for a simple confirmation dialog. It must be migrated, but its v2 API is straightforward.

---

## Phase 0: Dependency Update & Build Baseline

### Task 0.1 — Update go.mod imports

- Replace all `github.com/charmbracelet/bubbletea` imports with `charm.land/bubbletea/v2`
- Replace all `github.com/charmbracelet/lipgloss` imports with `charm.land/lipgloss/v2`
- Replace all `github.com/charmbracelet/bubbles/spinner` imports with `charm.land/bubbles/v2/spinner`
- Replace all `github.com/charmbracelet/bubbles/textarea` imports with `charm.land/bubbles/v2/textarea`
- Replace all `github.com/charmbracelet/bubbles/textinput` imports with `charm.land/bubbles/v2/textinput`
- Replace all `github.com/charmbracelet/bubbles/viewport` imports with `charm.land/bubbles/v2/viewport`
- Replace `github.com/charmbracelet/huh` import with `charm.land/huh/v2`
- Replace `github.com/charmbracelet/glamour` import with `charm.land/glamour/v2`
- Replace `github.com/charmbracelet/glamour/ansi` import with `charm.land/glamour/v2/ansi`
- Replace `github.com/charmbracelet/glamour/styles` import with `charm.land/glamour/v2/styles`
- Run `go get charm.land/bubbletea/v2 charm.land/lipgloss/v2 charm.land/bubbles/v2 charm.land/huh/v2 charm.land/glamour/v2` to resolve versions
- Run `go mod tidy`

**Files:** All `.go` files importing Charm packages (18 files)

**Expected result:** Compilation fails with type errors (addressed in subsequent tasks). This task just updates import paths and resolves module versions.

### Task 0.2 — Verify bubbles v2 API compatibility

Before starting the migration, verify the bubbles v2 API by checking:

- `spinner.Model` — does `.Spinner`, `.Style`, `.Tick`, `.Update()`, `.View()` still work?
- `textinput.Model` — are `.Placeholder`, `.CharLimit`, `.Width`, `.Prompt` direct fields or methods now?
- `textarea.Model` — is `.ShowLineNumbers` still a direct field?
- `viewport.New(w, h)` — does it still accept dimensions, or is it `viewport.New()` + `SetSize()`?
- `viewport.Model` — are `.Width`/`.Height` direct fields or setters?

Read the bubbles v2 source or changelog to confirm. Update this spec if any APIs differ.

**Estimated effort:** 30 min research

---

## Phase 1: Lip Gloss v2 Migration (Bottom-Up)

Start with `internal/tui` since it's a leaf package (no internal dependencies). Other packages depend on it, so migrating it first reduces cascading changes.

### Task 1.1 — Migrate `lipgloss.Color` type usage to `color.Color`

In v2, `lipgloss.Color` changed from a `type string` alias to a `func(string) color.Color` function. All places that use `lipgloss.Color` as a **type** (in struct fields, function return types, slice element types) must change.

**Changes in `internal/tui/theme.go`:**

- Add `import "image/color"` (or use the concrete type `lipgloss.Color` returns)
- Change `Theme` struct fields from `lipgloss.Color` to `color.Color`:

```go
// Before
type Theme struct {
    Error            lipgloss.Color
    Success          lipgloss.Color
    ...
    EpicPalette []lipgloss.Color
}

// After
type Theme struct {
    Error            color.Color
    Success          color.Color
    ...
    EpicPalette []color.Color
}
```

- Change `string(t.Accent)` at line 134 — since `color.Color` is an interface, not a string, we need a different approach. Convert using the `lipgloss.Color` function result. The simplest approach: keep a `colorStr` helper or change `GlamourStyleConfig.Heading.Color` assignment to use an explicit string from theme data.

**Approach for `string(t.Accent)`:** The `GlamourStyleConfig.Heading.Color` field is `*string`. In v1, `lipgloss.Color` was a string type so `string(t.Accent)` worked. In v2, we need to store the original color string separately. Options:
  - (A) Add a `ColorStrings` struct or `AccentStr` field to `Theme` that holds the original string values for glamour config. Then assign from `t.AccentStr` instead of `string(t.Accent)`.
  - (B) Define a helper `colorToString(color.Color) string` that type-asserts to `lipgloss.Color`-returned types and extracts the string.
  - **Recommendation:** Option A is cleaner. Add an `AccentStr string` field to `Theme` alongside `Accent color.Color`. Each theme entry already uses string literals, so we just duplicate the accent string.

**Changes in `internal/tui/styles.go`:**

- Change `IssueTypeColor()` return type from `lipgloss.Color` to `color.Color`
- Change `EpicColor()` return type from `lipgloss.Color` to `color.Color`
- Change `DaysColor()` return type from `lipgloss.Color` to `color.Color`
- Change `epicPalette` variable from `[]lipgloss.Color` to `[]color.Color`
- Change `Color*` package-level vars from type `lipgloss.Color` (which was a string alias) — since `lipgloss.Color("12")` now returns `color.Color`, the var types change implicitly. But since `lipgloss.Color` is now a function not a type, we can't use it as a type annotation. Change:

```go
// Before
var ColorSpinner lipgloss.Color = lipgloss.Color("12")

// After
var ColorSpinner color.Color = lipgloss.Color("12")
```

**Note on `epicPalette` default values:** The default `epicPalette` uses bare strings like `"39"`, `"208"` which were implicitly `lipgloss.Color` (when `lipgloss.Color` was `string`). In v2, `lipgloss.Color("39")` must be called explicitly. So:

```go
// Before
var epicPalette = []lipgloss.Color{"39", "208", ...}

// After
var epicPalette = []color.Color{lipgloss.Color("39"), lipgloss.Color("208"), ...}
```

**Important:** The `EpicColor("")` path currently returns `""` (empty string). In v2, it should return `nil` (since `color.Color` is an interface). Callers already use `EpicColor(key)` in `.Foreground()` calls and `.Foreground(nil)` is valid in v2 lipgloss (it means "unset").

**Files:** `internal/tui/theme.go`, `internal/tui/styles.go`

### Task 1.2 — Fix `lipgloss.Place` calls

`lipgloss.Place` signature is unchanged in v2 (it still returns a string). No changes needed unless `WithWhitespaceForeground` or `WithWhitespaceBackground` options were used (they're not — this project only uses the basic `lipgloss.Place(w, h, hPos, vPos, content)` form).

**Files:** No changes needed. ✅

### Task 1.3 — Update `lipgloss.NewStyle()` usage

In v2, `lipgloss.NewStyle()` no longer carries a renderer. All existing calls are just `lipgloss.NewStyle()` which still works. The renderer was never explicitly used in this codebase.

**Files:** No changes needed. ✅

### Task 1.4 — Update theme color string handling for glamour

Since `lipgloss.Color` is no longer a string type, storing color strings for glamour config requires a parallel mechanism. 

**Approach:** Add a `themeColors` helper struct or map that stores original color strings alongside `color.Color` values. Since glamour v1 expects `*string` for color fields, we only need the accent string.

**Changes in `internal/tui/theme.go`:**

- Add an `AccentStr string` field to `Theme`
- Set it in each theme definition alongside `Accent`
- Replace `string(t.Accent)` with `t.AccentStr`

```go
type Theme struct {
    // ...
    Accent    color.Color
    AccentStr string // original string for glamour config
    // ...
}
```

**Files:** `internal/tui/theme.go`, `internal/tui/theme_test.go`

### Task 1.5 — Fix `internal/tui` compilation

After Tasks 1.1 and 1.4, run `go build ./internal/tui/...` and fix any remaining type errors. This package has **no internal dependencies** so it should compile cleanly once these changes are in.

**Verification:** `go build ./internal/tui/...` succeeds, `go test ./internal/tui/...` passes.

---

## Phase 2: Bubble Tea v2 Migration — Core Types

### Task 2.1 — Change `View() string` to `View() tea.View`

Every type implementing the `tea.Model` interface must change its `View()` return type.

**Files and types to change:**

| Type | File | Change |
|------|------|--------|
| `boardModel` | `internal/app/board_overlays.go` | `View() string` → `View() tea.View` |
| `kanbanModel` | `internal/app/kanban_view.go` | `View() string` → `View() tea.View` |
| `blModel` | `internal/app/backlog_view.go` | `View() string` → `View() tea.View` |
| `spinnerModel[T]` | `internal/tui/spinner.go` | `View() string` → `View() tea.View` |
| `*editModel` | `internal/app/edit_form.go` | `View() string` → `View() tea.View` |
| `*commentInputModel` | `internal/app/comment_form.go` | `View() string` → `View() tea.View` |

For each, wrap the return value with `tea.NewView(content)`:

```go
// Before
func (m boardModel) View() string {
    // ... builds string content
    return content
}

// After
func (m boardModel) View() tea.View {
    // ... builds string content
    return tea.NewView(content)
}
```

**Important:** Sub-models that do NOT implement `tea.Model` (like `PickerModel`, `OptionPickerModel`, `HelpModel`) have `View()` methods with custom signatures (e.g., `View(innerW, maxRows int) string`). These return plain strings because they're internal rendering methods, not the Bubbletea interface. Leave them as is.

**Complication:** `boardModel.View()` delegates to sub-models (backlog, kanban, edit form, comment form, overlays). Since `blModel.View()` will now return `tea.View`, and `boardModel.View()` needs to return `tea.View`, the delegation chain must be updated:

```go
// Before
case ViewKanban:
    return m.kanban.View()
default:
    return m.backlog.View()

// After — extract content from sub-model's tea.View
case ViewKanban:
    v := m.kanban.View()
    return v
default:
    v := m.backlog.View()
    return v
```

Since `tea.View` can be returned directly, delegation works fine.

**Complication:** `renderIssueDetailView` returns a `string` and is called from both `kanban_view.go` and `backlog_view.go`. It uses `lipgloss.Place` which returns a string. The callers wrap this in `tea.NewView()`. The function itself doesn't need to change.

**Files:** 6 files listed above

### Task 2.2 — Handle sub-view string composition

Many `View()` methods compose strings from sub-views. For example:

```go
// board_overlays.go — spinner views return strings from .View()
msg := m.editSpinner.View() + tui.MutedStyle.Render(" Fetching issue…")
return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, msg)
```

In v2, `spinner.Model.View()` still returns `string` (bubbles components' `View()` methods still return `string`). Same for `textinput.Model.View()`, `textarea.Model.View()`, and `viewport.Model.View()`. So string composition with sub-component views continues to work.

Only the **top-level** `View() tea.View` methods need wrapping. Internal string building is fine.

**Key rule:** The only methods that return `tea.View` are the ones implementing `tea.Model`. Everything else returns `string`.

**No changes needed for:** `PickerModel.View()`, `OptionPickerModel.View()`, `HelpModel.View()`, `renderIssueDetailView()`, all lipgloss string composition, `spinner.Model.View()`, `textinput.Model.View()`, etc.

**Files:** No additional changes beyond Task 2.1.

### Task 2.3 — Move `tea.WithAltScreen()` to declarative `View`

**Current code in `internal/app/board.go:1004`:**
```go
p := tea.NewProgram(m, tea.WithAltScreen())
```

**After:**
```go
p := tea.NewProgram(m)
```

And in `boardModel.View()`:
```go
func (m boardModel) View() tea.View {
    v := tea.NewView(content)
    v.AltScreen = true
    return v
}
```

**Current code in `internal/tui/spinner.go:69-73`:**
```go
p := tea.NewProgram(spinnerModel[T]{...}, tea.WithOutput(os.Stderr))
```

`tea.WithOutput` still exists in v2 but `tea.WithAltScreen` is removed. The spinner doesn't use alt screen, so no change needed. However, `tea.WithOutput` may have been renamed or removed — verify in v2 API.

**Files:** `internal/app/board.go`, `internal/app/board_overlays.go`

### Task 2.4 — Move mouse mode to declarative `View`

The board TUI uses mouse support. In v1, this was set via program options. In v2, it's set in `View()`.

**Check if `tea.WithMouseCellMotion()` is used.** Search found it's NOT used currently — the program doesn't enable mouse tracking. No change needed.

**Files:** No changes needed. ✅

### Task 2.5 — Replace `tea.KeyMsg` with `tea.KeyPressMsg`

All type assertions on `tea.KeyMsg` must change to `tea.KeyPressMsg`. This is the most pervasive change.

**Exact changes needed:**

1. **`internal/app/backlog_update.go`** — 12 occurrences of `case tea.KeyMsg:` or `msg.(tea.KeyMsg)` → `case tea.KeyPressMsg:` or `msg.(tea.KeyPressMsg)`. Also add import alias if not already present.

2. **`internal/app/board.go`** — 6 occurrences

3. **`internal/app/kanban.go`** — 3 occurrences

4. **`internal/app/edit_form.go`** — 1 occurrence

5. **`internal/app/comment_form.go`** — 1 occurrence

6. **`internal/tui/picker.go`** — 1 occurrence at line 161: `key, ok := msg.(tea.KeyMsg)` → `key, ok := msg.(tea.KeyPressMsg)`

7. **`internal/tui/option_picker.go`** — 1 occurrence at line 46: `key, ok := msg.(tea.KeyMsg)` → `key, ok := msg.(tea.KeyPressMsg)`

8. **`internal/tui/help.go`** — 1 occurrence at line 170

**Pattern for switch statements:**
```go
// Before
switch msg := msg.(type) {
case tea.KeyMsg:
    switch msg.String() {

// After
case tea.KeyPressMsg:
    switch msg.String() {
```

**Pattern for type assertions:**
```go
// Before
key, ok := msg.(tea.KeyMsg)

// After
key, ok := msg.(tea.KeyPressMsg)
```

**Files:** 8 files with key handling

### Task 2.6 — Change space bar match from `" "` to `"space"`

One occurrence found at `internal/app/backlog_update.go:211`:

```go
// Before
case " ":

// After
case "space":
```

**Files:** `internal/app/backlog_update.go`

### Task 2.7 — Rename `tea.Sequentially` → `tea.Sequence` (if used)

Not used in this codebase. **No changes needed.** ✅

### Task 2.8 — Rename `tea.WindowSize()` → `tea.RequestWindowSize` (if used)

Not used as a command. The code uses `tea.WindowSizeMsg` in `Update()` handlers, which is a message type, not a command. `tea.WindowSizeMsg` still exists in v2. **No changes needed.** ✅

---

## Phase 3: Bubbles v2 Migration

The bubbles v2 upgrade guide confirms specific breaking changes. Research (Task 3.1) is no longer needed — exact changes are documented below.

### Task 3.1 — *(Removed — research complete, specifics inlined below)*

### Task 3.2 — Update `spinner.Model` usage

Used in 4 places: `boardModel`, `kanbanModel`, `blModel`, and `spinnerModel[T]`.

**Confirmed v2 changes:**
- Import path: `github.com/charmbracelet/bubbles/spinner` → `charm.land/bubbles/v2/spinner`
- `spinner.NewModel()` removed → use `spinner.New()` (already using this)
- **`spinner.Tick` (package-level function) → `model.Tick()` (method on spinner model)**

Current pattern:
```go
s := spinner.New()
s.Spinner = spinner.Dot
s.Style = lipgloss.NewStyle().Foreground(ColorSpinner)
// And in Init():
return m.spinner.Tick  // ← this changes
```

**Changes required:**

1. Update import path
2. Change all `m.spinner.Tick` / `m.loadSpinner.Tick` / `m.editSpinner.Tick` from field access to method calls: `m.spinner.Tick()`, `m.loadSpinner.Tick()`, `m.editSpinner.Tick()`

In v1, `spinner.Tick` was a package-level `tea.Cmd` constant. In v2, it's a method on `spinner.Model` that returns `tea.Cmd`. All current usages of `m.X.Tick` must add `()`:

| File | Line(s) | Change |
|------|---------|--------|
| `internal/tui/spinner.go` | 29 | `m.spinner.Tick` → `m.spinner.Tick()` |
| `internal/app/kanban.go` | 350 | `m.loadSpinner.Tick` → `m.loadSpinner.Tick()` |
| `internal/app/board.go` | 464, 556, 725, 751, 828, 851 | `m.editSpinner.Tick` → `m.editSpinner.Tick()` |
| `internal/app/backlog.go` | 751 | `m.backlog.loadSpinner.Tick` → `m.backlog.loadSpinner.Tick()` |

**Files:** `internal/app/board.go`, `internal/app/kanban.go`, `internal/app/backlog.go`, `internal/tui/spinner.go`

### Task 3.3 — Update `textinput.Model` usage

Used in `picker.go`, `edit_form.go`, `backlog.go`, `backlog_update.go`.

**Confirmed v2 changes:**
- Import path: `github.com/charmbracelet/bubbles/textinput` → `charm.land/bubbles/v2/textinput`
- **`Model.Width` (field) → `Model.SetWidth()` / `Model.Width()` (getter/setter methods)**
- **`Model.PromptStyle` → `StyleState.Prompt`** (moved into nested styles struct)
- **`Model.PlaceholderStyle` → `StyleState.Placeholder`** (moved)
- **`Model.TextStyle` → `StyleState.Text`** (moved)
- **`Model.CompletionStyle` → `StyleState.Suggestion`** (renamed + moved)
- **`Model.CursorStyle` → `Styles.Cursor`** (moved)
- **`Model.Cursor` (cursor.Model field) → `Model.Cursor()` (func returning `*tea.Cursor`)** — affects v2 cursor integration
- **`textinput.DefaultKeyMap` (var) → `textinput.DefaultKeyMap()` (func)** — not used in this codebase
- **`textinput.NewModel` removed → use `textinput.New()`** — already using `New()`
- `Model.Placeholder`, `Model.CharLimit`, `Model.Prompt` — likely still direct fields (not listed as removed)

**Concrete field-level changes:**

| Current (v1) | New (v2) | Files |
|---|---|---|
| `ti.Width = 50` | `ti.SetWidth(50)` | `edit_form.go`, `backlog_update.go` |
| `m.inputs[i].Width = w` | `m.inputs[i].SetWidth(w)` | `edit_form.go` |
| Reading `Width` as a field | `m.filterInput.Width()` etc. | All files reading `.Width` |

**Style changes:** The codebase doesn't set `PromptStyle`, `TextStyle`, `PlaceholderStyle`, `CompletionStyle`, or `CursorStyle` on textinput — it uses defaults. No style migration needed.

**Files:** `internal/tui/picker.go`, `internal/app/edit_form.go`, `internal/app/backlog.go`, `internal/app/backlog_update.go`

### Task 3.4 — Update `textarea.Model` usage

Used in `edit_form.go` and `comment_form.go`.

**Confirmed v2 changes:**
- Import path: `github.com/charmbracelet/bubbles/textarea` → `charm.land/bubbles/v2/textarea`
- **`Model.FocusedStyle` → `Model.Styles.Focused`** (nested into Styles struct)
- **`Model.BlurredStyle` → `Model.Styles.Blurred`** (nested)
- **`Model.SetCursor(col)` → `Model.SetCursorColumn(col)`** (renamed)
- **`textarea.DefaultKeyMap` (var) → `textarea.DefaultKeyMap()` (func)** — not used in this codebase
- **`textarea.DefaultStyles()` → `textarea.DefaultStyles(isDark bool)`** — now requires bool param
- **`textarea.Style` (type) → `textarea.StyleState` (type)** — renamed
- **`Model.Cursor` (cursor.Model) → `Model.Cursor()` (func → `*tea.Cursor`)** — affects v2 integration

Current usage:
```go
ta := textarea.New()
ta.ShowLineNumbers = false
ta.SetValue("...")
ta.SetWidth(w)
ta.SetHeight(h)
```

**Note on `.ShowLineNumbers`:** Not mentioned in the v2 migration guide as removed or changed. Likely still a direct field. Verify at migration time.

**Style changes:** The codebase doesn't set `FocusedStyle` or `BlurredStyle` on textarea — it uses defaults. No style struct migration needed. However, if `DefaultStyles()` is called anywhere, it now requires `isDark bool`.

**Files:** `internal/app/edit_form.go`, `internal/app/comment_form.go`

### Task 3.5 — Update `viewport.Model` usage

Used in `kanban.go` and `backlog.go`.

**Confirmed v2 changes:**
- Import path: `github.com/charmbracelet/bubbles/viewport` → `charm.land/bubbles/v2/viewport`
- **`viewport.New(w, h)` → `viewport.New(viewport.WithWidth(w), viewport.WithHeight(h))`** or `viewport.New()` + setters
- **`Model.Width` (field) → `Model.SetWidth()` / `Model.Width()` (getter/setter)**
- **`Model.Height` (field) → `Model.SetHeight()` / `Model.Height()` (getter/setter)**
- **`Model.YOffset` (field) → `Model.SetYOffset()` / `Model.YOffset()` (getter/setter)**
- **`HighPerformanceRendering` removed** — not used in this codebase

**Concrete changes:**

```go
// Before (kanban.go:262-263, backlog.go:491-492)
vp.Width = vpW
vp.Height = vpH

// After
vp.SetWidth(vpW)
vp.SetHeight(vpH)
```

```go
// Before (kanban.go:274, backlog.go:522)
vp := viewport.New(vpW, vpH)

// After
vp := viewport.New(viewport.WithWidth(vpW), viewport.WithHeight(vpH))
```

**Any reads of `.Width` / `.Height`** on viewport models must also change from field access to method calls.

**Files:** `internal/app/kanban.go`, `internal/app/backlog.go`

### Task 3.6 — Update `textinput.Blink` and `textarea.Blink`

Used as init commands:
- `internal/app/edit_form.go:142` — `textinput.Blink` (returned in `Init()`)
- `internal/app/comment_form.go:55` — `textarea.Blink` (returned in `Init()`)

These are package-level `tea.Cmd` values used to start cursor blinking. They may still exist in v2 or may have been replaced by the component's own `Init()` method. **Verify at migration time** — if removed, use the model's own `Init()` return value.

**Files:** `internal/app/edit_form.go`, `internal/app/comment_form.go`

### Task 3.7 — Update cursor model references (if any)

The v2 cursor model changed significantly:
- `cursor.Model` → accessed via `Model.Cursor()` func returning `*tea.Cursor`
- `model.Blink` → `model.IsBlinked`
- `model.BlinkCmd()` → `model.Blink()`

The codebase doesn't directly use `cursor.Model` — it's only accessed indirectly through textinput/textarea. However, if the code reads `.Cursor` on any input model as a `cursor.Model`, it must change to `.Cursor()`.

**Search for `.Cursor` accesses on textinput/textarea models** to confirm scope.

**Files:** Potentially `internal/app/edit_form.go`, `internal/app/comment_form.go`

---

## Phase 4: Huh v2 Migration

### Task 4.1 — Migrate `huh` usage in `cmd/tira/get.go`

Single usage at line 151:
```go
huh.NewConfirm().Title("Validation failed. Re-open editor?").Value(&retry).Run()
```

**Confirmed v2 changes:**
- Import path: `github.com/charmbracelet/huh` → `charm.land/huh/v2`
- Theme functions now require `isDark bool` parameter: `huh.ThemeCharm()` → `huh.ThemeCharm(isDark)`
- `WithAccessible()` removed from individual fields (now form-level only)
- All Bubble Tea types updated to v2

Since this code doesn't use `WithTheme()` or `WithAccessible()` on the confirm, the only change is the import path. The `.Title().Value().Run()` chain is preserved in v2.

**Files:** `cmd/tira/get.go`

---

## Phase 5: Glamour v2 Migration

### Task 5.1 — Update glamour import paths

Glamour v2 is now available at `charm.land/glamour/v2`. All imports must be updated:

```diff
-import "github.com/charmbracelet/glamour"
-import "github.com/charmbracelet/glamour/ansi"
-import "github.com/charmbracelet/glamour/styles"
+import "charm.land/glamour/v2"
+import "charm.land/glamour/v2/ansi"
+import "charm.land/glamour/v2/styles"
```

**Files:** `internal/app/board.go` (glamour), `internal/app/backlog.go` (glamour), `internal/tui/theme.go` (glamour/ansi, glamour/styles)

### Task 5.2 — Replace `glamour.WithAutoStyle()` with explicit style selection

**Breaking change:** `glamour.WithAutoStyle()` has been removed in v2. The v1 code used it in two places:

1. **`internal/app/board.go:1001`** — warm-up call to force termenv detection:
   ```go
   _, _ = glamour.NewTermRenderer(glamour.WithAutoStyle())
   ```
   This was a hack to trigger termenv's `sync.Once` before Bubbletea takes over the TTY. In v2, this is unnecessary because:
   - Glamour no longer auto-detects terminal background (it's pure now)
   - Bubbletea v2 handles background color reporting declaratively
   - The termenv cache warm-up is no longer needed

   **Action:** Remove this call entirely.

2. **`internal/app/backlog.go:797-800`** — actual markdown rendering:
   ```go
   renderer, err := glamour.NewTermRenderer(
       glamour.WithStyles(tui.GlamourStyleConfig),
       glamour.WithWordWrap(wrapWidth),
   )
   ```
   This uses `WithStyles()` (not `WithAutoStyle()`), so it's not affected by the `WithAutoStyle()` removal. `WithStyles()` still works in v2.

   **Action:** No change needed to this call site. It keeps working as-is.

**For choosing dark/light style automatically:** If we later want auto-detection, use lipgloss v2's `lipgloss.HasDarkBackground()` or Bubbletea's `tea.BackgroundColorMsg` to pick between `WithStylePath("dark")` and `WithStylePath("light")`. The current code already uses theme-based style configs (`WithStyles(tui.GlamourStyleConfig)`), which always produces consistent output regardless of terminal background — this is the preferred v2 approach.

**Files:** `internal/app/board.go` (remove warm-up call)

### Task 5.3 — Remove `glamour.WithColorProfile()` (if used)

The v2 codebase does not use `WithColorProfile()` — `glamour.WithAutoStyle()` was used instead. `WithColorProfile()` is removed in v2, but since it wasn't used here, no changes are needed.

In v2, color downsampling is handled at the output layer by lipgloss, not by glamour. Since the TUI renders glamour output through the Bubbletea view (which handles color automatically), no `lipgloss.Print()` wrapper is needed for the markdown content that goes into viewports.

**Files:** No changes needed. ✅

### Task 5.4 — Verify custom `ansi.StyleConfig` compatibility

`internal/tui/theme.go` defines a large `catppuccinGlamourStyle` using `ansi.StyleConfig` with `stringPtr("#hexcolor")` fields. In v2, the `ansi.StyleConfig` struct itself is unchanged — only the import path changes.

However, verify that the `Overlined` field was not used anywhere in the custom style config (it's removed in v2). Our `catppuccinGlamourStyle` does not use `Overlined`, so no changes are needed.

**Files:** `internal/tui/theme.go` (import path only, already covered by Task 5.1)

### Task 5.5 — Maintain the `GlamourStyleConfig` package-level variable

The package-level `GlamourStyleConfig` variable and `SetTheme()` function work as follows:

```go
var GlamourStyleConfig = styles.DarkStyleConfig  // default

func SetTheme(name string) error {
    // ...
    if gs, ok := glamourStyles[name]; ok {
        GlamourStyleConfig = gs
    }
    accentStr := t.AccentStr
    GlamourStyleConfig.Heading.Color = &accentStr
    return nil
}
```

In v2, `styles.DarkStyleConfig` is still available (import path changed to `charm.land/glamour/v2/styles`). The `styles.TokyoNightStyleConfig` reference also still works — it was already referenced in `glamourStyles` map.

The `GlamourStyleConfig.Heading.Color = &accentStr` line uses a `*string` field. Since `accentStr` is a plain `string` (not a `lipgloss.Color`), this works the same in v2. The change from Task 1.4 (adding `AccentStr` field to `Theme`) handles this correctly.

**Files:** `internal/tui/theme.go` (covered by Tasks 5.1 and 1.4)

### Task 5.6 — Handle lipgloss v2 dependency compatibility

Glamour v2 depends on `charm.land/lipgloss/v2`, which is the same version we're migrating to. This means there's no dependency conflict — glamour v2 and our direct lipgloss v2 usage will share the same module.

This is a major improvement over the v1 situation where glamour v1 pulled in lipgloss v1 as a transitive dependency.

**Files:** `go.mod` (resolved automatically by `go mod tidy`)

---

## Phase 6: Integration & Testing

### Task 6.1 — Fix compilation errors

After all previous tasks are done:

1. Run `go build ./...` and fix all remaining type errors
2. Common issues to watch for:
   - `lipgloss.Color` used as a type anywhere (should be `color.Color`)
   - `tea.KeyMsg` used anywhere (should be `tea.KeyPressMsg`)
   - `View() string` on `tea.Model` implementors (should be `View() tea.View`)
   - Missing `tea.NewView()` wrapping in top-level views
   - Import path inconsistencies

**Files:** Whatever compilation reveals

### Task 6.2 — Fix color type mismatches in callers of `styles.go`

After Task 1.1 changes `IssueTypeColor()`, `EpicColor()`, and `DaysColor()` to return `color.Color`, callers that pass these to `.Foreground()` must work. Since lipgloss v2's `.Foreground()` accepts `color.Color`, this should be fine. But verify there are no places that assign the return value to a `lipgloss.Color`-typed variable.

**Files:** All files that call `IssueTypeColor()`, `EpicColor()`, `DaysColor()`

### Task 6.3 — Run full test suite

```bash
make test
make test-race
```

### Task 6.4 — Run linters

```bash
make fmt
make vet
make lint
```

### Task 6.5 — Manual smoke test

Launch the TUI and verify:
- Board renders correctly (alt screen)
- Keyboard navigation works (j/k/enter/escape/space)
- Mouse interaction (if applicable)
- Colors and styles render correctly
- Theme switching works
- Edit form works (text input, textarea)
- Comment form works
- Picker (fuzzy search) works
- Option picker works
- Help overlay works
- Spinner displays during loading
- Sprint form (text inputs) works

---

## Task Dependency Graph

```
Phase 0 (imports) ─────────────────────────────────────────┐
                                                              │
Phase 1 (lipgloss) ──────────────────────────────────────────┤
  1.1 (Color type) → 1.4 (theme strings) → 1.5 (compile)   │
  1.2 (Place) ✅    1.3 (NewStyle) ✅                         │
                                                              │
Phase 2 (bubbletea) ─────────────────────────────────────────┤
  2.1 (View()) → 2.2 (sub-views) → 2.3 (AltScreen)         │
  2.5 (KeyPressMsg) → 2.6 (space)                            │
  2.4 (mouse) ✅  2.7 (Sequence) ✅  2.8 (WindowSize) ✅     │
                                                              │
Phase 3 (bubbles) ───────────────────────────────────────────┤
  3.2 (spinner.Tick()) → 3.3 (textinput.Width)               │
  3.4 (textarea) → 3.5 (viewport.New+SetWidth)               │
  3.6 (Blink) → 3.7 (cursor model)                           │
                                                              │
Phase 4 (huh) ───────────────────────────────────────────────┤
  4.1 (import path only)                                      │
                                                              │
Phase 5 (glamour) ─────────────────────────────────────────┤
  5.1 (import paths) → 5.2 (WithAutoStyle removal)         │
                    → 5.3 (WithColorProfile) ✅               │
                    → 5.4 (StyleConfig compat) ✅             │
                    → 5.5 (GlamourStyleConfig) ✅             │
                    → 5.6 (lipgloss dep compat) ✅             │
                                                              │
Phase 6 (testing) ────────────────────────────────────────────┘
  6.1 (compile) → 6.2 (color callers) → 6.3 (tests)
                → 6.4 (lint) → 6.5 (smoke test)
```

**Recommended order:**
1. Task 1.1 + 1.4 + 1.5 (lipgloss Color type migration — start with leaf package)
2. Task 0.1 (update all import paths including glamour v2)
3. Task 2.5 + 2.6 (KeyPressMsg + space)
4. Task 2.1 + 2.2 + 2.3 (View returns tea.View + AltScreen)
5. Task 3.2 (spinner.Tick → spinner.Tick())
6. Task 3.3 (textinput Width field → SetWidth method)
7. Task 3.5 (viewport.New + Width/Height setters)
8. Task 3.4 + 3.6 (textarea + Blink)
9. Task 3.7 (cursor model, if applicable)
10. Task 4.1 (huh import path)
11. Task 5.1 + 5.2 (glamour import paths + WithAutoStyle removal)
12. Tasks 6.1–6.5 (integration & testing)

---

## Risk Assessment

| Risk | Severity | Mitigation |
|------|----------|------------|
| Glamour v2 `WithAutoStyle()` removal | Low | Remove warm-up call; not needed in v2 |
| Bubbles v2 `spinner.Tick` becomes method call | Medium | Add `()` to all 8 call sites |
| Bubbles v2 `viewport.New` signature change | Medium | Use `viewport.New(WithWidth, WithHeight)` pattern |
| Bubbles v2 textinput/viewport `Width`/`Height` field→method | Medium | Replace all field assignments/reads with setters/getters |
| `lipgloss.Color` type removal cascades through codebase | Medium | Start with `internal/tui` (leaf package), fix bottom-up |
| `tea.View` vs `string` composition errors | Medium | Internal string composition is fine; only top-level `View()` needs wrapping |
| Huh v2 theme parameter change | Low | Code doesn't use themes on Confirm |
| Color rendering differences in v2 | Low | Smoke test visual output |
| Cursor model change (`Model.Cursor` → `Model.Cursor()`) | Low | Verify no direct cursor access; only indirect through textinput/textarea |

---

## Summary Statistics

| Category | Count |
|----------|-------|
| Files needing import path changes | 20 |
| Files with `View() string` → `View() tea.View` | 6 |
| Files with `tea.KeyMsg` → `tea.KeyPressMsg` | 8 |
| Files with `lipgloss.Color` type changes | 3 |
| Files with bubbles component usage | 7 |
| Files with `spinner.Tick` → `spinner.Tick()` | 4 |
| Files with viewport `New`/`Width`/`Height` changes | 2 |
| Files with textinput `Width` field → method | 4 |
| Files with `tea.WithAltScreen()` removal | 1 |
| Files with glamour `WithAutoStyle()` removal | 1 |
| Files with space bar `" "` → `"space"` | 1 |
| Files with `huh` import path change | 1 |
| Total files touched | ~20 |