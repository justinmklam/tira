# CLI Commands

tira provides the following commands:

| Command | Description |
|---------|-------------|
| `get <key> [--edit]` | Fetch and display a single issue; optionally edit it |
| `create [--project <key>] [--type <type>] [--parent <key>]` | Create a new issue via `$EDITOR` |
| `board` | Launch the unified TUI (backlog + kanban views) |
| `backlog` | Launch the TUI starting in backlog view |
| `kanban` | Launch the TUI starting in kanban view |

All commands use the `--profile` flag to select a config profile (default: `"default"`).

---

## `tira get <key> [--edit]`

**File:** `cmd/tira/get.go`

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
```

### With `--edit` (Edit Mode)

Edits an existing issue via `$EDITOR`:

1. Fetches issue (with spinner)
2. Fetches valid values (issue types, priorities, assignees) with spinner; gracefully degrades on error
3. Derives `projectKey` from issue key (e.g., `"MP-101"` → `"MP"`)
4. Calls `runEditLoop`:
   - Renders template to temp file
   - Opens `$EDITOR`
   - Validates edited template
   - Updates via API
5. Prints a field diff to stderr before updating

**Edit loop** (`openAndValidate`):
```
WriteTempFile → OpenEditor → ReadFile
→ compare to original (abort if no changes)
→ ParseTemplate → Validate
→ if errors: AnnotateTemplate → WriteFile → ask to retry → loop
→ if valid: ResolveAssigneeID → return fields → UpdateIssue
```

**Example:**
```bash
./tira get MP-101 --edit
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

All three commands launch the same unified TUI. The only difference is the starting view:
- `board` — starts in backlog view (default)
- `backlog` — starts in backlog view
- `kanban` — starts in kanban view

### Execution Flow

All three commands call `runBoardCmd(startView)` which:

1. Checks `cfg.BoardID != 0` (fatal if missing)
2. Creates API client from config
3. Calls `fetchBoardData` with spinner — fetches sprint groups + board columns **concurrently**
4. Calls `runBoardTUI` — starts the `tea.Program` with `tea.WithAltScreen()`

### Board Data Fetch

Two goroutines run in parallel:
- `client.GetSprintGroups(boardID)` — fetches all active/future sprints + backlog
- `client.GetBoardColumns(boardID)` — fetches board column configuration

**Example:**
```bash
# Start in backlog view (default)
./tira board

# Start in kanban view
./tira kanban

# Use specific profile
./tira --profile dev backlog
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

## See Also

- [Configuration](configuration.md) — Config file format and profiles
- [TUI Architecture](tui-architecture.md) — Board TUI internals
- [Keybindings](keybindings-backlog.md) — TUI keybinding reference
