# tira — keybinding reference

Modelled on yazi's philosophy: modal only where the modality is obvious from context, not as a first-class concept the user has to track.

> **Note:** This document reflects the actual implemented keybindings. Last updated: 2026-08-25

---

## Navigation (Backlog View)

| Key | Action |
|-----|--------|
| `j` / `k` | Move down / up within a sprint |
| `J` / `}` | Jump to next sprint header |
| `K` / `{` | Jump to previous sprint header |
| `g` / `G` | Jump to first / last ticket in the list |
| `d` / `u` | Scroll issue list 1/4 page down / up |
| `<C-d>` / `<C-u>` | Scroll sidebar 1/4 page down / up |
| `z` | Toggle collapse current sprint |
| `Z` | Toggle collapse all sprints |
| `/` | Filter tickets (fuzzy search by summary or key) |
| `f` | Jump to issue by number (e.g. type `123` to jump to `DEV-123`) |
| `L` | Pick a linked item to open in Jira (see [Linked Items](#linked-items-l)) |
| `<Enter>` | Toggle expand/collapse sprint or open ticket detail |
| `<Esc>` | Clear filter / cancel current action / clear selection |

---

## Navigation (Kanban View)

| Key | Action |
|-----|--------|
| `h` / `j` / `k` / `l` | Move left / down / up / right between columns and issues |
| `L` | Pick a linked item to open in Jira (see [Linked Items](#linked-items-l)) |
| `<Enter>` | Open ticket detail pane |
| `<Esc>` | Close detail pane / cancel action |

---

## Navigation (Epics View)

| Key | Action |
|-----|--------|
| `j` / `k` | Move down / up through epics |
| `g` / `G` | Jump to first / last epic |
| `d` / `u` | Scroll the epic list by 1/4 page |
| `<C-d>` / `<C-u>` | Scroll the selected epic sidebar |
| `/` | Filter epics (fuzzy search by name or key) |
| `<Enter>` | Open selected epic detail |
| `<Esc>` / `q` | Close epic detail / cancel action |
| `L` | Pick a linked item or child work item and open it in Jira |
| `o` | Open selected epic in Jira |
| `b` | Switch to Backlog and filter by selected epic |

---

## Editing (Epics View)

| Key | Action |
|-----|--------|
| `l` | Edit the selected epic's complete label set |
| `<Enter>` | Save labels |
| `<Esc>` | Cancel label editing |

Labels are entered as a comma-separated list. Saving an empty value clears all
labels; existing labels are replaced by the submitted list.

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
| `L` | Pick a linked item to open in Jira (works in the list and the detail pane) |
| `o` | Open ticket in browser (cursor issue) |
| `O` | Open all selected tickets in browser |
| `y` | Copy ticket URL to clipboard (cursor issue) |
| `<C-n>` | Create a new sprint — form with name, start date, duration |
| `E` | Edit the sprint under the cursor (name, start date, duration) |

Sprint form: `tab`/`shift+tab` to move between fields, `ctrl+s` to save, `esc` to cancel.

Quick pickers (`s`, `P`, `A`, `F`) open a small overlay, navigate with `j`/`k`, confirm with `<Enter>`, cancel with `<Esc>`.

---

## Editing (Kanban View)

| Key | Action |
|-----|--------|
| `e` | Edit ticket in `$EDITOR` (full template flow) |
| `s` | Change status — picker |
| `A` | Set assignee — fuzzy picker |
| `L` | Pick a linked item to open in Jira (works on the board and in the detail pane) |
| `o` | Open ticket in browser |

---

## View (Global)

| Key | Action |
|-----|--------|
| `1` | Switch to backlog view |
| `2` | Switch to kanban board view |
| `3` | Switch to epics view |
| `<Tab>` | Cycle between backlog, kanban, and epics |
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
| Filter (backlog or epics) | `/` | `<Enter>` to apply, `<Esc>` to clear |
| Key search | `f` | `<Enter>` to jump, `<Esc>` to cancel |
| Detail view | `<Enter>` on issue | `<Esc>` or `q` to close |
| Epic detail view | `<Enter>` on epic | `<Esc>` or `q` to close |
| Linked items picker | `L` on a ticket or epic (list, board, or detail) | `<Enter>` to open in browser, `<Esc>` to cancel |
| Label edit | `l` on an epic | `<Enter>` to save, `<Esc>` to cancel |
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

### Linked Items (`L`)
- Available in the backlog (ticket list and detail pane), the kanban view (board and detail
  pane), and the epics view (epic list and detail pane)
- Opens a picker of everything related to the selection:
  - Backlog: explicit issue links, subtasks, and the parent
  - Kanban: explicit issue links, subtasks, and the parent
  - Epics: explicit issue links, subtasks, and child work items (stories and tasks whose
    parent is the epic)
- Epic children are fetched from JQL (`parent = "<EPIC-KEY>"`), so they include items that
  are not on the board and items that are closed
- The backlog and kanban read items from a full issue fetch. The backlog already fetches
  one for its sidebar, so its picker fills in when that lands; the kanban board holds only
  list-view fields, so pressing `L` on the board fetches the full issue first (the footer
  shows a spinner while it waits)
- Items are listed flat as `relationship KEY` with the summary, type, and status alongside;
  deduplicated by key
- The list is filterable: type to narrow it (matching key, summary, type, or status) and use
  arrow keys to move the selection, the same way the Set Parent picker works
- `<Enter>` opens the highlighted item in the browser; related items are often not on the
  board, so the picker opens in Jira instead of moving the cursor
- The epic sidebar and detail pane list child work items under a `Child Work Items`
  heading; a failed fetch is reported inline rather than hidden

---

## Not Yet Implemented (Documented Elsewhere)

The following keybindings are planned but not yet implemented:

| Key | Planned Action |
|-----|----------------|
| `m` | Move selected ticket(s) to sprint — opens a sprint picker |
| `r` | Rename — inline edit of summary only |
| `t` | Change type — picker |
| `p` (lowercase, for priority) | Change priority — picker (currently `p` is paste) |
| `V` | Select all tickets in current sprint |
| `*` | Invert selection across all sprints |
| `n` / `N` | Next / previous filter match |
| `S` (for sort) | Cycle sort: default → priority → points → assignee |
