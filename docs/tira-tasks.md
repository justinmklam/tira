# tira — task list

## Phase 1 — scaffold + config

- [ ] Initialise Go module (`go mod init github.com/justinmklam/tira`)
- [ ] Add all dependencies to `go.mod`
- [ ] Create project directory structure
- [ ] Implement `internal/config/config.go` — load `JIRA_URL`, `JIRA_EMAIL`, `JIRA_API_TOKEN` via viper `AutomaticEnv()`
- [ ] Load optional `~/.config/tira/config.yaml` for `default_project` and `default_board_id`
- [ ] Fail fast with a clear error message if any required env var is missing
- [ ] Write `config.example.yaml`
- [ ] Set up cobra root command in `cmd/tira/root.go` with `--debug` flag
- [ ] Wire `config.Load()` into the cobra `PersistentPreRunE` so all subcommands get a config or exit early

---

## Phase 2 — data models + API client

- [ ] Define `Issue`, `IssueFields`, `ValidValues`, `Assignee`, `Sprint` structs in `internal/models/models.go`
- [ ] Define `Client` interface in `internal/api/client.go`
- [ ] Implement `NewClient(cfg *config.Config) Client` using go-jira v2
- [ ] Implement `GetIssue(key string) (*models.Issue, error)`
- [ ] Implement `GetValidValues(projectKey string)` — fetch issue types, priorities, assignees, sprints in parallel using `errgroup`
- [ ] Add in-memory cache with 10-minute TTL for `GetValidValues` results
- [ ] Implement `UpdateIssue(key string, fields models.IssueFields) error`
- [ ] Implement `CreateIssue(projectKey string, fields models.IssueFields) (*models.Issue, error)`
- [ ] Implement `GetActiveSprint(boardID int) ([]models.Issue, error)`
- [ ] Implement `GetBacklog(projectKey string) ([]models.Sprint, error)`
- [ ] Smoke test: `get MP-101` prints raw JSON to stdout

---

## Phase 3 — display

- [ ] Implement `internal/display/issue.go` — lipgloss styled issue header (key, type, status badge, priority, assignee, story points)
- [ ] Colour-code status badge by category (gray = to do, blue = in progress, green = done)
- [ ] Pipe issue description through `glamour.Render()` with `"auto"` style
- [ ] Add `bubbles/spinner` for API call loading state (write to stderr so stdout stays clean for piping)
- [ ] Implement `internal/display/list.go` — lipgloss table for sprint issues (key, summary, type, priority, assignee, points)
- [ ] Truncate summary column based on live terminal width
- [ ] Group list rows by status with a subtle header row per group

---

## Phase 4 — editor: template + parse

- [ ] Implement `editor/template.go` — `RenderTemplate(issue *models.Issue, valid *models.ValidValues) string`
- [ ] Embed valid values as comments on the line above each field
- [ ] Include `<!-- tira: do not remove this line -->` sentinel as the first line
- [ ] Use `---` separator between structured fields and free-form description
- [ ] Write `.md` temp file to `os.TempDir()` with `.md` extension
- [ ] Implement `editor/parse.go` — `ParseTemplate(content string) (*models.IssueFields, error)`
- [ ] Split on `---`, parse `key: value` lines, skip comment lines
- [ ] Strip `# KEY: Summary` heading before capturing description body
- [ ] Return error if sentinel line is missing
- [ ] Write table-driven unit tests covering: all fields present, missing optional fields, extra whitespace, mangled sentinel, special characters in description, multiline description

---

## Phase 5 — editor: open + validation loop

- [ ] Implement `editor/open.go` — resolve `$EDITOR` → `$VISUAL` → `vi` fallback
- [ ] Handle editors with args (e.g. `EDITOR="code --wait"`) via `strings.Fields`
- [ ] Connect `cmd.Stdin/Stdout/Stderr` to the terminal so the editor has full control
- [ ] Implement `validator/validate.go` — `Validate(fields, valid) []ValidationError`
- [ ] Validate type, priority, assignee with case-insensitive match
- [ ] Validate story points as non-negative number or blank
- [ ] Implement `validator/annotate.go` — `AnnotateTemplate(content string, errs []ValidationError) string`
- [ ] Replace the existing hint comment above each erroring field with an `<!-- ERROR: ... -->` comment
- [ ] Write unit tests for validation (invalid type, invalid priority, bad story points, valid input passes)
- [ ] Write unit tests for annotation (error comment inserted correctly, no comment stacking on repeated failures)

---

## Phase 6 — `get` command

- [ ] Implement `cmd/tira/get.go` with cobra subcommand
- [ ] `tira get <key>` — fetch and display issue using display package
- [ ] `tira get <key> --edit` — run the full editor loop:
  - [ ] Fetch issue + valid values (with spinner)
  - [ ] Render template to temp file
  - [ ] Open `$EDITOR`
  - [ ] Detect no-change and abort cleanly
  - [ ] Parse file, validate fields
  - [ ] On failure: annotate file, print lipgloss error summary, `huh` confirm re-open
  - [ ] On success: show lipgloss diff of changed fields, `huh` confirm save
  - [ ] Call `UpdateIssue`, print confirmation with issue key

---

## Phase 7 — `list` command

- [ ] Implement `cmd/tira/list.go` with cobra subcommand
- [ ] `tira list` — fetch active sprint, render lipgloss table
- [ ] `tira list --backlog` — fetch all sprints, render tree grouped by sprint
- [ ] Collapse closed sprints by default with `[+N more]` indicator
- [ ] Add `--project` flag to override `default_project` from config

---

## Phase 8 — `create` command

- [ ] Implement `cmd/tira/create.go` with cobra subcommand
- [ ] Start from a blank template with placeholder values
- [ ] Reuse the same editor + validation loop as `get --edit`
- [ ] Call `CreateIssue` on confirmed save, print new issue key
- [ ] Add `--project` flag to override default project
- [ ] Add `--type` flag to pre-fill issue type (skips one round-trip through the editor for common workflows)

---

## Phase 9 — polish

- [ ] Wire `charmbracelet/log` throughout, gated behind `--debug` flag
- [ ] Add `--no-color` / respect `NO_COLOR` env var for CI environments
- [ ] Handle Jira API rate limit errors with a clear user-facing message
- [ ] Handle network timeout with a configurable timeout flag (default 10s)
- [ ] Add `tira version` subcommand
- [ ] Write a `README.md` covering installation, env var setup, and command reference
