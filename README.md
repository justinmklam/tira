# tira

A lazygit-style CLI for Jira, built in Go with Charm tooling. This project provides a fast, scriptable, and extensible interface to Jira issues, sprints, and boards.

## Features
- View and edit Jira issues from the terminal
- List active sprints and backlog
- Create new issues with a markdown template
- Supports multiple configuration profiles
- Fast, stateless auth via config file

## Getting Started

### Prerequisites
- Go 1.21+
- A Jira Cloud account with API access
- Clipboard support (optional):
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
    token: your_token_here
    project: MYPROJ
    board_id: 42
  dev:
    jira_url: https://dev-domain.atlassian.net
    email: dev-email@example.com
    token: dev-api-token
    project: DEVPROJ
    board_id: 43
```

### Usage

- Use a specific profile (defaults to `default`):
  ```sh
  ./tira --profile dev list
  ```
- View an issue:
  ```sh
  ./tira get MP-101
  ```
- Edit an issue in your editor:
  ```sh
  ./tira get MP-101 --edit
  ```
- List active sprint issues:
  ```sh
  ./tira list
  ```
- List all sprints and backlog:
  ```sh
  ./tira list --backlog
  ```
- Create a new issue:
  ```sh
  ./tira create
  ```

### Flags
- `--profile <name>` to select a config profile (default: `default`)
- `--debug` for verbose logging
- `--no-color` to disable color output

## Contributing
Pull requests welcome! See the plan in `docs/tira-plan.md` for roadmap and structure.

## License
MIT
