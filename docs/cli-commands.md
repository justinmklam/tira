# CLI Commands

tira provides the following commands:

| Command | Description |
|---------|-------------|
| `get <key\|url> [--edit]` | Fetch and display a single issue (accepts an issue key or a full browse URL); `--edit` is a **deprecated** alias for `update <key> --edit`-style interactive editing |
| `update <key\|url> [--show] [--no-edit] [--file <path>]` | Update an existing issue; non-interactively (agents) via `--show`/`--no-edit`/`--file`, or interactively via `$EDITOR` |
| `create [--project <key>] [--type <type>] [--parent <key>]` | Create a new issue via `$EDITOR` |
| `board [--view backlog\|kanban] [--board-id <id>]` | Launch the unified TUI (backlog + kanban views) |
| `backlog` | **Deprecated** — alias for `board --view backlog` |
| `kanban` | **Deprecated** — alias for `board --view kanban` |
| `version` | Print the tira version (also available as `tira --version`) |

All commands use the `--profile` flag to select a config profile (default: `"default"`), and the
`--debug`/`--debug-file <path>` flags to enable debug logging (see [Debug Logging](#debug-logging)
below).

> **Deprecation notice:** `get --edit`, `backlog`, `kanban`, and `update --template` are deprecated
> aliases kept for backward compatibility. They still work today, print a warning on stderr, and
> are planned for removal in a future major version. Prefer `update <key>`, `board --view backlog`,
> `board --view kanban`, and `update <key> --show`, respectively. See
> [command-restructure-proposal.md](command-restructure-proposal.md) for the full rationale and
> migration timeline.

---

## `tira get <key|url> [--edit]`

**File:** `cmd/tira/get.go`

### Key resolution

The argument may be a bare issue key (e.g. `PROJ-123`) or a full Jira browse
URL (e.g. `https://example.atlassian.net/browse/PROJ-123`). `extractIssueKey`
pulls the `KEY-123`-shaped key out of either form via regex (case-insensitive,
whitespace-trimmed) before calling the API — this works for any project, not
just the one configured in `project`/`board_id`.

### Without `--edit` (View Mode)

Displays a single issue in a terminal pager:

1. Creates API client from config
2. Fetches issue with `tui.RunWithSpinner` (shows spinner during fetch)
3. Renders to Markdown via `display.RenderIssue`
4. Pages the output:
   - Tries `glow --pager --style=dracula --width=120 -` first
   - Falls back to `less -R`
   - Falls back to stdout
5. If stdout is not a TTY (piped), writes raw Markdown directly

**Example:**
```bash
./tira get MP-101
./tira get MP-101 | grep "Status"  # pipe to another command
./tira get https://example.atlassian.net/browse/MP-101  # full URL also works
```

### With `--edit` (Edit Mode) — deprecated

**Deprecated:** use `tira update <KEY>` instead (identical behavior, clearer name). `--edit` is
kept as a backward-compatible alias and prints a deprecation warning on stderr, but still works.

**Example:**
```bash
./tira get MP-101 --edit   # deprecated; prefer: ./tira update MP-101
```

---

## `tira update <key|url> [--show] [--no-edit] [--file <path>]`

**File:** `cmd/tira/update.go`

Updates an existing issue. Only non-empty fields/sections in the parsed template are sent to the
API (`client.UpdateIssue` skips empty fields) — omitted fields are left unchanged on the ticket.

### Non-interactive mode (recommended for AI agents)

1. `tira update <KEY> --show` fetches the issue and valid values, renders the full
   `editor.RenderTemplate` output (identical to what `$EDITOR` would show), and prints it to
   stdout — no mutation happens. Agents should always run this first to capture current values
   before editing, rather than guessing or reconstructing the template from `tira get` output.
2. The caller edits only the fields/sections that need to change.
3. `tira update <KEY> --no-edit` (or piping via stdin/`--file` without `--no-edit`, or non-terminal
   stdin, which is auto-detected the same way `create` detects it) reads the edited template via
   `readInput`, parses it with `editor.ParseTemplate`, validates via `validator.Validate`
   (returning validation errors without opening an editor if invalid), resolves the assignee ID,
   diffs against the current issue (`printFieldDiff`), and calls `client.UpdateIssue`.

**Example:**
```bash
./tira update MP-101 --show > /tmp/mp-101.md
# edit /tmp/mp-101.md
cat /tmp/mp-101.md | ./tira update MP-101 --no-edit
```

> **Note:** `--template` is a deprecated alias for `--show` (kept for backward compatibility;
> prints a deprecation warning). Use `--show`.

### Interactive mode

With no `--template`/`--no-edit`/`--file` and a terminal stdin, falls back to the same
`runEditLoop` used by `tira get --edit`: renders the template to a temp file, opens `$EDITOR`,
validates (looping on validation errors with `AnnotateTemplate`), and updates via the API.

```bash
./tira update MP-101
```

---

## `tira create [--project <key>] [--type <type>] [--parent <key>] [--file <path>] [--no-edit]`

**File:** `cmd/tira/create.go`

Creates a new issue in either **interactive mode** (via `$EDITOR`) or **non-interactive mode** (via file/stdin).

### Interactive Mode (default)

When no `--file` or `--no-edit` flag is provided and stdin is a terminal:

1. Resolves project key from `--project` flag or `cfg.Project`
2. Fetches valid values (with spinner)
3. Validates `--type` early if provided
4. Builds a blank `*models.Issue` pre-filled with `IssueType` and `ParentKey`
5. Pre-fills defaults:
   - `IssueType`: first valid type from the list
   - `Priority`: middle value from priorities list
6. Calls `openAndValidate` (same loop as edit)
7. Validates that Summary is non-empty and not the placeholder text
8. Calls `client.CreateIssue`

**Flags:**
- `--project <key>` — Project key (overrides config default)
- `--type <type>` — Issue type (e.g., `Bug`, `Story`, `Task`)
- `--parent <key>` — Parent issue key (for sub-tasks)

**Example:**
```bash
# Interactive create with defaults
./tira create

# Create in specific project
./tira create --project DEV

# Create a sub-task under a parent
./tira create --type Sub-task --parent MP-100
```

### Non-Interactive Mode (for AI agents / automation)

When `--file`, `--no-edit`, or piped stdin is used:

1. Reads template content from file (`--file`) or stdin (`--no-edit` or pipe)
2. Parses the template format (YAML-like front matter + Markdown body)
3. Validates all fields (type, priority, required summary)
4. Resolves assignee display name to account ID if provided
5. Calls `client.CreateIssue`

**Flags:**
- `--file <path>`, `-f` — Read issue template from a file
- `--no-edit` — Read issue template from stdin (equivalent to piping)

**Template Format:**
```markdown
<!-- tira: do not remove this line or change field names -->
<!-- Valid types: Bug, Story, Task -->
type: Task
<!-- Valid priorities: Low, Medium, High -->
priority: High
assignee: John Doe
<!-- Enter a number or leave blank -->
story_points: 3
<!-- Comma-separated, e.g. backend, auth -->
labels: backend, api

---

# Summary goes here

## Description

Issue description in Markdown.

## Acceptance Criteria

- [ ] Criterion 1
- [ ] Criterion 2
```

**Examples:**
```bash
# Create from a file
./tira create --file issue-template.md

# Pipe from stdin (e.g., from an AI agent)
echo -e "type: Task\npriority: High\n---\n# My Summary\n\n## Description\n\nDo the thing" | ./tira create --no-edit

# Generate with AI and create in one command
ai-generate-issue-prompt "Fix the login bug" | ./tira create --no-edit

# Use a heredoc for rich content
./tira create --no-edit << 'EOF'
type: Story
priority: High
assignee: Jane Smith
labels: frontend, auth

---

# Implement OAuth2 Login

## Description

Add OAuth2 login flow with Google provider.

## Acceptance Criteria

- [ ] User can sign in with Google
- [ ] Session is persisted correctly
- [ ] Logout clears the session
EOF
```

**Notes:**
- Validation is still performed in non-interactive mode (valid types, priorities, required fields)
- If `type` or `priority` is omitted, defaults are applied (first type, middle priority)
- Assignee is resolved by display name (case-insensitive match)
- Errors are returned with clear messages for invalid templates

### Template Format (for AI Agents)

To get the complete template format specification, run:

```bash
./tira create --template
```

This outputs detailed documentation including:
- All front matter fields and their descriptions
- Markdown body structure
- Minimal and full examples
- Validation rules

AI agents can use this to generate properly formatted issue templates programmatically.

---

## `tira board` / `tira backlog` / `tira kanban`

**File:** `cmd/tira/board.go`

All three commands launch the same unified TUI. `board` is the canonical command; `backlog` and
`kanban` are **deprecated** aliases kept for backward compatibility (print a deprecation warning,
still fully functional):
- `board [--view backlog|kanban]` — starts in the given view (`backlog` if `--view` omitted)
- `backlog` — deprecated alias for `board --view backlog`
- `kanban` — deprecated alias for `board --view kanban`

All three also accept `--board-id <id>` to override the `board_id` configured for the active
profile without editing the config file.

### Execution Flow

All three commands call `runBoardCmd(startView)` which:

1. Resolves the board ID: `--board-id` flag if set, else `cfg.BoardID` (fatal if both are missing)
2. Creates API client from config
3. Calls `fetchBoardData` with spinner — fetches sprint groups + board columns **concurrently**
4. Calls `runBoardTUI` — starts the `tea.Program` with `tea.WithAltScreen()`

### Board Data Fetch (Progressive Loading)

The initial fetch is split into two phases for fast time-to-first-render:

**Phase 1 (blocking, with spinner):**
- `client.GetSprintList(boardID)` + `client.GetBoardColumns(boardID)` — run in parallel
- `client.GetSprintGroupsBatch(boardID, first3Sprints)` — fetches issues for the first 3 sprints only
- TUI renders immediately with partial data

**Phase 2 (background, after TUI renders):**
- `client.GetSprintGroupsBatch(boardID, remainingSprints)` — remaining sprint issues
- `client.GetBacklogIssues(boardID)` — backlog issues
- Results are appended seamlessly via `blLazyLoadDoneMsg`

Manual refresh (`R`) fetches everything at once via `GetSprintGroups`.

**Example:**
```bash
# Start in backlog view (default)
./tira board

# Start in kanban view
./tira board --view kanban

# Override the configured board ID
./tira board --board-id 42

# Deprecated aliases (still work, print a warning)
./tira backlog
./tira kanban

# Use specific profile
./tira --profile dev board
```

### View Switching

Once the TUI is running:
- `Tab` — toggle between backlog and kanban
- `1` — switch to backlog
- `2` — switch to kanban
- `q` or `Ctrl+C` — quit

---

## Common Patterns

### Spinner Usage

All blocking operations use `tui.RunWithSpinner[T]` to show a loading indicator:

```go
issue, err := tui.RunWithSpinner("Fetching issue...", func() (*models.Issue, error) {
    return client.GetIssue(key)
})
```

### Error Handling

- Network errors are returned with context (e.g., "failed to fetch issue MP-101: ...")
- Validation errors in edit/create show inline comments in the editor
- Missing required config fields cause immediate fatal error

### Editor Integration

Both `get --edit` and `create` use the same editor flow:
- Template format: YAML-like front matter + Markdown body, separated by `---`
- Sentinel comment `<!-- tira: ... -->` detects template corruption
- Validation errors are annotated inline with `<!-- ERROR: ... -->` comments
- Editor resolution: `$EDITOR` → `$VISUAL` → `vi`

See [Editor Flow](editor-flow.md) for template format details.

---

## `tira version` / `tira --version`

**Files:** `cmd/tira/version.go`, `cmd/tira/main.go`

Prints the tira version and exits. Does not require config (`~/.config/tira/config.yaml`), so it
works even before `tira` is configured — useful as a first sanity check for agents.

```bash
./tira version
./tira --version   # equivalent, handled by cobra before config/profile resolution
```

The version string is injected at build time via `-X main.version=...` (see `.goreleaser.yml`);
locally-built binaries report `dev`.

---

## Debug Logging

Pass `--debug` (or `--debug-file <path>`) to any command to enable verbose debug logging:

```bash
./tira --debug get MP-101
./tira --debug-file /tmp/tira-debug.log update MP-101 --show
```

- `--debug` enables logging to the default location and prints the resolved path to stderr on
  startup (`Debug logging to <path>`).
- `--debug-file <path>` enables logging to a specific path (implies `--debug`).
- Default location: `$XDG_STATE_HOME/tira/debug.log`, falling back to
  `~/.local/state/tira/debug.log` if `$XDG_STATE_HOME` is unset. (Prior versions wrote
  `debug.log` to the current working directory — this no longer happens by default.)
- See `internal/debug/logger.go` for `Init(path)`, `DefaultLogPath()`, and `LogPath()`.

---

## See Also

- [Configuration](configuration.md) — Config file format and profiles
- [TUI Architecture](tui-architecture.md) — Board TUI internals
- [Keybindings](keybindings-backlog.md) — TUI keybinding reference
