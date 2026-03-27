# tira

![Go Version](https://img.shields.io/badge/Go-1.25-blue)
![Go Report Card](https://goreportcard.com/badge/github.com/justinmklam/tira)
![License](https://img.shields.io/badge/License-MIT-green)

A terminal interface for Jira. Fast, keyboard-driven, and built for people who find the web UI insufferable.

![backlog](./docs/screenshots/backlog.png)

![kanban](./docs/screenshots/kanban.png)

Demo:
<video src="https://github.com/user-attachments/assets/65dfa774-d51a-4980-a965-76393f43c00f" width="320" height="240" controls></video>

## Features

- Create and edit issues
- Organize issues across sprints
- Set issue status, assignee, story points, epics, and more
- Fuzzy search picker for setting assignees and parent issues
- Copy issue links to clipboard, or open them directly in your browser
- Vim-inspired keybindings
- Optimized for speed and responsiveness - no more fighting with the Jira web ui!

## Getting Started

### Installation

Install the tool with curl:

```sh
curl -fsSL https://raw.githubusercontent.com/justinmklam/tira/main/bin/install.sh | bash
```

Or with go:

```sh
go install github.com/justinmklam/tira/cmd/tira@latest
```

Then create `~/.config/tira/config.yaml` and add a default profile:

```yaml
profiles:
  default:
    jira_url: https://yourorg.atlassian.net
    email: you@example.com
    token: your_api_token_here
    project: MYPROJ
    board_id: 42
    classic_project: true   # Optional, set to true for company-managed (classic) projects
```

For clipboard support, install `xclip` (e.g. `sudo apt install xclip`) for Linux. macOS uses `pbcopy`, which is built in to the OS.

### Usage

#### Board TUI

Launch the interactive board TUI:

```sh
# Start in backlog view
tira backlog

# Start in kanban view
tira kanban
```

**Common keybindings:**

| Key | Action |
|-----|--------|
| `Tab` | Toggle between backlog and kanban |
| `j`/`k` | Move cursor to next/prev issue |
| `J`/`K` | Move cursor to next/prev sprint |
| `Enter` | Open issue detail in fullscreen / Toggle sprint collapse |
| `e` | Edit issue |
| `c` | Add comment |
| `s` | Set status |
| `A` | Set assignee |
| `S` | Set story points |
| `P` | Set parent |
| `f<num>` | Jump to issue by number |
| `/<keyword>` | Filter issues by keyword |
| `F` | Open parent issue picker to filter issues by parent |
| `Space` | Select issue |
| `v` | Visual mode (multi-select) |
| `x` / `p` | Cut / Paste selected issue(s) |
| < / > | Move selected issue(s) to prev/next sprint |
| `R` | Refresh from Jira |
| `?` | Show help |
| `q` | Quit |

See [Keybindings](docs/keybindings-backlog.md) for the complete reference.

#### CLI Commands

**View an issue:**

```sh
tira get MP-101
```

**Edit an issue in your editor:**

```sh
tira get MP-101 --edit
```

**Create a new issue:**

```sh
# Interactive with defaults
tira create

# Specify project and type
tira create --project DEV --type Bug
```

## Build & Development

Clone the repository and build the CLI:

```sh
make check        # Run all checks (fmt, vet, lint, test)
make build        # Compile the binary
make run          # Run the tui using your default profile
```

For development, a second `dev` profile can be added to your `~/.config/tira.yaml`:

```yaml
profiles:
  ...
  dev:
    jira_url: https://dev-domain.atlassian.net
    email: dev@example.com
    token: dev_token_here
    project: DEVPROJ
    board_id: 43
```

Other useful commands:

```sh
make run-dev      # Run the tui using your dev profile, with debug enabled
make test         # Run all tests
make test-race    # Run tests with race detector
make fmt          # Format code in-place
make vet          # Run go vet
make lint         # Run golangci-lint (requires golangci-lint installation)
```

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

## Acknowledgements

This project was built with the excellent [bubbletea](https://github.com/charmbracelet/bubbletea) framework by [Charm](https://github.com/charmbracelet).

Inspiration for this tool was taken from:

- [lazygit](https://github.com/jesseduffield/lazygit)
- [gh-dash](https://github.com/dlvhdr/gh-dash)
- [k9s](https://github.com/derailed/k9s)
- [oxker](https://github.com/mrjackwills/oxker)

## License

MIT
