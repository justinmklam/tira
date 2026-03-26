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
    theme: catppuccin       # color theme (default, catppuccin)
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
- `token` — Your Jira API token (generate from https://id.atlassian.com/manage-profile/security/api-tokens)

**Optional fields:**
- `project` — Default project key (e.g., `MYPROJ`)
- `board_id` — Default board ID for the `board`/`backlog`/`kanban` commands
- `classic_project` — Set to `true` for company-managed (classic) projects; affects browser URL construction only
- `theme` — Color theme for the TUI. Available themes: `default`, `catppuccin`. If omitted, uses terminal's default ANSI 256 colors

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
| `--debug` | `false` | Enables file-based debug logging to `./debug.log` |

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

# Use dev profile
./tira --profile dev board

# Use staging profile
./tira --profile staging get STG-101
```

## Environment Variables

The configuration system uses Viper but does **not** currently support environment variable overrides. All configuration must be in the YAML file.

## See Also

- [CLI Commands](cli-commands.md) — How configuration is used by each command
- [API Client](api-client.md) — How credentials are used for authentication
