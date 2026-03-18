# lazyjira — keybinding reference

Modelled on yazi's philosophy: modal only where the modality is obvious from context, not as a first-class concept the user has to track.

---

## Navigation

| Key | Action |
|-----|--------|
| `j` / `k` | Move down / up within a sprint |
| `J` / `K` | Jump to next / previous sprint header |
| `g` / `G` | Jump to first / last ticket in the list |
| `{` / `}` | Previous / next sprint |
| `<C-d>` / `<C-u>` | Half-page down / up |
| `z` | Toggle collapse current sprint |
| `Z` | Toggle collapse all sprints |
| `/` | Filter tickets (fuzzy search by summary or key) |
| `n` / `N` | Next / previous filter match |
| `<Esc>` | Clear filter / cancel current action |

---

## Selection

| Key | Action |
|-----|--------|
| `<Space>` | Toggle select ticket under cursor |
| `v` | Enter visual mode — extend selection with `j`/`k`, confirm with `<Enter>` |
| `V` | Select all tickets in current sprint |
| `*` | Invert selection across all sprints |
| `<Esc>` | Clear all selections |

---

## Moving tickets

| Key | Action |
|-----|--------|
| `m` | Move selected ticket(s) to sprint — opens a sprint picker |
| `<C-j>` / `<C-k>` | Move ticket one position down / up within its sprint |
| `>` / `<` | Move ticket to next / previous sprint directly |
| `B` | Move ticket to backlog (no sprint) |

---

## Editing

| Key | Action |
|-----|--------|
| `e` | Edit ticket in `$EDITOR` (full template flow) |
| `r` | Rename — inline edit of summary only, no editor |
| `t` | Change type — picker |
| `p` | Change priority — picker |
| `a` | Change assignee — picker |
| `s` | Set story points — inline numeric input |
| `l` | Edit labels — inline comma-separated input |
| `c` | Create new ticket in current sprint |
| `C` | Create new ticket in backlog |
| `x` | Delete ticket — requires `y` confirmation |
| `<Enter>` | Open ticket detail pane (press `e` from there to edit) |

Quick pickers (`t`, `p`, `a`) open a small overlay, navigate with `j`/`k`, confirm with `<Enter>`, cancel with `<Esc>`.

---

## View

| Key | Action |
|-----|--------|
| `1` | Switch to backlog view |
| `2` | Switch to kanban board view |
| `<Tab>` | Toggle between backlog and board |
| `f` | Cycle filter presets: all → mine → unassigned |
| `S` | Cycle sort: default → priority → points → assignee |
| `R` | Refresh from Jira API |
| `?` | Show keybindings help overlay |
| `q` | Quit |

---

## Modes

| Mode | Enter | Exit |
|------|-------|------|
| Normal | — | — |
| Visual | `v` | `<Esc>` |
| Inline edit | `r` or `s` | `<Enter>` to save, `<Esc>` to cancel |
| Filter | `/` | `<Esc>` to clear |

---

## Inline rename (`r`)

Pressing `r` replaces the ticket row with an editable input in place. `<Enter>` saves, `<Esc>` cancels — no editor required for summary-only changes.

```
▼ Sprint 42 (active)
    MP-101  [Fix navigation bug in settings page_      ]   Bug  ● Alice  3pt
    MP-105  Auth refactor                               Task ● Alice  5pt
```
