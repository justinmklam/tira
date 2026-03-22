# tira

A lazygit-style CLI for Jira, built in Go with Charm tooling. This project provides a fast, scriptable, and extensible interface to Jira issues, sprints, and boards.

![Go Version](https://img.shields.io/badge/Go-1.25-blue)
![License](https://img.shields.io/badge/License-MIT-green)

## Features

- **Interactive TUI** — Split-view backlog and kanban board with fuzzy search, multi-select, and drag-and-drop-like operations
- **View and edit issues** — Full issue detail view with comments, edit via `$EDITOR` or in-TUI form
- **Create issues** — New issues via markdown template in your editor
- **Multiple profiles** — Switch between Jira instances or accounts with `--profile`
- **Fast and stateless** — No local database; auth via config file

## Getting Started

### Prerequisites

- Go 1.25+
- A Jira Cloud account with API access
- Clipboard support (optional, for copying URLs):
  - **macOS**: `pbcopy` (built-in)
  - **Linux**: `xclip` (`sudo apt install xclip` or `sudo dnf install xclip`)

### Installation

Clone the repository and build the CLI:

```sh
git clone https://github.com/justinmklam/tira.git
cd tira
go build -o tira ./cmd/tira
```

### Configuration

Create `~/.config/tira/config.yaml` and add your profile(s):

```yaml
profiles:
  default:
    jira_url: https://yourorg.atlassian.net
    email: you@example.com
    token: your_api_token_here
    project: MYPROJ
    board_id: 42
    classic_project: true   # Set to true for company-managed (classic) projects
  dev:
    jira_url: https://dev-domain.atlassian.net
    email: dev@example.com
    token: dev_token_here
    project: DEVPROJ
    board_id: 43
```

**Required fields:**

- `jira_url` — Your Jira Cloud instance URL
- `email` — Your Jira Cloud email address
- `token` — Your Jira API token (generate from <https://id.atlassian.com/manage-profile/security/api-tokens>)

**Optional fields:**

- `project` — Default project key
- `board_id` — Required for `board`/`backlog`/`kanban` commands
- `classic_project` — Affects browser URL construction only

See [Configuration](docs/configuration.md) for details.

### Usage

#### Board TUI

Launch the interactive board TUI:

```sh
# Start in backlog view (default)
./tira board

# Start in kanban view
./tira kanban

# Use specific profile
./tira --profile dev backlog
```

**Common keybindings:**

| Key | Action |
|-----|--------|
| `Tab` | Toggle between backlog and kanban |
| `j`/`k` | Move down/up |
| `Enter` | Open issue detail / Toggle sprint collapse |
| `e` | Edit issue (in-TUI form) |
| `c` | Add comment |
| `/` | Filter issues |
| `Space` | Select issue |
| `v` | Visual mode (extend selection) |
| `x` / `p` | Cut / Paste issues |
| `s` | Change status |
| `A` | Set assignee |
| `S` | Set story points |
| `R` | Refresh from Jira |
| `?` | Show help |
| `q` | Quit |

See [Keybindings](docs/keybindings-backlog.md) for the complete reference.

#### CLI Commands

**View an issue:**

```sh
./tira get MP-101
```

**Edit an issue in your editor:**

```sh
./tira get MP-101 --edit
```

**Create a new issue:**

```sh
# Interactive with defaults
./tira create

# Specify project and type
./tira create --project DEV --type Bug
```

**Use a specific profile:**

```sh
./tira --profile dev get DEV-101
./tira --profile dev board
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--profile <name>` | `"default"` | Select which config profile to use |
| `--debug` | `false` | Enable file-based debug logging to `./debug.log` |
| `--no-color` | `false` | Disable color output |

### Commands

| Command | Description |
|---------|-------------|
| `board` | Launch TUI in backlog view |
| `backlog` | Launch TUI in backlog view |
| `kanban` | Launch TUI in kanban view |
| `get <key>` | Fetch and display an issue |
| `get <key> --edit` | Edit an issue via `$EDITOR` |
| `create` | Create a new issue via `$EDITOR` |

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture](docs/architecture.md) | System architecture and package structure |
| [CLI Commands](docs/cli-commands.md) | Detailed CLI command documentation |
| [Configuration](docs/configuration.md) | Configuration system details |
| [TUI Architecture](docs/tui-architecture.md) | TUI model architecture |
| [API Client](docs/api-client.md) | API client implementation |
| [Keybindings](docs/keybindings-backlog.md) | Complete keybinding reference |
| [Glossary](docs/glossary.md) | Glossary and key types |

## Build & Development

```sh
make build        # Compile the binary
make test         # Run all tests
make test-race    # Run tests with race detector
make fmt          # Format code in-place
make vet          # Run go vet
make lint         # Run golangci-lint (requires golangci-lint installation)
make check        # Run all checks (fmt, vet, lint, test)
```

Always run `make check` before pushing.

## Project Structure

```
tira/
├── cmd/tira/           # CLI layer (Cobra commands)
├── internal/
│   ├── app/            # TUI models (Bubbletea)
│   ├── api/            # Jira API client
│   ├── config/         # Configuration loading
│   ├── models/         # Data types
│   ├── tui/            # TUI helpers (zero internal deps)
│   ├── display/        # Issue → Markdown renderer
│   ├── editor/         # Template rendering
│   ├── validator/      # Field validation
│   └── debug/          # Debug logging
├── docs/               # Documentation
└── config.example.yaml # Example config
```

See [Architecture](docs/architecture.md) for the full package dependency graph.

## Contributing

Pull requests welcome! Key areas:

- **Bug fixes** — Especially around edge cases in API response parsing
- **New features** — Check [tira-plan.md](docs/tira-plan.md) for planned features
- **Documentation** — Improve clarity or add missing sections
- **Tests** — Add unit tests for API parsing, form logic, kanban mapping

Before submitting a PR:

1. Run `make check` to ensure all checks pass
2. Update keybindings in [docs/keybindings-backlog.md](docs/keybindings-backlog.md) if adding new keys
3. Update [docs/architecture.md](docs/architecture.md) if changing package structure

## License

MIT
