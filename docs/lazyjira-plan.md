# lazyjira — implementation plan

A lazygit-style CLI for Jira, built in Go with Charm tooling. Designed to be extended into a full TUI later — the CLI is the foundation, not a throwaway.

---

## Project layout

```
lazyjira/
├── cmd/lazyjira/
│   ├── main.go
│   ├── root.go        # cobra root, persistent flags
│   ├── get.go         # lazyjira get <key> [--edit]
│   ├── list.go        # lazyjira list [--backlog]
│   └── create.go      # lazyjira create [--project] [--type]
├── internal/
│   ├── config/
│   │   └── config.go  # viper load, env var auth
│   ├── models/
│   │   └── models.go  # Issue, Sprint, ValidValues
│   ├── api/
│   │   ├── client.go  # go-jira wrapper, interface
│   │   └── meta.go    # fetch valid field values
│   ├── display/
│   │   ├── issue.go   # lipgloss ticket card renderer
│   │   └── list.go    # lipgloss sprint/backlog table
│   ├── editor/
│   │   ├── template.go  # render issue → markdown
│   │   ├── parse.go     # parse markdown → IssueFields
│   │   └── open.go      # open $EDITOR, wait, return path
│   └── validator/
│       ├── validate.go  # validate fields against ValidValues
│       └── annotate.go  # rewrite file with error comments
├── go.mod
└── config.example.yaml
```

---

## Dependencies

```go
require (
    github.com/andygrunwald/go-jira/v2          // Jira REST client
    github.com/spf13/cobra                       // CLI framework
    github.com/spf13/viper                       // config file

    // Charm
    github.com/charmbracelet/lipgloss            // terminal styling
    github.com/charmbracelet/glamour             // markdown → terminal render
    github.com/charmbracelet/huh                 // select/confirm prompts
    github.com/charmbracelet/log                 // structured logging
    github.com/charmbracelet/bubbles/spinner     // loading spinner
)
```

---

## Auth

API credentials are read from environment variables — no keychain, no interactive prompt, no first-run setup command.

```bash
export JIRA_URL=https://yourorg.atlassian.net
export JIRA_EMAIL=you@example.com
export JIRA_API_TOKEN=your_token_here
```

`internal/config/config.go` reads these via viper's `AutomaticEnv()`. If any are missing, the CLI exits immediately with a clear error message pointing to the missing variable. This keeps auth completely stateless and CI-friendly.

```go
// config.go
type Config struct {
    JiraURL   string
    Email     string
    Token     string
    Project   string  // default project key, from config file
    BoardID   int     // default board ID, from config file
}

func Load() (*Config, error) {
    viper.AutomaticEnv()
    viper.SetEnvPrefix("")  // read JIRA_URL, JIRA_EMAIL, JIRA_API_TOKEN directly

    cfg := &Config{
        JiraURL: viper.GetString("JIRA_URL"),
        Email:   viper.GetString("JIRA_EMAIL"),
        Token:   viper.GetString("JIRA_API_TOKEN"),
    }

    if cfg.JiraURL == "" || cfg.Email == "" || cfg.Token == "" {
        return nil, fmt.Errorf(
            "missing required env vars: JIRA_URL, JIRA_EMAIL, JIRA_API_TOKEN",
        )
    }

    // optional: load default_project + default_board_id from ~/.config/lazyjira/config.yaml
    viper.SetConfigName("config")
    viper.AddConfigPath("$HOME/.config/lazyjira")
    _ = viper.ReadInConfig()  // not required — ignore error if absent
    cfg.Project = viper.GetString("default_project")
    cfg.BoardID = viper.GetInt("default_board_id")

    return cfg, nil
}
```

`config.example.yaml` (optional, for non-sensitive defaults only):

```yaml
default_project: MYPROJ
default_board_id: 42
```

---

## Data models

```go
// internal/models/models.go

type Issue struct {
    Key         string
    Summary     string
    Description string
    Status      string
    IssueType   string
    Priority    string
    Assignee    string   // display name
    AssigneeID  string   // accountId for API writes
    StoryPoints float64
    Labels      []string
    SprintID    int
    SprintName  string
}

// IssueFields is what we read back from the editor and send to the API
type IssueFields struct {
    Summary     string
    IssueType   string
    Priority    string
    AssigneeID  string
    StoryPoints float64
    Labels      []string
    Description string
}

// ValidValues holds allowed values fetched from the Jira project metadata
type ValidValues struct {
    IssueTypes []string
    Priorities []string
    Assignees  []Assignee
    Sprints    []Sprint
}

type Assignee struct {
    DisplayName string
    AccountID   string
}

type Sprint struct {
    ID    int
    Name  string
    State string // active | future | closed
}
```

---

## API client

A thin interface over go-jira. Defined as an interface from the start so it's easy to mock in tests.

```go
// internal/api/client.go

type Client interface {
    GetIssue(key string) (*models.Issue, error)
    UpdateIssue(key string, fields models.IssueFields) error
    CreateIssue(projectKey string, fields models.IssueFields) (*models.Issue, error)
    GetValidValues(projectKey string) (*models.ValidValues, error)
    GetActiveSprint(boardID int) ([]models.Issue, error)
    GetBacklog(projectKey string) ([]models.Sprint, error)
}
```

`internal/api/meta.go` — `GetValidValues` calls three Jira endpoints in parallel using `errgroup`: `/rest/api/3/project/{key}/statuses` for issue types, `/rest/api/3/priority` for priorities, and `/rest/agile/1.0/board/{id}/sprint` for sprints. Assignees come from `/rest/api/3/user/assignable/search`. Cache the result in memory with a 10-minute TTL — this data rarely changes within a session.

---

## Commands

```bash
lazyjira get MP-101              # pretty-print ticket to stdout
lazyjira get MP-101 --edit       # open in $EDITOR, save writes back to Jira

lazyjira list                    # active sprint issues
lazyjira list --backlog          # all sprints, grouped

lazyjira create                  # new ticket using $EDITOR template
lazyjira create --project OTHER  # override project
lazyjira create --type Bug       # pre-fill type field
```

---

## The `get --edit` flow

This is the core of the tool. The loop:

1. Fetch issue + valid field values from Jira API (with spinner)
2. Render a markdown template to a temp `.md` file (valid values embedded as comments)
3. Open `$EDITOR` and block until it exits
4. If no changes, abort cleanly
5. Parse the file back into `IssueFields`
6. Validate each field against the fetched valid values
7. If invalid: annotate the file with inline error comments, print a summary, confirm re-open
8. If valid: show a lipgloss diff of changed fields, confirm with `huh`, save to Jira

### Template format

```markdown
<!-- lazyjira: do not remove this line or change field names -->
<!-- Valid types: Bug, Story, Task, Epic, Subtask -->
type: Bug

<!-- Valid priorities: Highest, High, Medium, Low, Lowest -->
priority: Medium

<!-- Valid assignees: alice (alice.smith), bob (bob.jones) -->
assignee: alice

<!-- Enter a number or leave blank -->
story_points: 3

<!-- Comma-separated, e.g. backend, auth -->
labels: frontend, nav

---

# MP-101: Fix navigation bug

Fix the broken back-button behaviour on the settings page.
The issue occurs when...
```

Key decisions in this format: fields come first so parse errors are caught before the user reads a long description; comments sit on the line above each field so they're visible when the cursor is on the field in most editors; `---` cleanly separates structured fields from free-form description; the `<!-- lazyjira: ... -->` sentinel lets you detect if the user accidentally deletes the header.

### Parsing

`editor/parse.go` — split on `---`, parse lines above as `key: value` pairs (skip `<!-- ... -->` lines), treat everything below `---` after the `# KEY: Summary` heading as the description. Return an error if the sentinel line is missing.

### Opening `$EDITOR`

```go
// editor/open.go
func OpenEditor(path string) error {
    editor := os.Getenv("EDITOR")
    if editor == "" {
        editor = os.Getenv("VISUAL")
    }
    if editor == "" {
        editor = "vi"
    }
    // support editors with args, e.g. EDITOR="code --wait"
    parts := strings.Fields(editor)
    cmd := exec.Command(parts[0], append(parts[1:], path)...)
    cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
    return cmd.Run()
}
```

The temp file is written with a `.md` extension so editors apply markdown syntax highlighting automatically.

### Validation

```go
// validator/validate.go
type ValidationError struct {
    Field   string
    Value   string
    Message string  // e.g. `"foobar" is not valid. Choose: Bug, Story, Task`
}

func Validate(fields *models.IssueFields, valid *models.ValidValues) []ValidationError
```

Enumerable fields (type, priority, assignee) are validated with a case-insensitive match. Story points must be a non-negative number or blank. Labels and description are always valid.

### Error annotation

`validator/annotate.go` — `AnnotateTemplate(content string, errs []ValidationError) string` rewrites the file, inserting an error comment directly above the offending field, replacing the existing hint comment:

```markdown
<!-- ERROR: "foobar" is not a valid type. Valid: Bug, Story, Task, Epic, Subtask -->
type: foobar
```

This prevents stacked comment lines accumulating across multiple failed saves.

### The loop in `cmd/lazyjira/get.go`

```go
tmpFile := writeTempFile(renderTemplate(issue, valid))
defer os.Remove(tmpFile)
original := readFile(tmpFile)

for {
    if err := editor.Open(tmpFile); err != nil {
        return err
    }

    content := readFile(tmpFile)
    if content == original {
        fmt.Println("No changes. Aborting.")
        return nil
    }

    fields, err := editor.Parse(content)
    if err != nil {
        return fmt.Errorf("could not parse file: %w", err)
    }

    errs := validator.Validate(fields, valid)
    if len(errs) == 0 {
        break
    }

    writeFile(tmpFile, validator.Annotate(content, errs))
    printValidationSummary(errs)  // lipgloss error list

    var retry bool
    huh.NewConfirm().
        Title("Validation failed. Re-open editor?").
        Value(&retry).Run()
    if !retry {
        return nil
    }
}

// show diff + confirm before writing
showDiff(issue, fields)
var confirm bool
huh.NewConfirm().Title("Save changes to Jira?").Value(&confirm).Run()
if confirm {
    return client.UpdateIssue(issue.Key, fields)
}
```

---

## `list` command

`lazyjira list` fetches the active sprint and renders a lipgloss table. Columns: key, summary (truncated to terminal width), type badge, priority, assignee, story points. Issues are grouped by status with a subtle header row per group.

`lazyjira list --backlog` renders all sprints as a tree: sprint name as a bold section header, issues indented beneath. Closed sprints are collapsed by default with a `[+N more]` indicator.

Both commands truncate the summary column based on live terminal width via `lipgloss.Width()`.

---

## `create` command

Nearly identical to `get --edit` but starts from an empty template with placeholder values. On successful save, calls `CreateIssue` and prints the new issue key.

```bash
lazyjira create                    # uses default_project from config
lazyjira create --project OTHERP   # override project
lazyjira create --type Bug         # pre-fill type, saves one edit cycle
```

---

## Charm tool usage

| Tool | Used for |
|---|---|
| `lipgloss` | Issue header badge, status colours, diff display, error summaries, list tables |
| `glamour` | Rendering issue description markdown in `get` |
| `huh` | Re-open editor confirm, final save confirm |
| `bubbles/spinner` | API call loading state in `get` and `list` |
| `log` | Debug logging (hidden unless `--debug` flag) |

---

## Build order

Each step is independently runnable or testable before moving to the next.

1. `config` package — prove env var loading works, fail fast with a clear message
2. `api` package — prove API connectivity, `GetIssue` prints raw JSON
3. `display/issue.go` — `get MP-101` prints a styled card to stdout
4. `editor/template.go` + `editor/parse.go` — round-trip a template as pure string I/O (no editor, no API — easy to unit test with table-driven cases)
5. `editor/open.go` — wire in `$EDITOR`
6. `validator/validate.go` + `validator/annotate.go`
7. Full `get --edit` loop end-to-end
8. `list` + `list --backlog`
9. `create`

The template round-trip in step 4 is the most valuable thing to test thoroughly — it's pure `string → struct → string` with no I/O, so you can write table-driven tests covering all edge cases (missing fields, extra whitespace, mangled sentinel line, special characters in description) before touching the editor integration.
