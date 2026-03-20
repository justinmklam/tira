# tira — keybinding reference

Modelled on yazi's philosophy: modal only where the modality is obvious from context, not as a first-class concept the user has to track.

> **Note:** This document reflects the actual implemented keybindings. Last updated: 2026-03-20

---

## Navigation (Backlog View)

| Key | Action |
|-----|--------|
| `j` / `k` | Move down / up within a sprint |
| `J` / `}` | Jump to next sprint header |
| `K` / `{` | Jump to previous sprint header |
| `g` / `G` | Jump to first / last ticket in the list |
| `<C-d>` / `<C-u>` | Half-page down / up |
| `z` | Toggle collapse current sprint |
| `Z` | Toggle collapse all sprints |
| `/` | Filter tickets (fuzzy search by summary or key) |
| `<Enter>` | Toggle expand/collapse sprint or open ticket detail |
| `<Esc>` | Clear filter / cancel current action / clear selection |

---

## Navigation (Kanban View)

| Key | Action |
|-----|--------|
| `h` / `j` / `k` / `l` | Move left / down / up / right between columns and issues |
| `<Enter>` | Open ticket detail pane |
| `<Esc>` | Close detail pane / cancel action |

---

## Selection (Backlog View)

| Key | Action |
|-----|--------|
| `<Space>` | Toggle select ticket under cursor |
| `v` | Enter visual mode — extend selection with `j`/`k`, confirm with `<Enter>` |
| `<Esc>` | Clear all selections (when not in visual mode) |

---

## Moving Tickets (Backlog View)

| Key | Action |
|-----|--------|
| `<C-j>` / `<C-k>` | Move ticket one position down / up within its sprint |
| `>` / `<` | Move ticket to next / previous sprint directly |
| `B` | Move ticket to backlog (no sprint) |
| `x` | Cut selected ticket(s) for move |
| `p` | Paste cut ticket(s) to current sprint |

---

## Editing (Backlog View)

| Key | Action |
|-----|--------|
| `e` | Edit ticket in `$EDITOR` (full template flow) |
| `S` | Set story points — inline numeric input |
| `s` | Change status — picker |
| `a` | Create new ticket in current sprint |
| `C` | Create new ticket in backlog |
| `P` | Set parent — fuzzy picker (works on selection or cursor ticket) |
| `A` | Set assignee — fuzzy picker (works on selection or cursor ticket) |
| `F` | Filter by epic — fuzzy picker |
| `<Enter>` | Open ticket detail pane (press `e` from there to edit) |
| `o` | Open ticket in browser (cursor issue) |
| `O` | Open all selected tickets in browser |
| `y` | Copy ticket URL to clipboard (cursor issue) |

Quick pickers (`s`, `P`, `A`, `F`) open a small overlay, navigate with `j`/`k`, confirm with `<Enter>`, cancel with `<Esc>`.

---

## Editing (Kanban View)

| Key | Action |
|-----|--------|
| `e` | Edit ticket in `$EDITOR` (full template flow) |
| `s` | Change status — picker |
| `A` | Set assignee — fuzzy picker |
| `o` | Open ticket in browser |

---

## View (Global)

| Key | Action |
|-----|--------|
| `1` | Switch to backlog view |
| `2` | Switch to kanban board view |
| `<Tab>` | Toggle between backlog and board |
| `R` | Refresh from Jira API |
| `?` | Show keybindings help overlay |
| `q` | Quit |

---

## Help Overlay

When the help overlay is open (`?`), use these keys to navigate:

| Key | Action |
|-----|--------|
| `j` / `k` | Scroll down / up one line |
| `<C-d>` / `<C-u>` | Scroll half-page down / up |
| `g` / `G` | Jump to top / bottom |
| `?` / `<Esc>` | Close help overlay |

---

## Modes

| Mode | Enter | Exit |
|------|-------|------|
| Normal | — | — |
| Visual | `v` | `<Enter>` to confirm, `<Esc>` to cancel |
| Filter | `/` | `<Enter>` to apply, `<Esc>` to clear |
| Detail view | `<Enter>` on issue | `<Esc>` or `q` to close |
| Pickers (`s`, `P`, `A`, `F`) | Key press | `<Enter>` to select, `<Esc>` to cancel |

---

## Implementation Notes

### Cut and Paste (`x` then `p`)
1. Select ticket(s) with `Space` or visual mode (`v`)
2. Press `x` to cut (marks tickets for move)
3. Navigate to target sprint
4. Press `p` to paste — tickets are moved to the target sprint

### Visual Mode (`v`)
1. Press `v` to start visual selection at current cursor position
2. Use `j`/`k` to extend selection
3. Press `<Enter>` to confirm selection (toggles with existing selection)
4. Selected tickets shown with highlighted background

### Story Points (`S`)
- Works on single issue or multiple selected issues
- Enter numeric value (e.g., `1`, `2`, `3`, `5`, `8`)
- Empty value clears story points

### Epic Filter (`F`)
- Filters visible tickets to show only those belonging to selected epic
- Select `(none)` to clear filter

---

## Not Yet Implemented (Documented Elsewhere)

The following keybindings are planned but not yet implemented:

| Key | Planned Action |
|-----|----------------|
| `m` | Move selected ticket(s) to sprint — opens a sprint picker |
| `r` | Rename — inline edit of summary only |
| `t` | Change type — picker |
| `p` (lowercase, for priority) | Change priority — picker (currently `p` is paste) |
| `l` | Edit labels — inline comma-separated input |
| `V` | Select all tickets in current sprint |
| `*` | Invert selection across all sprints |
| `n` / `N` | Next / previous filter match |
| `f` | Cycle filter presets: all → mine → unassigned |
| `S` (for sort) | Cycle sort: default → priority → points → assignee |
