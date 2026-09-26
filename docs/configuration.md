# Configuration System

**File:** `internal/config/config.go`

## Config Structure

```go
type Config struct {
    JiraURL        string `mapstructure:"jira_url"`
    Email          string `mapstructure:"email"`
    Token          string `mapstructure:"token"`
    Project        string `mapstructure:"project"`
    BoardID        int    `mapstructure:"board_id"`
    ClassicProject bool   `mapstructure:"classic_project"`
    Theme          string `mapstructure:"theme"`
}
```

## Config File Location

The configuration file is loaded from:
- `~/.config/tira/config.yaml` (primary location)
- `./config.yaml` (current directory, for testing)

Config is organized as named profiles under the `profiles:` key.

## Config File Format

Example `config.example.yaml`:

```yaml
profiles:
  default:
    jira_url: https://your-domain.atlassian.net
    email: your-email@example.com
    token: your-api-token
    project: MYPROJ
    board_id: 42
    classic_project: true   # for company-managed (classic) projects
    theme: catppuccin       # color theme (default, tokyonight, catppuccin)
  dev:
    jira_url: https://dev-domain.atlassian.net
    email: dev-email@example.com
    token: dev-api-token
    project: DEVPROJ
    board_id: 43
```

## Required vs Optional Fields

**Required fields** — missing any causes a fatal error at startup:
- `jira_url` — Your Jira Cloud instance URL (e.g., `https://yourorg.atlassian.net`)
- `email` — Your Jira Cloud email address
- `token` — Your Jira API token (generate from https://id.atlassian.com/manage-profile/security/api-tokens). Can also be set via `JIRA_TOKEN` or `JIRA_API_TOKEN` environment variables (see below)

**Optional fields:**
- `project` — Default project key (e.g., `MYPROJ`)
- `board_id` — Default board ID for the `board`/`backlog`/`kanban` commands
- `classic_project` — Set to `true` for company-managed (classic) projects; affects browser URL construction only
- `theme` — Color theme for the TUI. Available themes: `default`, `tokyonight`, `catppuccin`. If omitted, uses terminal's default ANSI 256 colors

## Environment Variables

All configuration fields can be overridden via environment variables with the `TIRA_` prefix. Additionally, the API token supports `JIRA_TOKEN` and `JIRA_API_TOKEN` for compatibility with other tools. Environment variables take precedence over values in the config file.

| Config Field | Environment Variable |
|--------------|---------------------|
| `jira_url` | `TIRA_JIRA_URL` |
| `email` | `TIRA_EMAIL` |
| `token` | `TIRA_TOKEN`, `JIRA_TOKEN`, `JIRA_API_TOKEN` |
| `project` | `TIRA_PROJECT` |
| `board_id` | `TIRA_BOARD_ID` |
| `classic_project` | `TIRA_CLASSIC_PROJECT` |
| `theme` | `TIRA_THEME` |

The token is resolved in this order:
1. `TIRA_TOKEN` environment variable
2. Config file `token` field
3. `JIRA_TOKEN` environment variable
4. `JIRA_API_TOKEN` environment variable

**Example usage:**

```bash
# Use token from environment, rest from config file
export JIRA_TOKEN="your-api-token-here"
tira board

# Override multiple fields
export TIRA_JIRA_URL="https://myorg.atlassian.net"
export TIRA_EMAIL="me@myorg.com"
export TIRA_TOKEN="my-token"
export TIRA_PROJECT="MYPROJ"
tira backlog
```

This is useful for CI/CD pipelines or shared environments where you don't want to commit tokens to config files.

## Loading Configuration

Configuration is loaded via `config.Load(profileName string)` which uses Viper to read the config file.

**Loading process:**
1. Viper searches for config in `~/.config/tira/` and current directory
2. Selects the profile specified by `profileName`
3. Validates required fields are present
4. Returns a `*Config` struct

## Global Flags

The following global flags are available on all commands (defined in `cmd/tira/root.go`):

| Flag | Default | Description |
|------|---------|-------------|
| `--profile <name>` | `"default"` | Selects which config profile to use |
| `--debug` | `false` | Enables file-based debug logging to `$XDG_STATE_HOME/tira/debug.log` (falls back to `~/.local/state/tira/debug.log`) |
| `--debug-file <path>` | `""` | Enable debug logging to a specific path (implies `--debug`) |
| `--dev` | `false` | Run against a built-in fixture instead of Jira (no config file, credentials, or network) |
| `--dev-fixtures <path>` | `""` | Dev mode: YAML fixture file to load (default: the embedded demo fixture) |
| `--dev-state <path>` | `""` | Dev mode: JSON file that persists mutations across invocations |

## The `cfg` Global

Config is loaded in `PersistentPreRunE` and stored in the package-level `var cfg *config.Config`. All commands access it as `cfg`:

```go
// Example from cmd/tira/get.go
client, err := api.NewClient(cfg.JiraURL, cfg.Email, cfg.Token)
```

## Debug Logging

When `--debug` is passed:
- Debug logger is initialized to write to `./debug.log`
- All HTTP requests are logged (method, URL, body)
- The log file is created in the **current working directory** (not a temp or config directory)

## Multiple Profiles

Use different profiles for different Jira instances or accounts:

```bash
# Use default profile
./tira board

# Use a named profile (see Dev Mode above to run without Jira at all)
./tira --profile staging board

# Use another profile
./tira --profile stg-readonly get STG-101
```

Note that environment variable values apply across all profiles — they are not profile-specific. To use different env var values per profile, switch profiles with the `--profile` flag.

## Dev Mode (mocked Jira)

`--dev` swaps the real `api.Client` for an in-process fixture-backed fake
(`internal/mock`), so every command runs with no config file, no credentials, and no network:

```bash
tira --dev board                     # the real TUI, fed by the demo fixture
tira --dev get DEMO-1                # Markdown, pipe-safe
tira --dev board --snapshot          # one rendered frame, then exit
```

It is a **test double, not a Jira emulator**: `internal/api`'s JSON/ADF/paging code is never
exercised in dev mode, and no request is ever made to a Jira instance. Dev mode is never enabled
by config file contents — only by the flag or the environment — so a stray config value cannot
silently mock a real board. Every invocation prints `dev mode: fixture …` on stderr.

### Environment variables

| Variable | Equivalent flag |
|----------|-----------------|
| `TIRA_DEV_MODE` | `--dev` (truthy values: `1`, `true`, `yes`) |
| `TIRA_DEV_FIXTURES` | `--dev-fixtures` |
| `TIRA_DEV_STATE` | `--dev-state` |

Flags win over the environment variables. In dev mode the config file is optional
(`config.LoadDev`), so a missing or credential-free config is not an error — but a *malformed*
config file still is.

### Defaults from the fixture

When the config does not supply them, dev mode fills in `project` and `board_id` from the fixture
(the embedded demo fixture declares `DEMO` and board `1`), plus a placeholder `jira_url` of
`https://demo.atlassian.net` that is used only to build browser links. `classic_project` defaults
to `true` unless `TIRA_CLASSIC_PROJECT` is set. Use `--project`/`--board-id` to override.

### Fixture format

A fixture is a single YAML file (the embedded default is `internal/mock/fixtures/demo.yaml`):

```yaml
project: DEMO
users:
  - display_name: Ada Lovelace
    account_id: acct-ada
board:
  id: 1
  columns:
    - name: To Do
      statuses: [To Do]        # status names or IDs
sprints:
  - id: 1
    name: DEMO Sprint 1
    state: active              # active | future | closed; missing = future
    issues: [DEMO-3]
backlog: [DEMO-7]
issues:
  - key: DEMO-3
    summary: Redesign checkout form
    type: Story
    status: In Progress
    epic: DEMO-1               # resolved to EpicName/EpicStatus
    parent: DEMO-1             # optional
    subtasks: [DEMO-10]
    links:
      - relationship: blocks
        key: DEMO-4            # both directions must be declared explicitly
transitions:
  default:
    - id: "11"
      name: To Do
```

Unknown keys are rejected with the offending key named, as are dangling references
(`epic`, `parent`, `links[].key`, `subtasks[]`, `backlog[]`, `sprints[].issues[]`), duplicate issue
keys, and keys that do not start with `<project>-`. `priorities` and `issue_types` are optional;
when omitted the client derives them from the fixture contents.

### State file (`--dev-state`)

By default each invocation starts from the fixture and mutations are discarded. Point
`--dev-state` at a JSON file and mutations are persisted, so multi-command agent flows behave like
a real board:

```bash
tira --dev --dev-state /tmp/state.json create --no-edit < template.md
tira --dev --dev-state /tmp/state.json get DEMO-12
```

Three invariants matter:

1. **The state file *is* the fixture.** Once it exists and is non-empty it is loaded instead of
   `--dev-fixtures`, and the startup notice says so. Deleting the file resets to the fixtures.
2. **Unknown keys are rejected** on load, exactly as for a fixture.
3. The file is written atomically (temp file + rename) after every successful mutation, and is
   ignored by git (`.gitignore` covers `.tira-dev-state*` and `dev-state.json`).

A write failure is reported (`persisting dev state: …`) but does not roll back the in-memory
change for that invocation.

## See Also

- [CLI Commands](cli-commands.md) — How configuration is used by each command
- [API Client](api-client.md) — How credentials are used for authentication
